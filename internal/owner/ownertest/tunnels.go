// Package ownertest has a fake owner.Tunnels that runs no ssh.
package ownertest

import (
	"context"
	"sync"

	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// Tunnels records which forwards run. A started forward stays starting
// unless its host has an outcome; Fail ends one.
type Tunnels struct {
	mu       sync.Mutex
	statuses map[workspace.ForwardID]tunnel.Status
	outcomes map[string]tunnel.Status
	events   chan tunnel.Event
	// runOutput is what Run prints on every host.
	runOutput string
}

func NewTunnels() *Tunnels {
	return &Tunnels{
		statuses: map[workspace.ForwardID]tunnel.Status{},
		outcomes: map[string]tunnel.Status{},
		events:   make(chan tunnel.Event, 64),
	}
}

// SetOutcome makes every later Start on host reach status at once.
func (t *Tunnels) SetOutcome(host string, status tunnel.Status) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.outcomes[host] = status
}

// SetRunOutput makes Run print output.
func (t *Tunnels) SetRunOutput(output string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.runOutput = output
}

// Fail ends a forward as the master would.
func (t *Tunnels) Fail(id workspace.ForwardID, reason string) {
	t.set(id, tunnel.Status{State: tunnel.Failed, Reason: reason})
}

func (t *Tunnels) Start(forward workspace.Forward) {
	t.mu.Lock()
	if t.statuses[forward.ID].IsActive() {
		t.mu.Unlock()
		return
	}
	t.statuses[forward.ID] = tunnel.Status{State: tunnel.Starting}
	outcome, ok := t.outcomes[forward.Host]
	t.mu.Unlock()
	if ok {
		t.set(forward.ID, outcome)
	}
}

func (t *Tunnels) set(id workspace.ForwardID, status tunnel.Status) {
	t.mu.Lock()
	t.statuses[id] = status
	t.mu.Unlock()
	t.events <- tunnel.Event{ID: id, Status: status}
}

func (t *Tunnels) Stop(id workspace.ForwardID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.statuses, id)
}

func (t *Tunnels) Status(id workspace.ForwardID) tunnel.Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	if status, ok := t.statuses[id]; ok {
		return status
	}
	return tunnel.Status{State: tunnel.Stopped}
}

func (t *Tunnels) Events() <-chan tunnel.Event { return t.events }

func (t *Tunnels) Run(context.Context, string, string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.runOutput, nil
}

func (t *Tunnels) ControlPath(host string) string { return "/nonexistent/" + host }

func (t *Tunnels) StopAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	clear(t.statuses)
}
