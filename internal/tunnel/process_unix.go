//go:build unix

package tunnel

import (
	"errors"
	"syscall"
)

// isProcessAlive reports whether pid exists. The master is not lazyfwd's
// child, so lazyfwd cannot wait for it.
func isProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
