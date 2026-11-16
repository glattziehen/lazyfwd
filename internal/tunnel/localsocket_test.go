package tunnel

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// shortTempDir keeps socket paths under the 104-byte limit, which
// t.TempDir's long names can exceed on macOS.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "lzf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestFreeLocalSocketRemovesStaleSocket(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "stale.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	listener.Close()

	if err := freeLocalSocket(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("stale socket survived: %v", err)
	}
}

func TestFreeLocalSocketKeepsLiveSocket(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "live.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	if err := freeLocalSocket(path); err == nil {
		t.Fatal("freed a socket that accepts connections")
	}
}

func TestFreeLocalSocketKeepsOtherFiles(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "notes.txt")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := freeLocalSocket(path); err == nil {
		t.Fatal("freed a regular file")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("regular file removed: %v", err)
	}
}
