//go:build unix

package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// DefaultBaseDir is the parent of every lazyfwd run directory:
// $XDG_RUNTIME_DIR, else the temp dir, else /tmp, whichever first keeps
// socket paths short enough.
func DefaultBaseDir() string {
	name := "lazyfwd-" + strconv.Itoa(os.Getuid())
	for _, parent := range []string{os.Getenv("XDG_RUNTIME_DIR"), os.TempDir()} {
		base := filepath.Join(parent, name)
		if parent != "" && fitsSocketPath(base) {
			return base
		}
	}
	return filepath.Join("/tmp", name)
}

// ensurePrivateDir creates dir, or accepts an existing one, only if it is a
// real directory that only this user can access; others could otherwise
// reach the masters through its sockets.
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 || !ok || int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("%s must be a directory that only you can access", dir)
	}
	return nil
}
