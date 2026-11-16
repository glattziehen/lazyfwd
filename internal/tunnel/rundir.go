package tunnel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
)

// Each lazyfwd process keeps its masters' sockets and logs in its own run
// directory, <base>/<pid>, and holds an flock on <base>/<pid>/lock while it
// runs. The kernel drops the lock when the process dies, however it dies, so
// a later lazyfwd can tell a crashed instance's directory from a live one and
// close the masters left in it, which still hold their forwards' ports.

const (
	lockName = "lock"
	logExt   = ".log"
	// maxControlPathLength leaves room in a unix socket path (103 bytes on
	// macOS, 107 on Linux) for the 17-byte suffix ssh adds while it binds.
	maxControlPathLength = 86
	// socketNameLength is the hex length of a host's socket name.
	socketNameLength = 10
	// maxPIDLength fits Linux's largest pid_max, 4194304.
	maxPIDLength = 7
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

// fitsSocketPath reports whether every socket under base stays within
// maxControlPathLength.
func fitsSocketPath(base string) bool {
	return len(base)+len("/")+maxPIDLength+len("/")+socketNameLength <= maxControlPathLength
}

// socketName is a host's socket file name: short, fixed length, and free of
// characters a host name may hold.
func socketName(host string) string {
	sum := sha256.Sum256([]byte(host))
	return hex.EncodeToString(sum[:])[:socketNameLength]
}

var socketNamePattern = regexp.MustCompile(`^[0-9a-f]{` + strconv.Itoa(socketNameLength) + `}$`)

// isSocketName reports whether name is a host's socket name, rather than the
// lock, a log, or another file in the run directory.
func isSocketName(name string) bool {
	return socketNamePattern.MatchString(name)
}

// runDir is this process's locked directory of master sockets.
type runDir struct {
	path string
	lock *os.File
}

// openRunDir creates and locks this process's run directory under base,
// after closing the masters of crashed instances.
func openRunDir(base string) (runDir, error) {
	if err := ensurePrivateDir(base); err != nil {
		return runDir{}, err
	}
	closeLeftovers(base)
	path := filepath.Join(base, strconv.Itoa(os.Getpid()))
	if err := os.Mkdir(path, 0o700); err != nil {
		return runDir{}, err
	}
	lock, err := tryLock(filepath.Join(path, lockName))
	if err != nil {
		return runDir{}, err
	}
	if lock == nil {
		return runDir{}, fmt.Errorf("%s is locked by another process", path)
	}
	return runDir{path: path, lock: lock}, nil
}

func (d runDir) socketPath(host string) string {
	return filepath.Join(d.path, socketName(host))
}

// remove deletes the run directory and releases its lock.
func (d runDir) remove() {
	_ = os.RemoveAll(d.path)
	_ = d.lock.Close()
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

// closeLeftovers exits the masters in run directories whose lock is free,
// meaning their lazyfwd has died, and deletes those directories.
func closeLeftovers(base string) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, entry := range entries {
		dir := filepath.Join(base, entry.Name())
		lock, err := tryLock(filepath.Join(dir, lockName))
		if err != nil || lock == nil {
			continue
		}
		files, _ := os.ReadDir(dir)
		for _, file := range files {
			if isSocketName(file.Name()) {
				// -O ignores the destination, but ssh requires one.
				_, _ = runControl(context.Background(), controlArgs(filepath.Join(dir, file.Name()), "exit", "lazyfwd"))
			}
		}
		_ = os.RemoveAll(dir)
		_ = lock.Close()
	}
}

// tryLock opens path and takes an exclusive flock without waiting; it
// returns nil when another process holds the lock. Go opens files
// close-on-exec, so ssh children do not inherit the lock.
func tryLock(path string) (*os.File, error) {
	lock, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return lock, nil
	}
	_ = lock.Close()
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return nil, nil
	}
	return nil, err
}
