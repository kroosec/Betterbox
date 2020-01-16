package betterbox

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"time"
)

const (
	// Max number of requests (file/dir creations/modifications/deletions)
	// to buffer before sending them to server.
	requestsBufferSize = 100
	// Max time of requests buffering before sending them to server.
	requestsWaitTime = 5 * time.Second
)

type Client struct {
	path    string            // Path of directory to sync and monitor.
	server  string            // Server's address:port
	watcher *fsnotify.Watcher // Watcher for filsystem events.
	config  *tls.Config       // TLS config.
	// XXX Add custom logger
}

// getClientTLSConfig returns a TLS config for the client to verify the server.
func getClientTLSConfig() (*tls.Config, error) {
	// XXX Add flags for server cert path.
	caCert, err := ioutil.ReadFile("./certs/server.cert")
	if err != nil {
		return nil, errors.Wrap(err, "Creating TLS config failed")
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCert)
	return &tls.Config{RootCAs: certPool}, nil
}

// NewClient creates a new client, given the server's address and port and the
// path to a directory that would be synchronized with the server.
func NewClient(address string, port uint16, path string) (*Client, error) {
	// XXX Other directory checks ? eg. permissions of directory (and contained files/dirs) ?
	if !isDirectory(path) {
		return nil, fmt.Errorf("%s: Path not a directory", path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	addrport := net.JoinHostPort(address, fmt.Sprintf("%d", port))
	if _, err = net.ResolveTCPAddr("tcp", addrport); err != nil {
		return nil, err
	}
	config, err := getClientTLSConfig()
	if err != nil {
		return nil, err
	}

	return &Client{server: addrport, path: absPath, config: config}, nil
}

// serverConnect connects to the server through RPC over TLS.
func (c *Client) serverConnect() (*rpc.Client, error) {
	conn, err := tls.Dial("tcp", c.server, c.config)
	if err != nil {
		return nil, err
	}
	// XXX Replace with NewClientWithCodec() to use a custom RPC encoder,
	// to not buffer file content in Request.Data
	return rpc.NewClient(conn), nil
}

// sendRequests sends a list of Requests to the server. In case of a Request
// receiving an error Response by the server, the sending will stop.
func (c *Client) sendRequests(reqs []*Request) error {
	if len(reqs) == 0 {
		return nil
	}
	rconn, err := c.serverConnect()
	if err != nil {
		return errors.Wrap(err, "Connection to server failed")
	}
	defer rconn.Close()

	for _, req := range reqs {
		var resp Response
		// XXX On concurrency: We have to synchronize order for
		// Requests that are not interchangeable (eg. Create and Remove of the same file.)
		// Any other considerations ?
		// XXX Optimization for network bandwidth:
		// - Send file info from inode (last modified, size) to server to see if sending is needed.
		// - Send file in chunks (send/compare checksum with server first)
		// XXX Zero-copy: Remove Data buffer from Request, use
		// sendfile()/splice()/copy_file_range() + rpc.ClientCodec.
		rconn.Call("Server.ApplyRequest", req, &resp)
		// Stop sending of requests on first error from server.
		if resp.Type == responseErr {
			// XXX Should we continue ? How to handle files that caused errors in that case ?
			return fmt.Errorf("Sending request to server '%s' failed: %s", req, resp)
		}
	}
	return nil
}

// newMkdirRequest creates a new Mkdir Request.
func newMkdirRequest(name string) *Request {
	return &Request{Type: requestMkdir, Path: name}
}

// newCreateRequest creates a new Create Request.
func newCreateRequest(path, name string) (*Request, error) {
	// XXX Better to delay reading the file content until it is needed
	// inside sendRequests() loop, and skip the overhead from copying data.
	// ==> Replace Request.Data by the file descriptor, then use
	// splice(2) (or other) for zero-copying (use
	// rpc.NewClientWithCodec() instead of rpc.NewClient())
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &Request{Type: requestCreate, Path: name, Data: content}, nil
}

// newRemoveRequest creates a new Remove Request.
func newRemoveRequest(name string) *Request {
	return &Request{Type: requestRemove, Path: name}
}

// Sync walks through all the files and subdirectories in the client's
// directory, and sends them to the server.
func (c *Client) Sync() error {
	// Regroups commands (directory and file creations) before sending them.
	var reqs []*Request
	err := filepath.Walk(c.path, func(absPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// filepath.Walk() returns the root path too. Skip it.
		if absPath == c.path {
			return nil
		}
		// Get rel path: absPath minus c.path
		relPath, err := filepath.Rel(c.path, absPath)
		if err != nil {
			return err
		}
		if info.IsDir() {
			req := newMkdirRequest(relPath)
			reqs = append(reqs, req)
		} else {
			req, err := newCreateRequest(absPath, relPath)
			if err != nil {
				return err
			}
			reqs = append(reqs, req)
		}
		// Don't buffer requests forever. Especially important as
		// the Requests contain the full file content.
		if len(reqs) == requestsBufferSize {
			if err := c.sendRequests(reqs); err != nil {
				return err
			}
			reqs = nil
		}
		return nil
	})
	if err != nil {
		// No partial sending on filepath errors.
		return err
	}
	return c.sendRequests(reqs)
}

// startWatcher starts the monitoring of the client's directory for filesystem
// events (file creations, chmod's, dir creations etc,.)
func (c *Client) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	c.watcher = watcher
	if err = c.recursiveAddWatchers(c.path); err != nil {
		watcher.Close()
		return err
	}
	return nil
}

// recursiveAddWatchers recursively adds directories within the provided root directory.
func (c *Client) recursiveAddWatchers(root string) error {
	return walkDir(root, func(path string) error {
		return c.watcher.Add(path)
	})
}

// walkDir calls dirFunc function for all the subdirectories of the provided root directory.
func walkDir(root string, dirFunc func(string) error) error {
	return filepath.Walk(root, func(subPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		return dirFunc(subPath)
	})
}

// Close closes the client's directory watcher.
func (c *Client) Close() {
	if c.watcher != nil {
		c.watcher.Close()
	}
}

// SyncAndMonitor sends all files and directories to the server and watches for
// filesystem events in that directory (eg. a file is modified, a directory is
// removed etc,.) to send them to the server.
func (c *Client) SyncAndMonitor() error {
	// Start file events watcher before calling Sync(), to handle cases
	// where files are created/modified/deleted while data is initially
	// sent to the server. The events will be handled after the initial
	// sending by watcherLoop() accordingly.
	if err := c.startWatcher(); err != nil {
		return errors.Wrapf(err, "Monitoring directory '%s' failed", c.path)
	}
	if err := c.Sync(); err != nil {
		return errors.Wrap(err, "Initial files sending failure")
	}
	return c.watcherLoop()
}

// watcherLoop watches the client directory for any filesystem events and sends
// to the server.
func (c *Client) watcherLoop() error {
	var reqs []*Request
	// Events (File/Directory creation/modification/removal) are buffered
	// instead of being directly. This allows us to use the same TLS/TCP
	// connection for all the sent requests, instead of opening/closing a
	// new one each time. This would also allow for some possible
	// optimizations (not done currently) eg. if a file is modified
	// multiple times, only send it once.  The buffering is done up to
	// requestsWaitTime time of no-activity.
	// The requestsBufferSize cap is added in order to prevent constant
	// events (eg. a file modified every 1 second) from being held forever.
	for {
		select {
		case event, ok := <-c.watcher.Events:
			if !ok {
				// Exit on watcher close.
				log.Println("Done monitoring")
				return nil
			}
			req, err := c.handleEvent(event)
			if err != nil {
				// Stop monitoring on first error.
				return errors.Wrap(err, "Handling file event failed")
			}
			if req != nil {
				reqs = append(reqs, req)
			}
			// Don't keep buffering requests forever, in case of
			// constant filesystem activity in the watched directories.
			if len(reqs) == requestsBufferSize {
				if err := c.sendRequests(reqs); err != nil {
					return err
				}
				reqs = nil
			}
		case err, ok := <-c.watcher.Errors:
			if !ok {
				log.Println("Done monitoring")
				return nil
			}
			return err
		case <-time.After(requestsWaitTime):
			if len(reqs) > 0 {
				if err := c.sendRequests(reqs); err != nil {
					return err
				}
				reqs = nil
			}
		}
	}
}

// isDirectory checks if the provided path is a directory.
func isDirectory(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// handleEvent handles a filesystem event, returing an adequate Request
// eventually. In case of a Chmod event, nil is returned.
func (c *Client) handleEvent(event fsnotify.Event) (*Request, error) {
	relPath, err := filepath.Rel(c.path, event.Name)
	if err != nil {
		return nil, err
	}
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		if isDir := isDirectory(event.Name); isDir {
			if err := c.recursiveAddWatchers(event.Name); err != nil {
				return nil, err
			}
			return newMkdirRequest(relPath), nil
		} else {
			req, err := newCreateRequest(event.Name, relPath)
			if err != nil {
				return nil, err
			}
			return req, err
		}
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		return newRemoveRequest(relPath), nil
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		// Rename is treated like a delete. If the new
		// filename is within watched directories,
		// fsnotify will send a Create even accordingly.
		return newRemoveRequest(relPath), nil
	case event.Op&fsnotify.Write == fsnotify.Write:
		req, err := newCreateRequest(event.Name, relPath)
		if err != nil {
			return nil, err
		}
		return req, nil
	case event.Op&fsnotify.Chmod == fsnotify.Chmod:
		// XXX Nothing to do on the server's side ?
		return nil, nil
	default:
		return nil, fmt.Errorf("Erroneous event value (%d): %s", event.Op, event.Name)
	}
}
