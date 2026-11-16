package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

const (
	// masterCheckInterval is how often a connection looks for its master's death.
	masterCheckInterval = 2 * time.Second
	// probeTimeout bounds the check whether something listens on the host.
	probeTimeout = 5 * time.Second
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
// forward.
type connection struct {
	host       string
	socketPath string
	logPath    string
	supervisor *Supervisor
	operations chan func()
	// quit asks the run loop to close the master; closed reports it did.
	quit   chan struct{}
	closed chan struct{}

	// masterPID is 0 while no master is known.
	masterPID   int
	established map[workspace.ForwardID]established
}

func newConnection(supervisor *Supervisor, host, socketPath string) *connection {
	c := &connection{
		host:        host,
		socketPath:  socketPath,
		logPath:     socketPath + logExt,
		supervisor:  supervisor,
		operations:  make(chan func(), 256),
		quit:        make(chan struct{}),
		closed:      make(chan struct{}),
		established: map[workspace.ForwardID]established{},
	}
	go c.run()
	return c
}

func (c *connection) run() {
	ticker := time.NewTicker(masterCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case operation := <-c.operations:
			operation()
		case <-ticker.C:
			c.noticeMasterDeath()
		case <-c.quit:
			c.closeMaster()
			close(c.closed)
			return
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
func (c *connection) start(ctx context.Context, forward workspace.Forward, generation uint64) {
	if !c.supervisor.isCurrent(forward.ID, generation) {
		return
	}
	if err := c.ensureMaster(ctx); err != nil {
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
	output, err := exec.Command("ssh", forwardArgs(c.socketPath, forward)...).CombinedOutput()
	if err != nil && c.removeStaleRemoteSocket(ctx, forward) {
		output, err = exec.Command("ssh", forwardArgs(c.socketPath, forward)...).CombinedOutput()
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
	command := removeSocketCommand(forward.Bind.Path)
	return exec.CommandContext(ctx, "ssh", remoteCommandArgs(c.socketPath, c.host, command)...).Run() == nil
}

// stop takes the forward off the master.
func (c *connection) stop(id workspace.ForwardID) {
	entry, ok := c.established[id]
	if !ok {
		return
	}
	delete(c.established, id)
	_ = exec.Command("ssh", cancelArgs(c.socketPath, entry.forward)...).Run()
}

// ensureMaster makes sure a live master serves the socket: the known one, one
// an interactive login left there, or a new one opened without prompts.
func (c *connection) ensureMaster(ctx context.Context) error {
	c.noticeMasterDeath()
	if c.masterPID != 0 {
		return nil
	}
	if pid, ok := c.checkMaster(); ok {
		c.masterPID = pid
		return nil
	}
	// A stale socket would make the new master run without one.
	_ = os.Remove(c.socketPath)
	_ = os.Remove(c.logPath)
	// ssh -f returns once authenticated and leaves the master running.
	err := exec.CommandContext(ctx, "ssh", batchMasterArgs(c.socketPath, c.logPath, c.host)...).Run()
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
		_ = exec.Command("ssh", controlArgs(c.socketPath, "exit", c.host)...).Run()
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
