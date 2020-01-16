package betterbox_test

import (
	"betterbox"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	serverAddress = "localhost"
	serverPort    = 12345
)

type testEntry struct {
	name    string
	ftype   int
	content []byte
}

const (
	DIR  = 0
	FILE = 1
)

func createTempDirWithFiles(t *testing.T, tFiles []testEntry) string {
	t.Helper()
	dir, err := ioutil.TempDir("", "betterbox_test")
	if err != nil {
		t.Fatalf("Can't create temporary directory: %v", err)
	}
	for _, tFile := range tFiles {
		path := filepath.Join(dir, tFile.name)
		if tFile.ftype == DIR {
			err = os.Mkdir(path, 0700|os.ModeDir)
			if err != nil {
				t.Fatalf("Can't create temporary directory '%s': %v", tFile.name, err)
			}
		} else {
			err := ioutil.WriteFile(path, tFile.content, 0600)
			if err != nil {
				t.Fatalf("Can't create temporary file '%s': %v", tFile.name, err)
			}
		}
	}
	return dir
}

func TestClientServerIntegration(t *testing.T) {
	tFiles := []testEntry{
		{"file1", FILE, []byte("file1 content")},
		{"file2", FILE, []byte("file2 content")},
		{"file3", FILE, nil},
		{"dir1", DIR, nil},
		{"dir1/file4", FILE, []byte("file4 content")},
	}
	sdir := createTempDirWithFiles(t, nil)
	//defer os.RemoveAll(sdir)
	server, err := betterbox.NewServer(serverAddress, serverPort, sdir)
	if err != nil {
		t.Fatalf("Can't instantiate new server: %v", err)
	}
	go server.Listen()

	cdir := createTempDirWithFiles(t, tFiles)
	//defer os.RemoveAll(cdir)
	log.Println(cdir, sdir) //

	time.Sleep(1 * time.Second) //
	client, err := betterbox.NewClient(serverAddress, serverPort, cdir)
	if err != nil {
		t.Fatalf("Can't instantiate new client: %v", err)
	}

	err = client.Sync()
	if err != nil {
		t.Fatalf("Client can't send files to server: %v", err)
	}
	compareDirectories(t, cdir, sdir)
}

func compareDirectories(t *testing.T, dir1, dir2 string) {
	t.Helper()
	// XXX Walk directories, compare file names, last modified, size, content hash etc,.
	output, err := exec.Command("diff", "--brief", "-r", dir1, dir2).Output()
	if err != nil {
		// exit status not 0
		t.Fatalf("Directories differ: %s", output)
	}
}
