package lockfile

import (
	"bufio"
	"fmt"
	"os/exec"
	"time"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openLockFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

// Two lazyfwd instances must not both hold the lock; flock and LockFileEx
// both apply between open files of one process too, so one process can test it.
func TestTryLockRefusesASecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	if err := TryLock(openLockFile(t, path)); err != nil {
		t.Fatal(err)
	}

	if err := TryLock(openLockFile(t, path)); !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock: got %v, want ErrLocked", err)
	}
}

func TestTryLockSucceedsOnceTheHolderCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	first := openLockFile(t, path)
	if err := TryLock(first); err != nil {
		t.Fatal(err)
	}
	_ = first.Close()

	if err := TryLock(openLockFile(t, path)); err != nil {
		t.Fatalf("lock after release: %v", err)
	}
}

// holderEnv names the lock file a helper process locks before it waits to be
// killed.
const holderEnv = "LOCKFILE_TEST_HOLD"

// TestMain runs the helper when holderEnv is set, instead of the tests.
func TestMain(m *testing.M) {
	if path := os.Getenv(holderEnv); path != "" {
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err == nil {
			err = TryLock(file)
		}
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println("locked")
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// A crashed lazyfwd must not leave its lock behind.
func TestTryLockSucceedsOnceTheHolderIsKilled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	holder := exec.Command(os.Args[0])
	holder.Env = append(os.Environ(), holderEnv+"="+path)
	stdout, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(stdout).ReadString('\n')
	if line != "locked\n" {
		_ = holder.Process.Kill()
		t.Fatalf("holder: %q", line)
	}
	if err := TryLock(openLockFile(t, path)); !errors.Is(err, ErrLocked) {
		t.Fatalf("lock while the holder runs: got %v, want ErrLocked", err)
	}

	_ = holder.Process.Kill()
	_ = holder.Wait()

	if err := TryLock(openLockFile(t, path)); err != nil {
		t.Fatalf("lock after the holder was killed: %v", err)
	}
}
