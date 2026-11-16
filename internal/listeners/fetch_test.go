package listeners

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// exitError is a real *exec.ExitError: the remote command ran and failed.
func exitError(t *testing.T) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit 3").Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want an exit error, got %v", err)
	}
	return err
}

func runner(stdout string, err error) Runner {
	return func(context.Context, string, string) (string, error) { return stdout, err }
}

func TestFetchKeepsSSHsErrorForCallers(t *testing.T) {
	refused := errors.New("Permission denied (publickey).")

	_, err := Fetch(runner("", refused), "box")

	if !errors.Is(err, refused) || !strings.Contains(err.Error(), "listing ports on box failed") {
		t.Fatalf("got %v", err)
	}
}

func TestFetchReportsHostWithoutScriptOutputAsUnlistable(t *testing.T) {
	_, err := Fetch(runner("", exitError(t)), "box")

	if err == nil || !strings.HasPrefix(err.Error(), "cannot list ports on box") {
		t.Fatalf("got %v", err)
	}
}

func TestFetchUsesScriptOutputDespiteAFailedExit(t *testing.T) {
	// cat fails on a missing /proc/net/tcp6 after printing /proc/net/tcp.
	output := sectionProc + "\n" +
		"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 1 1 0000000000000000 100 0 0 10 0\n"

	found, err := Fetch(runner(output, exitError(t)), "box")

	if err != nil || len(found.Listeners) != 1 || found.Listeners[0].Port != 8080 {
		t.Fatalf("got %v, %v", found, err)
	}
}
