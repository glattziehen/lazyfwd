package tunnel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// On Windows a host's connection is one ssh process per forward, since the
// ssh that Windows ships can neither share a connection (ControlMaster) nor
// move to the background (-f). Each process runs in its own job object, so
// no ssh outlives lazyfwd, and each forward's run is one goroutine that owns
// its process from the checks before the start to the exit.

const (
	// readyCheckInterval is how often a starting local forward looks for its
	// listener.
	readyCheckInterval = 50 * time.Millisecond
	// configTimeout bounds ssh -G, which reads only local files.
	configTimeout = 5 * time.Second
	// waitDelay bounds the wait for ssh's output after it exits.
	waitDelay = time.Second
)

// remoteForwardReady is what ssh -v logs once the server has confirmed every
// remote forward; each process holds one.
const remoteForwardReady = "forwarding_success: all expected forwarding replies received"

// windowsLoginHint ends the reason of a failed authentication: on Windows an
// interactive login cannot help, since each forward connects on its own.
const windowsLoginHint = "on Windows lazyfwd needs a key that ssh uses without a prompt, such as one in ssh-agent; hosts that ask for a password, a passphrase, or 2FA on every connect are not supported"

// connection runs the forwards of one host.
type connection struct {
	host       string
	supervisor *Supervisor

	mu sync.Mutex
	// runs holds each forward's current run.
	runs map[workspace.ForwardID]*forwardRun
	// stopping holds the runs stopped and maybe not yet ended; a later start
	// on the host waits for them, so a stopped forward's port is free again
	// before an edit or another forward binds it.
	stopping []*forwardRun
	isClosed bool
}

// forwardRun is one start of a forward, until Stop or its failure.
type forwardRun struct {
	cancel context.CancelFunc
	// done closes once the run has ended and its ssh has exited.
	done chan struct{}
}

func newConnection(supervisor *Supervisor, host, _ string) *connection {
	return &connection{host: host, supervisor: supervisor, runs: map[workspace.ForwardID]*forwardRun{}}
}

// startForward starts the forward's run and returns at once.
func (c *connection) startForward(ctx context.Context, forward workspace.Forward, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isClosed {
		return
	}
	c.stopping = withoutEnded(c.stopping)
	previous := append([]*forwardRun(nil), c.stopping...)
	if earlier, ok := c.runs[forward.ID]; ok {
		previous = append(previous, earlier)
	}
	runCtx, cancel := context.WithCancel(ctx)
	run := &forwardRun{cancel: cancel, done: make(chan struct{})}
	c.runs[forward.ID] = run
	go c.run(runCtx, run, forward, generation, previous)
}

// stopForward ends the forward's run and returns at once.
func (c *connection) stopForward(id workspace.ForwardID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	run, ok := c.runs[id]
	if !ok {
		return
	}
	delete(c.runs, id)
	c.stopping = append(c.stopping, run)
	run.cancel()
}

// close ends every run and waits until their ssh processes have exited.
func (c *connection) close() {
	c.mu.Lock()
	c.isClosed = true
	runs := c.stopping
	for _, run := range c.runs {
		runs = append(runs, run)
	}
	c.mu.Unlock()
	for _, run := range runs {
		run.cancel()
	}
	for _, run := range runs {
		<-run.done
	}
}

// withoutEnded drops the runs that have ended.
func withoutEnded(runs []*forwardRun) []*forwardRun {
	kept := runs[:0]
	for _, run := range runs {
		select {
		case <-run.done:
		default:
			kept = append(kept, run)
		}
	}
	return kept
}

// run starts the forward once the runs it must follow have ended, checks it,
// and keeps its ssh running until ctx ends or ssh exits.
func (c *connection) run(ctx context.Context, run *forwardRun, forward workspace.Forward, generation uint64, previous []*forwardRun) {
	defer func() {
		c.mu.Lock()
		if c.runs[forward.ID] == run {
			delete(c.runs, forward.ID)
		}
		c.mu.Unlock()
		close(run.done)
	}()
	for _, earlier := range previous {
		select {
		case <-earlier.done:
		case <-ctx.Done():
			return
		}
	}
	if err := c.check(ctx, forward); err != nil {
		if ctx.Err() == nil {
			c.supervisor.fail(forward.ID, generation, err.Error(), false)
		}
		return
	}
	for attempt := 0; ; attempt++ {
		result := c.runSSH(ctx, forward, generation)
		// sshd leaves a remote forward's socket file behind and refuses to
		// bind over it; remove it once if nothing accepts there.
		if !result.isStaleSocket || attempt > 0 || !c.removeStaleRemoteSocket(ctx, forward) {
			return
		}
	}
}

