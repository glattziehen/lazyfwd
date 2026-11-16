package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/glattziehen/lazyfwd/internal/workspace"
)

const (
	// masterCheckInterval is how often a connection looks for its master's death.
	masterCheckInterval = 2 * time.Second
	// probeTimeout bounds the check whether something listens on the host.
	probeTimeout = 5 * time.Second
	// masterOpenTimeout bounds opening a master without prompts. ssh's
	// ConnectTimeout stops counting once the server sends its banner, so a
	// server that stalls after that would hang the connect for good.
	masterOpenTimeout = 20 * time.Second
	// controlTimeout bounds a control command to the master.
	controlTimeout = 10 * time.Second
)

// ConnectError reports that a host's connection could not open.
type ConnectError struct {
	Host string
	// Reason is the last line of ssh's log, or ssh's exit error.
	Reason string
	// Log is everything ssh logged.
	Log string
	// NeedsLogin is set when an interactive login would get further.
	NeedsLogin bool
}

func (e *ConnectError) Error() string {
	return e.Reason
}

// established is a forward that the master holds.
type established struct {
	forward    workspace.Forward
	generation uint64
}

// connection owns one host's master and the forwards on it. Its run loop is
// the only goroutine that touches its fields, so operations on one host
// happen in the order they were sent: an edit's cancel precedes its new
// forward. The queue has no bound, so sending an operation never waits,
// however slow the host.
type connection struct {
	host       string
	socketPath string
	logPath    string
	supervisor *Supervisor
	// mu guards queue; wake tells the run loop that queue grew.
	mu    sync.Mutex
	queue []func()
	wake  chan struct{}
	// quit asks the run loop to close the master; closed reports it did.
	quit   chan struct{}
	closed chan struct{}

	// masterPID is 0 while no master is known.
	masterPID   int
	established map[workspace.ForwardID]established
	// failedOpen is the last failed attempt to open the master; operations
	// queued before it ended share its error instead of trying again.
	failedOpen   error
	failedOpenAt time.Time
}

func newConnection(supervisor *Supervisor, host, socketPath string) *connection {
	c := &connection{
		host:        host,
		socketPath:  socketPath,
		logPath:     socketPath + logExt,
		supervisor:  supervisor,
		wake:        make(chan struct{}, 1),
		quit:        make(chan struct{}),
		closed:      make(chan struct{}),
		established: map[workspace.ForwardID]established{},
	}
	go c.run()
	return c
}

// enqueue adds an operation for the run loop without waiting.
func (c *connection) enqueue(operation func()) {
	c.mu.Lock()
	c.queue = append(c.queue, operation)
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default: // a wake-up is already pending
	}
}

func (c *connection) next() (func(), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.queue) == 0 {
		return nil, false
	}
	operation := c.queue[0]
	c.queue = c.queue[1:]
	return operation, true
}

func (c *connection) run() {
	ticker := time.NewTicker(masterCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.quit:
			c.closeMaster()
			close(c.closed)
			return
		default:
		}
		if operation, ok := c.next(); ok {
			operation()
			continue
		}
		select {
		case <-c.wake:
		case <-ticker.C:
			c.noticeMasterDeath()
		case <-c.quit:
		}
	}
}

// close ends the master and waits for it; queued operations are dropped.
func (c *connection) close() {
	close(c.quit)
	<-c.closed
}

// start puts the forward on the master, opening the master first if needed,
// unless the forward was stopped or restarted since generation began.
func (c *connection) start(ctx context.Context, forward workspace.Forward, generation uint64, queuedAt time.Time) {
	if !c.supervisor.isCurrent(forward.ID, generation) {
		return
	}
	if err := c.ensureMaster(ctx, queuedAt); err != nil {
		var connectErr *ConnectError
		if errors.As(err, &connectErr) {
			c.supervisor.fail(forward.ID, generation, connectErr.Reason, connectErr.NeedsLogin)
		} else {
			c.supervisor.fail(forward.ID, generation, err.Error(), false)
		}
		return
	}
	if !c.supervisor.isCurrent(forward.ID, generation) {
		return
	}
	if err := c.checkBind(ctx, forward); err != nil {
		c.supervisor.fail(forward.ID, generation, err.Error(), false)
		return
	}
	output, err := runControl(ctx, forwardArgs(c.socketPath, forward))
	if err != nil && c.removeStaleRemoteSocket(ctx, forward) {
		output, err = runControl(ctx, forwardArgs(c.socketPath, forward))
	}
	if err != nil {
		c.supervisor.fail(forward.ID, generation, controlFailure(string(output)), false)
		return
	}
	c.established[forward.ID] = established{forward: forward, generation: generation}
}

