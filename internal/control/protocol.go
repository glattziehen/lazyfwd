// Package control lets CLI commands reach the workspace's owner: the
// lazyfwd process (TUI or serve) that holds the workspace lock and the ssh
// connections. The owner answers on a unix socket in its private run
// directory and records the socket's path in the workspace lock file. With
// no owner running, a command takes the lock itself and changes the saved
// workspace through an Owner without connections.
//
// One connection carries one request and one response, each a JSON line.
package control

import (
	"github.com/glattziehen/lazyfwd/internal/listeners"
	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// protocolVersion changes with any change to Request or Response that an
// owner of another version would misread.
const protocolVersion = 1

// Op names an owner operation.
type Op string

const (
	OpState          Op = "state"
	OpAddHost        Op = "add_host"
	OpRemoveHost     Op = "remove_host"
	OpPinHost        Op = "pin_host"
	OpUnpinHost      Op = "unpin_host"
	OpAddForwards    Op = "add_forwards"
	OpEditForward    Op = "edit_forward"
	OpDeleteForwards Op = "delete_forwards"
	OpStartForwards  Op = "start_forwards"
	OpStopForwards   Op = "stop_forwards"
	OpToggleForwards Op = "toggle_forwards"
	OpListeners      Op = "listeners"
	OpControlPath    Op = "control_path"
	OpLoggedIn       Op = "logged_in"
)

// Request asks the owner for one operation. Each Op reads only its fields:
// Host for host operations, OpAddForwards, and the login and listener
// operations; IDs for the forward operations; Direction, Targets, and Bind
// for OpAddForwards; Target and Bind for OpEditForward; WaitMillis for
// OpStartForwards and OpToggleForwards.
type Request struct {
	Version   int                   `json:"version"`
	Op        Op                    `json:"op"`
	Host      string                `json:"host,omitempty"`
	IDs       []workspace.ForwardID `json:"ids,omitempty"`
	Direction workspace.Direction   `json:"direction,omitempty"`
	Targets   []workspace.End       `json:"targets,omitempty"`
	Target    workspace.End         `json:"target"`
	// Bind is the listening end; the zero End picks one automatically.
	Bind workspace.End `json:"bind"`
	// WaitMillis is how long to wait for started forwards to leave
	// Starting; 0 returns at once.
	WaitMillis int64 `json:"wait_millis,omitempty"`
}

// Response is the owner's answer: the operation's error, or its result.
type Response struct {
	Error *Error `json:"error,omitempty"`
	// State is the whole workspace, for OpState and the host operations.
	State *owner.State `json:"state,omitempty"`
	// Forwards are the forwards an operation added or acted on, as they
	// stand when it returns.
	Forwards    []owner.ForwardState `json:"forwards,omitempty"`
	Listing     *listeners.Listing   `json:"listing,omitempty"`
	ControlPath string               `json:"control_path,omitempty"`
}

// ErrorKind tells a caller how to react to an Error.
type ErrorKind string

const (
	// ErrorFailed is a refused change or an operation that failed.
	ErrorFailed ErrorKind = "failed"
	// ErrorNotFound names a host or forward that does not exist.
	ErrorNotFound ErrorKind = "not_found"
	// ErrorNoOwner is an operation that needs ssh connections while no
	// lazyfwd runs to hold them.
	ErrorNoOwner ErrorKind = "no_owner"
	// ErrorNeedsLogin is a connection that only an interactive login opens.
	ErrorNeedsLogin ErrorKind = "needs_login"
)

// Error is a failed operation.
type Error struct {
	Kind    ErrorKind `json:"kind"`
	Message string    `json:"message"`
}

func (e *Error) Error() string {
	return e.Message
}
