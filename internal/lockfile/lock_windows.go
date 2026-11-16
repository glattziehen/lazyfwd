package lockfile

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// TryLock takes an exclusive lock without waiting, or returns ErrLocked.
// Windows releases it when the handle closes, which includes the end of the
// process. A locked byte range refuses reads and writes through other
// handles, so the lock covers one byte far past any content, and the file
// stays readable.
func TryLock(file *os.File) error {
	beyondContent := windows.Overlapped{Offset: 0xFFFFFFFF, OffsetHigh: 0x7FFFFFFF}
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &beyondContent)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrLocked
	}
	return err
}
