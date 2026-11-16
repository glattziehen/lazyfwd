// Package e2e runs the lazyfwd binary against a real sshd: `lazyfwd serve`
// owns the connections, and CLI commands add, start, edit, and stop forwards
// that carry real traffic. CI provides the sshd and ssh config aliases through
// the variables below; without them the test skips.
//
//   - LAZYFWD_E2E_HOST: a host ssh reaches without prompts, on this machine's
//     network, so that it reaches the test's servers on 127.0.0.1 too.
//   - LAZYFWD_E2E_SSHD_PORT: the port its sshd listens on there, which the
//     port list must show. Optional: WSL 1 lists no listening sockets.
//   - LAZYFWD_E2E_CONFIG_FORWARD_HOST: the same host under an alias whose ssh
//     config has a LocalForward line.
//   - LAZYFWD_E2E_DENIED_HOST: the same host under an alias that ssh cannot
//     authenticate without a prompt.
package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// greeting is what the test's HTTP server answers.
const greeting = "lazyfwd-e2e"

// waitTimeout bounds every wait for lazyfwd or ssh.
const waitTimeout = 30 * time.Second

// Exit codes of the CLI, as `lazyfwd --help` documents them.
const (
	exitFailed     = 1
	exitNeedsLogin = 4
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

// isListening reports whether something accepts connections on port on the
// loopback, IPv4 or IPv6. It connects rather than binds: right after a
// forward stops, its connections' TIME_WAIT makes a bind fail on macOS.
func isListening(port int) bool {
	for _, host := range []string{"127.0.0.1", "::1"} {
		// Windows takes about 2 s to refuse a connect to a closed port.
		connection, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 5*time.Second)
		if err == nil {
			_ = connection.Close()
			return true
		}
	}
	return false
}

// serveHTTP answers greeting on a free port of 127.0.0.1.
func serveHTTP(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, greeting) }),
		ReadHeaderTimeout: waitTimeout,
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return listener.Addr().(*net.TCPAddr).Port
}

// freePort is a port that nothing listens on right now.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

// get fetches / through client and returns the body, or what went wrong.
func get(client *http.Client, url string) (string, error) {
	response, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	return string(body), err
}

// assertGreets fails unless greeting comes back through 127.0.0.1:port.
func assertGreets(t *testing.T, port int) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	body, err := get(client, fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil || body != greeting {
		t.Fatalf("through port %d: %q, %v", port, body, err)
	}
}

// remoteGreets asks host, over the test's own ssh, to fetch / through the
// unix socket path there.
func remoteGreets(t *testing.T, host, path string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ControlMaster=no", "-o", "ControlPath=none", "--", host, "curl -s --max-time 5 --unix-socket "+path+" http://lazyfwd/").CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%v: %s", err, output)
	}
	return string(output)
}

// configForwardPort is the local port of host's LocalForward, from ssh -G.
func configForwardPort(t *testing.T, host string) int {
	t.Helper()
	output, err := exec.Command("ssh", "-G", "--", host).Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "localforward "); ok {
			listen, _, _ := strings.Cut(rest, " ")
			port, err := strconv.Atoi(listen[strings.LastIndex(listen, ":")+1:])
			if err != nil {
				t.Fatalf("localforward %q", rest)
			}
			return port
		}
	}
	t.Fatalf("ssh -G %s shows no localforward", host)
	return 0
}

// listedPort is a port in `lazyfwd port list --json`.
type listedPort struct {
	Port int `json:"port"`
}

func requireEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Skipf("%s is not set; CI provides the sshd this test needs", name)
	}
	return value
}