// check refuses what cannot work on Windows, and readies the forward's
// listening end or says why it cannot listen there.
func (c *connection) check(ctx context.Context, forward workspace.Forward) error {
	if forward.LocalEnd().IsSocket() {
		return errors.New("unix sockets on this machine are not supported on Windows; use a TCP port as the local end")
	}
	if err := c.checkConfigForwards(ctx); err != nil {
		return err
	}
	switch {
	case forward.Direction == workspace.Local:
		return checkLocalPort(forward.Bind.Port)
	case !forward.Bind.IsSocket():
		return c.checkRemotePort(ctx, forward.Bind.Port)
	}
	return nil
}

// checkConfigForwards refuses a host whose ssh config forwards ports. Each
// lazyfwd forward is its own ssh process, which would open those forwards
// too, and ClearAllForwardings would clear the forward's own -L or -R.
func (c *connection) checkConfigForwards(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, configTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ssh", configArgs(c.host)...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if reason := lastSignificantLine(stderr.String()); reason != "" {
			return errors.New(reason)
		}
		return fmt.Errorf("reading the ssh config of %s: %w", c.host, err)
	}
	var forwards []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		keyword, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch keyword {
		case "localforward", "remoteforward", "dynamicforward":
			forwards = append(forwards, strings.TrimSpace(line))
		}
	}
	if len(forwards) > 0 {
		return fmt.Errorf("the ssh config of %s forwards ports (%s); on Windows lazyfwd cannot start forwards beside those, so remove them or reach the host through a Host alias without them", c.host, strings.Join(forwards, ", "))
	}
	return nil
}

// checkRemotePort reports an error naming the port when something listens on
// it on the host's loopback. Like ssh on the local machine, sshd reports
// success when it binds localhost on only one of IPv4 and IPv6.
func (c *connection) checkRemotePort(ctx context.Context, port int) error {
	for _, host := range loopbackHosts {
		if c.isRemoteListening(ctx, net.JoinHostPort(host, strconv.Itoa(port))) {
			return fmt.Errorf("localhost:%d on %s is already in use", port, c.host)
		}
	}
	return nil
}

// isRemoteListening reports whether something on the host accepts on target,
// a socket path or host:port, over a connection of its own. A server that
// holds the probe's connection open counts as listening; a connect that does
// not finish does not, and the forward's own ssh then reports why.
func (c *connection) isRemoteListening(ctx context.Context, target string) bool {
	ctx, cancel := context.WithTimeout(ctx, masterOpenTimeout)
	defer cancel()
	log := newSSHLog("Authenticated to ")
	cmd := exec.Command("ssh", windowsProbeArgs(c.host, target)...)
	cmd.Stderr = log
	cmd.WaitDelay = waitDelay
	process, err := startProcess(cmd)
	if err != nil {
		return false
	}
	defer process.kill()
	authenticated := log.marked
	var held <-chan time.Time
	for {
		select {
		case <-process.exited:
			return process.err == nil
		case <-authenticated:
			authenticated = nil
			held = time.After(probeTimeout)
		case <-held:
			return true
		case <-ctx.Done():
			return false
		}
	}
}

// removeStaleRemoteSocket removes a remote forward's socket path on the host
// when nothing accepts on it. It reports whether it removed one.
func (c *connection) removeStaleRemoteSocket(ctx context.Context, forward workspace.Forward) bool {
	if c.isRemoteListening(ctx, forward.Bind.Path) {
		return false
	}
	command, err := removeSocketCommand(forward.Bind.Path)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, controlTimeout)
	defer cancel()
	_, err = c.runCommand(ctx, command)
	return err == nil
}

// sshResult is how a forward's ssh ended.
type sshResult struct {
	// isStaleSocket is set when ssh exited because sshd refused a remote
	// socket path, which a socket from an earlier run may hold.
	isStaleSocket bool
}

