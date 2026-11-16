package tunnel

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"syscall"
)

// freeLocalSocket readies path for a forward to listen on. ssh refuses to
// bind over an existing file and leaves its socket file behind when a forward
// stops, so a stale socket from an earlier run is removed. A socket that
// still accepts connections, or any other file, is left alone and reported.
func freeLocalSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode().Type() != fs.ModeSocket {
		return fmt.Errorf("%s exists and is not a socket", path)
	}
	connection, err := net.Dial("unix", path)
	if err == nil {
		connection.Close()
		return fmt.Errorf("%s is in use by another program", path)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return err
	}
	return os.Remove(path)
}
