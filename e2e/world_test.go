package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// world is one isolated lazyfwd installation: its binary, workspace, and run
// directory.
type world struct {
	t       *testing.T
	binary  string
	env     []string
	runBase string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "lazyfwd")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "github.com/glattziehen/lazyfwd")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}
	// Socket paths must stay short on Unix, so the run directory's parent
	// is under /tmp there.
	runBase, err := os.MkdirTemp(shortTempRoot(), "lzf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runBase) })
	config := filepath.Join(dir, "config")
	env := append(os.Environ(),
		// The workspace's directory on Linux, macOS, and Windows. ssh reads
		// its own config from the account's home, whatever HOME says.
		"XDG_CONFIG_HOME="+config,
		"HOME="+config,
		"APPDATA="+config,
		// The run directory's parent on Unix and Windows.
		"XDG_RUNTIME_DIR="+runBase,
		"TMP="+runBase,
		"TEMP="+runBase,
	)
	return &world{t: t, binary: binary, env: env, runBase: runBase}
}

// result is one CLI command's outcome.
type result struct {
	stdout, stderr string
	code           int
}

// lazyfwd runs a CLI command and waits for it.
func (w *world) lazyfwd(args ...string) result {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*waitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.binary, args...)
	cmd.Env = w.env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		w.t.Fatalf("lazyfwd %v: %v", args, err)
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: cmd.ProcessState.ExitCode()}
}

// mustLazyfwd runs a CLI command that must succeed.
func (w *world) mustLazyfwd(args ...string) string {
	w.t.Helper()
	r := w.lazyfwd(args...)
	if r.code != 0 {
		w.t.Fatalf("lazyfwd %v: exit %d\n%s", args, r.code, r.stderr)
	}
	return r.stdout
}

// addForward adds one stopped forward and returns its ID.
func (w *world) addForward(args ...string) string {
	w.t.Helper()
	var added []struct {
		ID int `json:"id"`
	}
	output := w.mustLazyfwd(append([]string{"--json", "forward", "add"}, args...)...)
	if err := json.Unmarshal([]byte(output), &added); err != nil || len(added) != 1 {
		w.t.Fatalf("forward add %v printed %q: %v", args, output, err)
	}
	return strconv.Itoa(added[0].ID)
}

// owner is a running `lazyfwd serve`.
type owner struct {
	cmd    *exec.Cmd
	exited chan struct{}
}

// serve starts `lazyfwd serve` and waits until it answers.
func (w *world) serve() *owner {
	w.t.Helper()
	cmd := exec.Command(w.binary, "serve")
	cmd.Env = w.env
	cmd.SysProcAttr = ownProcessGroup()
	stderr, err := cmd.StderrPipe()
	if err != nil {
		w.t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		w.t.Fatal(err)
	}
	o := &owner{cmd: cmd, exited: make(chan struct{})}
	serving := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			w.t.Logf("serve: %s", scanner.Text())
			if strings.Contains(scanner.Text(), "serving") {
				close(serving)
			}
		}
		_ = cmd.Wait()
		close(o.exited)
	}()
	select {
	case <-serving:
	case <-o.exited:
		w.t.Fatal("lazyfwd serve exited before it served")
	case <-time.After(waitTimeout):
		_ = cmd.Process.Kill()
		w.t.Fatal("lazyfwd serve did not start serving")
	}
	w.t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-o.exited
	})
	return o
}

// quit stops the owner the way ctrl+c does and waits for it to exit.
func (w *world) quit(o *owner) {
	w.t.Helper()
	if err := interrupt(o.cmd.Process); err != nil {
		w.t.Fatalf("interrupt lazyfwd serve: %v", err)
	}
	select {
	case <-o.exited:
	case <-time.After(waitTimeout):
		w.t.Fatal("lazyfwd serve did not exit after ctrl+c")
	}
	if code := o.cmd.ProcessState.ExitCode(); code != 0 {
		w.t.Errorf("lazyfwd serve exited with %d after ctrl+c", code)
	}
}

// crash kills the owner without letting it clean up.
func (w *world) crash(o *owner) {
	w.t.Helper()
	if err := o.cmd.Process.Kill(); err != nil {
		w.t.Fatal(err)
	}
	<-o.exited
}

// eventually fails the test unless check returns "" within waitTimeout; a
// non-empty string says what is still wrong.
func eventually(t *testing.T, what string, check func() string) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for {
		problem := check()
		if problem == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %s", what, problem)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// assertNothingLeft fails unless no ssh of this world runs and nothing
// listens on the ports any more.
func (w *world) assertNothingLeft(ports ...int) {
	w.t.Helper()
	eventually(w.t, "after lazyfwd ended", func() string {
		if processes := leftoverSSH(w.runBase); len(processes) > 0 {
			return fmt.Sprintf("ssh still runs: %v", processes)
		}
		for _, port := range ports {
			if isListening(port) {
				return fmt.Sprintf("port %d still listens", port)
			}
		}
		return ""
	})
}
