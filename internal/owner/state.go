package owner

import (
	"slices"

	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// State is the workspace at one moment, with each forward's status.
type State struct {
	// Hosts are pinned first, then by name.
	Hosts []workspace.Host `json:"hosts"`
	// Forwards are in creation order.
	Forwards []ForwardState `json:"forwards"`
}

// ForwardState is a forward and where it stands.
type ForwardState struct {
	Forward workspace.Forward `json:"forward"`
	Status  tunnel.Status     `json:"status"`
}

// Forward finds a forward by ID.
func (s State) Forward(id workspace.ForwardID) (ForwardState, bool) {
	i := slices.IndexFunc(s.Forwards, func(f ForwardState) bool { return f.Forward.ID == id })
	if i < 0 {
		return ForwardState{}, false
	}
	return s.Forwards[i], true
}

// ForwardsOf lists the forwards through one host in creation order.
func (s State) ForwardsOf(host string) []workspace.Forward {
	var forwards []workspace.Forward
	for _, state := range s.Forwards {
		if state.Forward.Host == host {
			forwards = append(forwards, state.Forward)
		}
	}
	return forwards
}

// Status is where a forward stands; an unknown forward is stopped.
func (s State) Status(id workspace.ForwardID) tunnel.Status {
	if state, ok := s.Forward(id); ok {
		return state.Status
	}
	return tunnel.Status{State: tunnel.Stopped}
}

// ListenerForward finds host's local forward to its listening port: the
// forward to localhost:port.
func (s State) ListenerForward(host string, port int) (workspace.Forward, bool) {
	for _, forward := range s.ForwardsOf(host) {
		if forward.Direction == workspace.Local && forward.Target == (workspace.End{Host: workspace.DefaultTargetHost, Port: port}) {
			return forward, true
		}
	}
	return workspace.Forward{}, false
}
