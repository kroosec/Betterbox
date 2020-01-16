package betterbox

import (
	"crypto/tls"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"path/filepath"
)

type Server struct {
	// IP Address to listen on.
	address string
	// TCP Port to listen on.
	port uint16
	// Destination path of the files received from the client.
	path string
	// TLS configuration of the server.
	config *tls.Config
	// XXX Add custom logger
}

// checkOrMakeEmptyDirectory checks that the provided path is an empty
// directory. If no file or directory was found in the provided path, a new directory is created.
func checkOrMakeEmptyDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		// If path contains no file or directory, create an empty directory.
		return os.Mkdir(path, 0755|os.ModeDir)
	}
	defer dir.Close()
	dirInfo, err := dir.Stat()
	if err != nil {
		return err
	}
	if !dirInfo.IsDir() {
		return fmt.Errorf("%s: Not a directory", path)
	}
	if _, err = dir.Readdir(1); err != io.EOF {
		return fmt.Errorf("%s: Directory is not empty", path)
	}
	// XXX Check directory permissions ? Other checks ?
	return nil
}

// NewServer creates a new server, using the provided IP address, TCP port and
// destination path for the received files.
func NewServer(address string, port uint16, path string) (*Server, error) {
	// Validate provided parameters.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := checkOrMakeEmptyDirectory(path); err != nil {
		return nil, err
	}
	config, err := newServerTLSConfig()
	if err != nil {
		return nil, err
	}
	return &Server{address: address, port: port, path: absPath, config: config}, nil
}

func (sv *Server) String() string {
	return fmt.Sprintf("%s:%d -> %s", sv.address, sv.port, sv.path)
}

// newServerTLSConfig creates a new TLS config for the server.
func newServerTLSConfig() (*tls.Config, error) {
	// XXX Hardcoded paths.
	cert, err := tls.LoadX509KeyPair("./certs/server.cert", "./certs/server.key")
	if err != nil {
		return nil, errors.Wrap(err, "Creating TLS config failed")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil
}

// Listen listens for client connections on the provided address and port and
// executes the received RPC commands.
func (sv *Server) Listen() {
	// Using default RPC server.
	if err := rpc.Register(sv); err != nil {
		log.Println("Registering RPC service", err)
		return
	}
	listener, err := tls.Listen("tcp", fmt.Sprintf("%s:%d", sv.address, sv.port), sv.config)
	if err != nil {
		log.Println("Starting TCP listener: ", err)
		return
	}
	// XXX Synchronous, blocking handling of client(s) as the order of Requests (eg.
	// creating a file, and removing it) is not interchangeable.
	// Would synchronizing operations on directory be sufficient ?
	rpc.Accept(listener)
}

// validateRequest validates that a received Request doesn't contain erroneous information.
func (sv *Server) validateRequest(req *Request) error {
	if req.Path == "" {
		return fmt.Errorf("Missing request path")
	}
	if req.Path != filepath.Clean(req.Path) {
		// XXX Catches all path traversal attempts ?
		// Does also exclude "valid" path values such as "foo/bar/../somefile"
		return fmt.Errorf("Erroneous path value: '%s'", req.Path)
	}
	// XXX More sanity checks
	return nil
}

// ApplyRequest applies the provided Request, and returns a Response adequately.
func (sv *Server) ApplyRequest(req *Request, resp *Response) error {
	log.Println("Received request: ", req)
	var err error
	resp.Type = responseOk
	resp.Message = ""
	if err = sv.validateRequest(req); err != nil {
		resp.Type = responseErr
		resp.Message = err.Error()
		return nil
	}
	absPath := filepath.Join(sv.path, req.Path)
	switch req.Type {
	case requestMkdir:
		err = os.Mkdir(absPath, 0700|os.ModeDir)
	case requestCreate:
		// XXX Create parent directories if they don't exist ? Not
		// needed, as client does / has to send Mkdir before that.
		err = ioutil.WriteFile(absPath, req.Data, 0600)
	case requestRemove:
		err = os.RemoveAll(absPath)
	default:
		err = fmt.Errorf("Unhandled request: %s", req)
	}
	if err != nil {
		resp.Type = responseErr
		// XXX Information disclosure to the client.
		resp.Message = err.Error()
	}
	return nil
}
