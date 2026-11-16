//go:build unix

package lockfile

import (
	"errors"
	"os"
	"syscall"
)

// TryLock takes an exclusive flock on file without waiting, or returns
// ErrLocked.
func TryLock(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrLocked
	}
	return err
}