func TestForwardsCarryTrafficAndEndWithTheOwner(t *testing.T) {
	host := requireEnv(t, "LAZYFWD_E2E_HOST")
	sshdPort := os.Getenv("LAZYFWD_E2E_SSHD_PORT")
	configForwardHost := requireEnv(t, "LAZYFWD_E2E_CONFIG_FORWARD_HOST")
	deniedHost := requireEnv(t, "LAZYFWD_E2E_DENIED_HOST")
	w := newWorld(t)
	target := serveHTTP(t)
	targetEnd := "127.0.0.1:" + strconv.Itoa(target)
	isWindows := runtime.GOOS == "windows"

	o := w.serve()
	w.mustLazyfwd("host", "add", host)

	// A local forward carries traffic, and stops.
	localPort := freePort(t)
	local := w.addForward(host, targetEnd, "--listen", strconv.Itoa(localPort))
	w.mustLazyfwd("forward", "start", local)
	assertGreets(t, localPort)
	w.mustLazyfwd("forward", "stop", local)
	eventually(t, "stopped local forward", func() string {
		if isListening(localPort) {
			return fmt.Sprintf("port %d still listens", localPort)
		}
		return ""
	})

	// Editing a running forward moves it to its new port.
	w.mustLazyfwd("forward", "start", local)
	editedPort := freePort(t)
	w.mustLazyfwd("forward", "edit", local, "--listen", strconv.Itoa(editedPort))
	eventually(t, "edited local forward", func() string {
		if _, err := get(&http.Client{Timeout: 2 * time.Second}, fmt.Sprintf("http://127.0.0.1:%d/", editedPort)); err != nil {
			return err.Error()
		}
		if isListening(localPort) {
			return fmt.Sprintf("old port %d still listens", localPort)
		}
		return ""
	})
	assertGreets(t, editedPort)

	// A remote forward listens on the host; its sshd shares this machine's
	// network, so the test reaches it on 127.0.0.1.
	remotePort := freePort(t)
	remote := w.addForward(host, targetEnd, "--direction", "remote", "--listen", strconv.Itoa(remotePort))
	w.mustLazyfwd("forward", "start", remote)
	assertGreets(t, remotePort)

	// A remote forward whose port another program holds on the host's
	// loopback is refused before ssh starts.
	taken := w.addForward(host, targetEnd, "--direction", "remote", "--listen", strconv.Itoa(target))
	if r := w.lazyfwd("forward", "start", taken); r.code != exitFailed || !strings.Contains(r.stderr, "already in use") {
		t.Errorf("remote forward on a taken port: exit %d, %q", r.code, r.stderr)
	}

	// A remote forward on a unix socket on the host, started twice: sshd
	// leaves the socket file behind, and lazyfwd removes it.
	socketPath := fmt.Sprintf("/tmp/lazyfwd-e2e-%d.sock", remotePort)
	remoteSocket := w.addForward(host, targetEnd, "--direction", "remote", "--listen", socketPath)
	for range 2 {
		w.mustLazyfwd("forward", "start", remoteSocket)
		if got := remoteGreets(t, host, socketPath); got != greeting {
			t.Fatalf("through %s on %s: %q", socketPath, host, got)
		}
		w.mustLazyfwd("forward", "stop", remoteSocket)
	}

	// The listening-ports fetch reaches the host and sees its sshd.
	var listing struct {
		Ports []listedPort `json:"ports"`
	}
	output := w.mustLazyfwd("--json", "port", "list", host)
	if err := json.Unmarshal([]byte(output), &listing); err != nil {
		t.Fatalf("port list printed %q: %v", output, err)
	}
	if sshdPort != "" && !slices.ContainsFunc(listing.Ports, func(p listedPort) bool { return strconv.Itoa(p.Port) == sshdPort }) {
		t.Errorf("port list %s misses sshd's port %s:\n%s", host, sshdPort, output)
	}

	// Forwards that Windows refuses, and what the same forwards do on Unix.
	localSocket := w.addForward(host, targetEnd, "--listen", filepath.ToSlash(filepath.Join("/tmp", fmt.Sprintf("lazyfwd-e2e-local-%d.sock", localPort))))
	r := w.lazyfwd("forward", "start", localSocket)
	if isWindows && (r.code != exitFailed || !strings.Contains(r.stderr, "unix sockets on this machine are not supported on Windows")) {
		t.Errorf("local socket on Windows: exit %d, %q", r.code, r.stderr)
	}
	if !isWindows && r.code != 0 {
		t.Errorf("local socket: exit %d, %q", r.code, r.stderr)
	}

	w.mustLazyfwd("host", "add", configForwardHost)
	configPort := configForwardPort(t, configForwardHost)
	configForward := w.addForward(configForwardHost, targetEnd, "--listen", strconv.Itoa(freePort(t)))
	r = w.lazyfwd("forward", "start", configForward)
	if isWindows && (r.code != exitFailed || !strings.Contains(r.stderr, "forwards ports")) {
		t.Errorf("host whose config forwards ports on Windows: exit %d, %q", r.code, r.stderr)
	}
	if !isWindows && r.code != 0 {
		t.Errorf("host whose config forwards ports: exit %d, %q", r.code, r.stderr)
	}
	if isListening(configPort) {
		t.Errorf("the config's LocalForward port %d listens", configPort)
	}

	w.mustLazyfwd("host", "add", deniedHost)
	denied := w.addForward(deniedHost, targetEnd, "--listen", strconv.Itoa(freePort(t)))
	r = w.lazyfwd("forward", "start", denied)
	if isWindows && (r.code != exitFailed || !strings.Contains(r.stderr, "on Windows lazyfwd needs a key")) {
		t.Errorf("host that needs a prompt on Windows: exit %d, %q", r.code, r.stderr)
	}
	if !isWindows && r.code != exitNeedsLogin {
		t.Errorf("host that needs a prompt: exit %d, %q", r.code, r.stderr)
	}

	// Quitting ends every forward.
	w.quit(o)
	w.assertNothingLeft(editedPort, remotePort)

	// A crashed owner leaves nothing behind either: on Windows its job
	// objects end the ssh processes at once; on Unix the masters outlive it,
	// and the next lazyfwd closes them.
	o = w.serve()
	w.mustLazyfwd("forward", "start", local, remote)
	assertGreets(t, editedPort)
	assertGreets(t, remotePort)
	w.crash(o)
	if isWindows {
		w.assertNothingLeft(editedPort, remotePort)
		return
	}
	o = w.serve()
	w.assertNothingLeft(editedPort, remotePort)
	w.quit(o)
	w.assertNothingLeft()
}
