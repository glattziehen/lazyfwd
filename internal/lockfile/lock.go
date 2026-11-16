// Package lockfile takes exclusive locks on files that last until the file
// closes or the process ends, however it ends, so no stale lock survives a
// crash.
package lockfile

import "errors"

// ErrLocked reports that another open file, usually in another process,
// holds the lock.
var ErrLocked = errors.New("locked by another process")
