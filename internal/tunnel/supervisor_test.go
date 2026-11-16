package tunnel

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

// fakeSSH puts an ssh on PATH that logs its pid and arguments to a file in
// dir, then runs body. It returns that log's path.
func fakeSSH(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$$ $*\" >> " + calls + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

func newSupervisor(t *testing.T) *Supervisor {
	t.Helper()
	supervisor, err := NewSupervisor(filepath.Join(shortTempDir(t), "run"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(supervisor.StopAll)
	return supervisor
}

func localForward(id workspace.ForwardID, host string) workspace.Forward {
	return workspace.Forward{ID: id, Host: host, Direction: workspace.Local, Bind: workspace.End{Host: workspace.BindHost, Port: 9000 + int(id)}, Target: workspace.End{Host: "localhost", Port: 80}}
}

// within fails the test unless f returns before timeout.
func within(t *testing.T, timeout time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%s took longer than %s", what, timeout)
	}
}

// waitForCalls waits until the fake ssh has logged count calls.
func waitForCalls(t *testing.T, calls string, count int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, _ := os.ReadFile(calls)
		if lines := strings.Split(strings.TrimSpace(string(content)), "\n"); len(content) > 0 && len(lines) >= count {
			return lines
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fake ssh got fewer than %d calls", count)
	return nil
}

// A host whose connect hangs must not block the key presses that start and
// stop its forwards, however many pile up.
func TestStartAndStopNeverWaitOnAHangingHost(t *testing.T) {
	fakeSSH(t, "exec sleep 30")
	supervisor := newSupervisor(t)
	forward := localForward(1, "tarpit")

	within(t, 2*time.Second, "2000 starts and stops", func() {
		for range 1000 {
			supervisor.Start(forward)
			supervisor.Stop(forward.ID)
		}
	})
}

// Quitting while a connect hangs must return at once and leave no ssh behind.
func TestStopAllEndsAHangingConnect(t *testing.T) {
	calls := fakeSSH(t, "exec sleep 30")
	supervisor := newSupervisor(t)
	supervisor.Start(localForward(1, "tarpit"))
	pid, _ := strconv.Atoi(strings.Fields(waitForCalls(t, calls, 1)[0])[0])

	within(t, 2*time.Second, "StopAll", supervisor.StopAll)

	if err := syscall.Kill(pid, 0); err == nil {
		t.Errorf("ssh %d still runs after StopAll", pid)
	}
}

// Forwards queued on a host while its connect fails share that one attempt.
func TestQueuedForwardsShareOneFailedConnect(t *testing.T) {
	calls := fakeSSH(t, "sleep 0.3; exit 255")
	supervisor := newSupervisor(t)
	for id := range workspace.ForwardID(3) {
		supervisor.Start(localForward(id+1, "down"))
	}
	for range 3 {
		select {
		case <-supervisor.Exits():
		case <-time.After(5 * time.Second):
			t.Fatal("a forward on the failed host never failed")
		}
	}

	content, _ := os.ReadFile(calls)
	if connects := strings.Count(string(content), " -f -N "); connects != 1 {
		t.Errorf("%d connect attempts, want 1:\n%s", connects, content)
	}
}

// Quitting while the Listening fetch connects must not wait for the fetch's
// own deadline.
func TestStopAllEndsAHangingFetch(t *testing.T) {
	calls := fakeSSH(t, "exec sleep 30")
	supervisor := newSupervisor(t)
	go func() { _, _ = supervisor.Run(context.Background(), "tarpit", "true") }()
	pid, _ := strconv.Atoi(strings.Fields(waitForCalls(t, calls, 1)[0])[0])

	within(t, 2*time.Second, "StopAll", supervisor.StopAll)

	if err := syscall.Kill(pid, 0); err == nil {
		t.Errorf("ssh %d still runs after StopAll", pid)
	}
}

// ssh reports some errors, such as an invalid host name or a config error,
// before -E opens its log; the failure must still carry ssh's words.
func TestConnectFailureShowsWhatSSHPrintedBeforeItsLog(t *testing.T) {
	fakeSSH(t, "echo 'hostname contains invalid characters' >&2; exit 255")
	supervisor := newSupervisor(t)

	_, err := supervisor.Run(context.Background(), "bad", "true")

	if err == nil || err.Error() != "hostname contains invalid characters" {
		t.Fatalf("got %v", err)
	}
}
