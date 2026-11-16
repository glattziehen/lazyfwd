package tunnel

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

// masterCheckInterval is how often a connection looks for its master's death.
const masterCheckInterval = 2 * time.Second

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
	output, err := exec.Command("ssh", forwardArgs(c.socketPath, forward)...).CombinedOutput()
	if err != nil {
		c.supervisor.fail(forward.ID, generation, controlFailure(string(output)), false)
		return
	}
	c.established[forward.ID] = established{forward: forward, generation: generation}
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
