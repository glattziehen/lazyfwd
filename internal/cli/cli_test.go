package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glattziehen/lazyfwd/internal/control"
	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/owner/ownertest"
	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// newEnv has its own workspace and no ssh config hosts.
func newEnv(t *testing.T) env {
	t.Helper()
	dir := t.TempDir()
	return env{
		stdin:          strings.NewReader(""),
		storePath:      filepath.Join(dir, "workspace.json"),
		sshConfigPaths: []string{filepath.Join(dir, "missing_ssh_config")},
	}
}

type result struct {
	stdout, stderr string
	code           int
}

func runCLI(e env, args ...string) result {
	var stdout, stderr bytes.Buffer
	e.stdout, e.stderr = &stdout, &stderr
	code := run(args, "test", e)
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func mustRun(t *testing.T, e env, args ...string) result {
	t.Helper()
	r := runCLI(e, args...)
	if r.code != exitOK {
		t.Fatalf("lazyfwd %s: exit %d\n%s", strings.Join(args, " "), r.code, r.stderr)
	}
	return r
}

// startOwner runs an owner of e's workspace on fake tunnels, as the TUI or
// serve does.
func startOwner(t *testing.T, e env) (*owner.Owner, *ownertest.Tunnels) {
	t.Helper()
	store := workspace.FileStore{Path: e.storePath}
	lock, err := control.Claim(store)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	runDir, err := os.MkdirTemp("", "lazyfwd") // short enough for a socket path
	if err != nil {
		t.Fatal(err)
	}
	tunnels := ownertest.NewTunnels()
	o := owner.New(store, tunnels, ws, nil)
	server, err := control.Serve(o, lock, runDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		o.Close()
		server.Close()
		lock.Release()
		_ = os.RemoveAll(runDir)
	})
	return o, tunnels
}

// assertGolden compares output with testdata/name, the documented shape.
func assertGolden(t *testing.T, name, output string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if output != string(want) {
		t.Errorf("%s differs:\n got:\n%s\nwant:\n%s", name, output, want)
	}
}

func TestUsageErrorsExitWithTwo(t *testing.T) {
	e := newEnv(t)
	for _, args := range [][]string{
		{"forwrd", "list"},
		{"forward", "start", "three"},
		{"forward", "add", "box"},
		{"forward", "add", "box", "4966", "--direction", "sideways"},
		{"forward", "list", "--colour"},
		{"forward", "edit", "1"},
	} {
		if r := runCLI(e, args...); r.code != exitUsage {
			t.Errorf("lazyfwd %s: exit %d, want %d\n%s", strings.Join(args, " "), r.code, exitUsage, r.stderr)
		}
	}
}

// Saved state changes without an owner; anything needing a connection
// exits with 3 and names the commands that start an owner.
func TestWithoutOwnerSavedStateChangesAndConnectionsExitWithThree(t *testing.T) {
	e := newEnv(t)
	mustRun(t, e, "host", "add", "box")
	mustRun(t, e, "forward", "add", "box", "4966", "--listen", "14966")
	mustRun(t, e, "forward", "add", "box", "/var/run/docker.sock", "--listen", "/tmp/box-docker.sock")
	mustRun(t, e, "forward", "add", "box", "3000", "--direction", "remote")

	for _, args := range [][]string{
		{"forward", "start", "1"},
		{"forward", "stop", "1"},
		{"forward", "toggle", "1"},
		{"forward", "add", "box", "8080", "--start"},
		{"port", "list", "box"},
	} {
		r := runCLI(e, args...)
		if r.code != exitNoOwner || !strings.Contains(r.stderr, "lazyfwd serve") {
			t.Errorf("lazyfwd %s: exit %d, want %d\n%s", strings.Join(args, " "), r.code, exitNoOwner, r.stderr)
		}
	}
	assertGolden(t, "forward_list.json", mustRun(t, e, "forward", "list", "--json").stdout)
	assertGolden(t, "forward_list.txt", mustRun(t, e, "forward", "list").stdout)
	assertGolden(t, "host_list.json", mustRun(t, e, "host", "list", "--json").stdout)
	if ws, _ := (workspace.FileStore{Path: e.storePath}).Load(); len(ws.Forwards) != 3 {
		t.Errorf("saved %d forwards, want the 3 added", len(ws.Forwards))
	}
}

// A command reaches the running owner, so the TUI, which shows the owner's
// state, sees the change at once.
func TestCommandsChangeTheRunningOwner(t *testing.T) {
	e := newEnv(t)
	o, tunnels := startOwner(t, e)
	tunnels.SetOutcome("box", tunnel.Status{State: tunnel.Running})

	r := mustRun(t, e, "forward", "add", "box", "4966", "--listen", "14966", "--start", "--json")

	assertGolden(t, "forward_add_start.json", r.stdout)
	if forwards := o.State().Forwards; len(forwards) != 1 || forwards[0].Status.State != tunnel.Running {
		t.Fatalf("owner has %+v, want one running forward", forwards)
	}
}

func TestStartExitCodeTellsWhatHappened(t *testing.T) {
	cases := []struct {
		outcome  tunnel.Status
		wantCode int
		wantText string
	}{
		{tunnel.Status{State: tunnel.Running}, exitOK, ""},
		{tunnel.Status{State: tunnel.Failed, Reason: "localhost:14966 is already in use"}, exitFailed, "forward 1 failed: localhost:14966 is already in use"},
		{tunnel.Status{State: tunnel.Failed, Reason: "Permission denied (password).", NeedsLogin: true}, exitNeedsLogin, "lazyfwd host login box"},
	}
	for _, c := range cases {
		e := newEnv(t)
		_, tunnels := startOwner(t, e)
		tunnels.SetOutcome("box", c.outcome)
		mustRun(t, e, "forward", "add", "box", "4966", "--listen", "14966")

		r := runCLI(e, "forward", "start", "1")

		if r.code != c.wantCode || !strings.Contains(r.stderr, c.wantText) {
			t.Errorf("%s: exit %d, stderr %q; want %d and %q", c.outcome.State, r.code, r.stderr, c.wantCode, c.wantText)
		}
	}
}

// A wait that ends before the connection is a distinct exit, so a script can
// tell "still trying" from "failed".
func TestStartOutcomeOfAForwardStillStarting(t *testing.T) {
	forwards := []owner.ForwardState{{Forward: workspace.Forward{ID: 3}, Status: tunnel.Status{State: tunnel.Starting}}}

	if code := exitCode(startOutcome(forwards, true)); code != exitStillStarting {
		t.Errorf("after a wait: exit %d, want %d", code, exitStillStarting)
	}
	if err := startOutcome(forwards, false); err != nil {
		t.Errorf("with --no-wait: %v, want success", err)
	}
}

func TestPortListShowsYourPortsAndTheirForwards(t *testing.T) {
	e := newEnv(t)
	_, tunnels := startOwner(t, e)
	tunnels.SetRunOutput(`lazyfwd:ss
LISTEN 0 128 127.0.0.1:4966 0.0.0.0:* users:(("diffle",pid=7,fd=9))
LISTEN 0 128 0.0.0.0:5432 0.0.0.0:*
LISTEN 0 128 127.0.0.1:8080 0.0.0.0:* users:(("node",pid=8,fd=3))
`)
	mustRun(t, e, "forward", "add", "box", "4966", "--listen", "14966")

	assertGolden(t, "port_list.json", mustRun(t, e, "port", "list", "box", "--json").stdout)
	assertGolden(t, "port_list.txt", mustRun(t, e, "port", "list", "box").stdout)
}
