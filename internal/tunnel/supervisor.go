// Package tunnel runs forwards on one shared ssh connection per host.
package tunnel

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

// Exited reports that a forward ended without Stop being called: its
// connection did not open, the master refused the forward, or the master died.
type Exited struct {
	ID workspace.ForwardID
	// Reason is ssh's explanation, or its exit error when it gave none.
	Reason string
	// NeedsLogin is set when an interactive login would let the forward start.
	NeedsLogin bool
}

// running is a forward between Start and Stop or its Exited.
type running struct {
	forward workspace.Forward
	// generation tells a restart apart from the run it replaced.
	generation uint64
}

// Supervisor owns one connection per host and tracks which forwards run.
// Start and Stop return at once; the host's connection does the ssh work in
// order, and failures arrive on Exits.
type Supervisor struct {
	runDir runDir
	// ctx ends in-flight connects when the supervisor stops.
	ctx    context.Context
	cancel context.CancelFunc
	exits  chan Exited

	mu          sync.Mutex
	isStopped   bool
	connections map[string]*connection
	running     map[workspace.ForwardID]running
	generation  uint64
}

// NewSupervisor keeps its sockets in a run directory under baseDir, after
// closing the forwards of lazyfwd processes that died without cleaning up.
func NewSupervisor(baseDir string) (*Supervisor, error) {
	dir, err := openRunDir(baseDir)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Supervisor{
		runDir:      dir,
		ctx:         ctx,
		cancel:      cancel,
		exits:       make(chan Exited, 64),
		connections: map[string]*connection{},
		running:     map[workspace.ForwardID]running{},
	}, nil
}

// Exits delivers one Exited per forward that ends on its own.
func (s *Supervisor) Exits() <-chan Exited {
	return s.exits
}

// Start opens the forward; starting a running forward is a no-op.
func (s *Supervisor) Start(forward workspace.Forward) {
	s.mu.Lock()
	if _, ok := s.running[forward.ID]; ok || s.isStopped {
		s.mu.Unlock()
		return
	}
	s.generation++
	generation := s.generation
	s.running[forward.ID] = running{forward: forward, generation: generation}
	c := s.connection(forward.Host)
	// Send unlocked: the connection takes s.mu while it works.
	s.mu.Unlock()
	queuedAt := time.Now()
	c.enqueue(func() { c.start(s.ctx, forward, generation, queuedAt) })
}

// Stop closes the forward; stopping a stopped forward is a no-op.
func (s *Supervisor) Stop(id workspace.ForwardID) {
	s.mu.Lock()
	entry, ok := s.running[id]
	if !ok || s.isStopped {
		s.mu.Unlock()
		return
	}
	delete(s.running, id)
	c := s.connection(entry.forward.Host)
	s.mu.Unlock()
	c.enqueue(func() { c.stop(id) })
}

// StopAll closes every connection, which ends all forwards, and removes the
// run directory. The supervisor starts nothing afterwards.
func (s *Supervisor) StopAll() {
	s.mu.Lock()
	if s.isStopped {
		s.mu.Unlock()
		return
	}
	s.isStopped = true
	clear(s.running)
	connections := s.connections
	s.mu.Unlock()
	s.cancel()
	for _, c := range connections {
		c.close()
	}
	s.runDir.remove()
}

// IsRunning reports whether the forward is started and has not failed.
func (s *Supervisor) IsRunning(id workspace.ForwardID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.running[id]
	return ok
}

// Login is an interactive ssh that opens host's connection on the terminal.
func (s *Supervisor) Login(host string) *Login {
	socketPath := s.runDir.socketPath(host)
	logPath := socketPath + logExt
	return &Login{
		Host:       host,
		socketPath: socketPath,
		logPath:    logPath,
		cmd:        exec.Command("ssh", loginArgs(socketPath, logPath, host)...),
	}
}

// Run runs command on host over its connection, opening the connection
// without prompts if needed, and returns its stdout, also when it fails. The
// error is an *exec.ExitError when the command ran and exited non-zero, a
// *ConnectError when the connection did not open, and ssh's first stderr
// line for ssh's other failures.
func (s *Supervisor) Run(ctx context.Context, host, command string) (string, error) {
	s.mu.Lock()
	if s.isStopped {
		s.mu.Unlock()
		return "", errors.New("lazyfwd is stopping")
	}
	c := s.connection(host)
	s.mu.Unlock()
	// End the connect and the command when StopAll begins, too.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(s.ctx, cancel)()
	connected := make(chan error, 1)
	queuedAt := time.Now()
	c.enqueue(func() { connected <- c.ensureMaster(ctx, queuedAt) })
	select {
	case err := <-connected:
		if err != nil {
			return "", err
		}
	case <-ctx.Done():
		return "", ctx.Err()
	}
	cmd := exec.CommandContext(ctx, "ssh", remoteCommandArgs(c.socketPath, host, command)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	// ssh exits 255 for its own errors; other statuses are the command's.
	var exitErr *exec.ExitError
	if err == nil || (errors.As(err, &exitErr) && exitErr.ExitCode() != 255) {
		return stdout.String(), err
	}
	reason, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
	if reason == "" {
		reason = err.Error()
	}
	return stdout.String(), errors.New(strings.TrimSpace(reason))
}

// connection returns host's connection, creating it on first use; s.mu must
// be held.
func (s *Supervisor) connection(host string) *connection {
	c, ok := s.connections[host]
	if !ok {
		c = newConnection(s, host, s.runDir.socketPath(host))
		s.connections[host] = c
	}
	return c
}

// isCurrent reports whether the forward still runs as generation.
func (s *Supervisor) isCurrent(id workspace.ForwardID, generation uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[id].generation == generation
}

// fail ends the forward's run as generation and reports why, unless it was
// stopped or restarted since.
func (s *Supervisor) fail(id workspace.ForwardID, generation uint64, reason string, needsLogin bool) {
	s.mu.Lock()
	entry, ok := s.running[id]
	isCurrent := ok && entry.generation == generation
	if isCurrent {
		delete(s.running, id)
	}
	s.mu.Unlock()
	if !isCurrent {
		return
	}
	select {
	case s.exits <- Exited{ID: id, Reason: reason, NeedsLogin: needsLogin}:
	case <-s.ctx.Done(): // nobody reads exits once StopAll began
	}
}