// checkBind readies the forward's listening end, or says why it cannot
// listen there. A remote socket left from an earlier run is removed only
// after ssh refuses it, in removeStaleRemoteSocket.
func (c *connection) checkBind(ctx context.Context, forward workspace.Forward) error {
	switch {
	case forward.Direction == workspace.Local && forward.Bind.IsSocket():
		return freeLocalSocket(forward.Bind.Path)
	case forward.Direction == workspace.Local:
		return checkLocalPort(forward.Bind.Port)
	case !forward.Bind.IsSocket():
		return c.checkRemotePort(ctx, forward.Bind.Port)
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
// a socket path or host:port. A server that holds the probe's connection
// open counts as listening.
func (c *connection) isRemoteListening(ctx context.Context, target string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	err := exec.CommandContext(probeCtx, "ssh", probeArgs(c.socketPath, c.host, target)...).Run()
	return err == nil || probeCtx.Err() != nil
}

// removeStaleRemoteSocket removes a remote forward's socket path on the host
// when nothing accepts on it. sshd leaves the socket file behind when a
// forward stops and, unless the server sets StreamLocalBindUnlink, refuses to
// bind over it. It reports whether it removed one.
func (c *connection) removeStaleRemoteSocket(ctx context.Context, forward workspace.Forward) bool {
	if forward.Direction != workspace.Remote || !forward.Bind.IsSocket() {
		return false
	}
	if c.isRemoteListening(ctx, forward.Bind.Path) {
		return false
	}
	command, err := removeSocketCommand(forward.Bind.Path)
	if err != nil {
		return false
	}
	_, err = runControl(ctx, remoteCommandArgs(c.socketPath, c.host, command))
	return err == nil
}

// stop takes the forward off the master.
func (c *connection) stop(id workspace.ForwardID) {
	entry, ok := c.established[id]
	if !ok {
		return
	}
	delete(c.established, id)
	_, _ = runControl(c.supervisor.ctx, cancelArgs(c.socketPath, entry.forward))
}

// ensureMaster makes sure a live master serves the socket: the known one, one
// an interactive login left there, or a new one opened without prompts. An
// operation queued at queuedAt, before the last failed attempt ended, gets
// that attempt's error, so forwards queued on a dead host fail together.
func (c *connection) ensureMaster(ctx context.Context, queuedAt time.Time) error {
	c.noticeMasterDeath()
	if c.masterPID != 0 {
		return nil
	}
	if c.failedOpen != nil && queuedAt.Before(c.failedOpenAt) {
		return c.failedOpen
	}
	err := c.openMaster(ctx)
	// Share only what the host answered, not the end of one caller's context.
	if ctx.Err() == nil {
		c.failedOpen, c.failedOpenAt = err, time.Now()
	}
	return err
}

func (c *connection) openMaster(ctx context.Context) error {
	if pid, ok := c.checkMaster(); ok {
		c.masterPID = pid
		return nil
	}
	// A stale socket would make the new master run without one.
	_ = os.Remove(c.socketPath)
	_ = os.Remove(c.logPath)
	// ssh -f returns once authenticated and leaves the master running.
	openCtx, cancel := context.WithTimeout(ctx, masterOpenTimeout)
	defer cancel()
	cmd := exec.CommandContext(openCtx, "ssh", batchMasterArgs(c.socketPath, c.logPath, c.host)...)
	// ssh prints some errors, such as a config error, before -E opens the
	// log, so its stderr goes to the log too. A file, unlike a pipe, does not
	// keep Run waiting for the master that ssh -f leaves running.
	if log, err := os.OpenFile(c.logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600); err == nil {
		defer func() { _ = log.Close() }()
		cmd.Stderr = log
	}
	err := cmd.Run()
	if errors.Is(openCtx.Err(), context.DeadlineExceeded) {
		return &ConnectError{Host: c.host, Reason: fmt.Sprintf("%s did not finish connecting within %s", c.host, masterOpenTimeout)}
	}
	if err != nil {
		return connectError(c.host, c.logPath, err)
	}
	pid, ok := c.checkMaster()
	if !ok {
		return &ConnectError{Host: c.host, Reason: "ssh started no connection master"}
	}
	c.masterPID = pid
	return nil
}

func (c *connection) checkMaster() (int, bool) {
	return checkMaster(c.socketPath, c.host)
}

// noticeMasterDeath fails every established forward once the master is gone:
// the network dropped, the host closed the connection, or someone killed it.
func (c *connection) noticeMasterDeath() {
	if c.masterPID == 0 || isProcessAlive(c.masterPID) {
		return
	}
	c.masterPID = 0
	log, _ := os.ReadFile(c.logPath)
	reason := "connection to " + c.host + " closed"
	if line := lastLine(string(log)); line != "" {
		reason += ": " + line
	}
	for id, entry := range c.established {
		c.supervisor.fail(id, entry.generation, reason, false)
	}
	clear(c.established)
}

// closeMaster ends the master, which closes all its forwards, whether this
// connection opened it or a login did.
func (c *connection) closeMaster() {
	if _, err := os.Stat(c.socketPath); err == nil {
		// The supervisor's context has ended by now.
		_, _ = runControl(context.Background(), controlArgs(c.socketPath, "exit", c.host))
	}
	c.masterPID = 0
	clear(c.established)
}

// isProcessAlive reports whether pid exists. The master is not lazyfwd's
// child, so lazyfwd cannot wait for it.
func isProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
