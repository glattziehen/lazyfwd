package tunnel

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isProcessAlive reports whether pid exists and has not exited. A process
// lazyfwd may not open, such as another user's, counts as alive.
func isProcessAlive(pid int) bool {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(process) }()
	event, err := windows.WaitForSingleObject(process, 0)
	return err == nil && event == uint32(windows.WAIT_TIMEOUT)
}