// runSSH runs the forward's ssh until ctx ends or ssh exits, and reports to
// the supervisor when the forward runs and when it fails.
func (c *connection) runSSH(ctx context.Context, forward workspace.Forward, generation uint64) sshResult {
	log := newSSHLog(remoteForwardReady)
	cmd := exec.Command("ssh", forwardProcessArgs(forward)...)
	cmd.Stderr = log
	cmd.WaitDelay = waitDelay
	process, err := startProcess(cmd)
	if err != nil {
		c.supervisor.fail(forward.ID, generation, err.Error(), false)
		return sshResult{}
	}
	defer process.kill()

	deadline := time.NewTimer(masterOpenTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(readyCheckInterval)
	defer ticker.Stop()
	for isReady := false; !isReady; {
		select {
		case <-ctx.Done():
			return sshResult{}
		case <-process.exited:
			if forward.Direction == workspace.Remote && forward.Bind.IsSocket() && strings.Contains(log.String(), "remote port forwarding failed for listen path") {
				return sshResult{isStaleSocket: true}
			}
			reason, needsLogin := explainFailure(log.String(), process.err)
			c.supervisor.fail(forward.ID, generation, reason, needsLogin)
			return sshResult{}
		case <-deadline.C:
			process.kill()
			c.supervisor.fail(forward.ID, generation, fmt.Sprintf("%s did not finish connecting within %s", c.host, masterOpenTimeout), false)
			return sshResult{}
		case <-log.marked:
			isReady = forward.Direction == workspace.Remote
		case <-ticker.C:
			isReady = forward.Direction == workspace.Local && isListeningBy(cmd.Process.Pid, forward.Bind.Port)
		}
	}
	c.supervisor.establish(forward.ID, generation)
	select {
	case <-ctx.Done():
	case <-process.exited:
		reason, _ := explainFailure(log.String(), process.err)
		c.supervisor.fail(forward.ID, generation, "connection to "+c.host+" closed: "+reason, false)
	}
	return sshResult{}
}

// runCommand runs command on the host over a connection of its own;
// Supervisor.Run documents the result.
func (c *connection) runCommand(ctx context.Context, command string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("ssh", commandArgs(c.host, command)...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = waitDelay
	process, err := startProcess(cmd)
	if err != nil {
		return "", err
	}
	select {
	case <-process.exited:
		err = process.err
	case <-ctx.Done():
		process.kill()
		return stdout.String(), ctx.Err()
	}
	// ssh exits 255 for its own errors; other statuses are the command's.
	var exitErr *exec.ExitError
	if err == nil || (errors.As(err, &exitErr) && exitErr.ExitCode() != 255) {
		return stdout.String(), err
	}
	reason, needsLogin := explainFailure(stderr.String(), err)
	return stdout.String(), &ConnectError{Host: c.host, Reason: reason, Log: stderr.String(), NeedsLogin: needsLogin}
}

// explainFailure is the reason in a failed ssh's log, and whether an
// interactive login would let ssh connect. On Windows that login only
// accepts a host key.
func explainFailure(log string, err error) (string, bool) {
	reason := lastSignificantLine(log)
	if reason == "" && err != nil {
		reason = err.Error()
	}
	if strings.Contains(log, "Permission denied") {
		return strings.TrimSuffix(reason, ".") + "; " + windowsLoginHint, false
	}
	return reason, strings.Contains(log, "Host key verification failed")
}

// lastSignificantLine is the last line of an ssh log that is not -v's
// debugging or transfer statistics.
func lastSignificantLine(log string) string {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "debug") || strings.HasPrefix(line, "Transferred:") || strings.HasPrefix(line, "Bytes per second:") {
			continue
		}
		return line
	}
	return ""
}

// sshLog collects what ssh writes to stderr, keeping the last lines that
// matter, and closes marked once a line holds the marker. ssh -v logs a line
// per connection through a forward, so it keeps no more than it needs.
type sshLog struct {
	marker string
	marked chan struct{}

	mu       sync.Mutex
	partial  []byte
	lines    []string
	isMarked bool
}

// sshLogLines bounds the lines an sshLog keeps.
const sshLogLines = 50

func newSSHLog(marker string) *sshLog {
	return &sshLog{marker: marker, marked: make(chan struct{})}
}

func (l *sshLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial = append(l.partial, p...)
	for {
		i := bytes.IndexByte(l.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(l.partial[:i]), "\r")
		l.partial = l.partial[i+1:]
		if !l.isMarked && strings.Contains(line, l.marker) {
			l.isMarked = true
			close(l.marked)
		}
		if strings.HasPrefix(line, "debug") {
			continue
		}
		l.lines = append(l.lines, line)
		if len(l.lines) > sshLogLines {
			l.lines = l.lines[1:]
		}
	}
	return len(p), nil
}

// String is the kept lines, without -v's debugging.
func (l *sshLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(append(l.lines, string(l.partial)), "\n")
}
