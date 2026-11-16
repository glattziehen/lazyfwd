// Package tunnel runs forwards on one shared ssh connection per host.
package tunnel

import (
	"context"
	"errors"
	"sync"

	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// State is where a forward stands.
type State string

const (
	// Stopped is a forward that was never started, or was stopped.
	Stopped State = "stopped"
	// Starting is a started forward that the master does not hold yet.
	Starting State = "starting"
	// Running is a forward that the master holds.
	Running State = "running"
	// Failed is a forward that ended without Stop being called: its
	// connection did not open, the master refused the forward, or the master
	// died.
	Failed State = "failed"
)

// Status is a forward's state, with the reason for a Failed one.
type Status struct {
	State State `json:"state"`
	// Reason is ssh's explanation, or its exit error when it gave none.
	Reason string `json:"reason,omitempty"`
	// NeedsLogin is set when an interactive login would let the forward start.
	NeedsLogin bool `json:"needs_login,omitempty"`
}

// IsActive reports whether the forward is started and has not failed.
func (s Status) IsActive() bool {
	return s.State == Starting || s.State == Running
}

// Event reports a status change that no Start or Stop made: the master took
// the forward (Running), or the forward ended on its own (Failed).
type Event struct {
	ID     workspace.ForwardID
	Status Status
}

// running is a forward between Start and Stop or its failure.
type running struct {
	forward workspace.Forward
	// generation tells a restart apart from the run it replaced.
	generation uint64
	// isEstablished is set once the master holds the forward.
	isEstablished bool
}

// Supervisor owns one connection per host and tracks which forwards run.
// Start and Stop return at once; the host's connection does the ssh work in
// order, and the outcome arrives on Events.
type Supervisor struct {
	runDir runDir
	// ctx ends in-flight connects when the supervisor stops.
	ctx    context.Context
	cancel context.CancelFunc
	events chan Event

	mu          sync.Mutex
	isStopped   bool
	connections map[string]*connection
	running     map[workspace.ForwardID]running
	// failures holds the Failed status of each forward that failed since it
	// last started or stopped.
	failures   map[workspace.ForwardID]Status
	generation uint64
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
		events:      make(chan Event, 64),
		connections: map[string]*connection{},
		running:     map[workspace.ForwardID]running{},
		failures:    map[workspace.ForwardID]Status{},
	}, nil
}

// Events delivers one Event per forward that the master takes and per
// forward that ends on its own. Status already reports the change when its
// Event arrives.
func (s *Supervisor) Events() <-chan Event {
	return s.events
}

// RunDir is the directory that this process alone owns while the supervisor
// runs: private to the user, and removed by StopAll. Other parts of lazyfwd
// may keep their own sockets there.
func (s *Supervisor) RunDir() string {
	return s.runDir.path
}

// Start opens the forward; starting a starting or running forward is a no-op.
func (s *Supervisor) Start(forward workspace.Forward) {
	s.mu.Lock()
	if _, ok := s.running[forward.ID]; ok || s.isStopped {
		s.mu.Unlock()
		return
	}
	delete(s.failures, forward.ID)
	s.generation++
	generation := s.generation
	s.running[forward.ID] = running{forward: forward, generation: generation}
	c := s.connection(forward.Host)
	// Send unlocked: the connection takes s.mu while it works.
	s.mu.Unlock()
	c.startForward(s.ctx, forward, generation)
}

// Stop closes the forward and forgets its failure; stopping a stopped
// forward is a no-op.
func (s *Supervisor) Stop(id workspace.ForwardID) {
	s.mu.Lock()
	delete(s.failures, id)
	entry, ok := s.running[id]
	if !ok || s.isStopped {
		s.mu.Unlock()
		return
	}
	delete(s.running, id)
	c := s.connection(entry.forward.Host)
	s.mu.Unlock()
	c.stopForward(id)
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
	clear(s.failures)
	connections := s.connections
	s.mu.Unlock()
	s.cancel()
	for _, c := range connections {
		c.close()
	}
	s.runDir.remove()
}

// Status reports where the forward stands.
func (s *Supervisor) Status(id workspace.ForwardID) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.running[id]; ok {
		if entry.isEstablished {
			return Status{State: Running}
		}
		return Status{State: Starting}
	}
	if failure, ok := s.failures[id]; ok {
		return failure
	}
	return Status{State: Stopped}
}

// ControlPath is the socket of host's connection, which a Login opens.
func (s *Supervisor) ControlPath(host string) string {
	return s.runDir.socketPath(host)
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
	return c.runCommand(ctx, command)
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

// establish marks the forward's run as generation as held by the master,
// unless it was stopped or restarted since.
func (s *Supervisor) establish(id workspace.ForwardID, generation uint64) {
	s.mu.Lock()
	entry, ok := s.running[id]
	isCurrent := ok && entry.generation == generation
	if isCurrent {
		entry.isEstablished = true
		s.running[id] = entry
	}
	s.mu.Unlock()
	if isCurrent {
		s.send(Event{ID: id, Status: Status{State: Running}})
	}
}

// fail ends the forward's run as generation and reports why, unless it was
// stopped or restarted since.
func (s *Supervisor) fail(id workspace.ForwardID, generation uint64, reason string, needsLogin bool) {
	status := Status{State: Failed, Reason: reason, NeedsLogin: needsLogin}
	s.mu.Lock()
	entry, ok := s.running[id]
	isCurrent := ok && entry.generation == generation
	if isCurrent {
		delete(s.running, id)
		s.failures[id] = status
	}
	s.mu.Unlock()
	if isCurrent {
		s.send(Event{ID: id, Status: status})
	}
}

func (s *Supervisor) send(event Event) {
	select {
	case s.events <- event:
	case <-s.ctx.Done(): // nobody reads events once StopAll began
	}
}
