package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// maxWait bounds how long one request waits for forwards to start.
const maxWait = 2 * time.Minute

// handle runs one request on o, whether o is a running owner reached over
// the socket or a CLI command's own Owner without connections.
func handle(o *owner.Owner, request Request) Response {
	if request.Version != protocolVersion {
		return failure(&Error{Kind: ErrorFailed, Message: fmt.Sprintf(
			"the running lazyfwd speaks control protocol %d and this one %d; quit the running lazyfwd and start it again", protocolVersion, request.Version)})
	}
	switch request.Op {
	case OpState:
		return stateResponse(o, nil)
	case OpAddHost:
		return stateResponse(o, o.AddHost(request.Host))
	case OpRemoveHost:
		return stateResponse(o, o.RemoveHost(request.Host))
	case OpPinHost:
		return stateResponse(o, o.PinHost(request.Host))
	case OpUnpinHost:
		return stateResponse(o, o.UnpinHost(request.Host))
	case OpAddForwards:
		added, err := o.AddForwards(request.Host, request.Direction, request.Targets, request.Bind)
		if len(added) == 0 {
			return failure(err)
		}
		ids := make([]workspace.ForwardID, len(added))
		for i, forward := range added {
			ids[i] = forward.ID
		}
		return forwardsResponse(o, ids, err)
	case OpEditForward:
		if len(request.IDs) != 1 {
			return failure(fmt.Errorf("edit one forward at a time, got %d IDs", len(request.IDs)))
		}
		_, err := o.EditForward(request.IDs[0], request.Target, request.Bind)
		return forwardsResponse(o, request.IDs, err)
	case OpDeleteForwards:
		return eachForward(o, request.IDs, o.DeleteForward, 0)
	case OpStartForwards:
		return eachForward(o, request.IDs, o.StartForward, request.WaitMillis)
	case OpStopForwards:
		return eachForward(o, request.IDs, o.StopForward, 0)
	case OpToggleForwards:
		return eachForward(o, request.IDs, o.ToggleForward, request.WaitMillis)
	case OpListeners:
		listing, err := o.Listeners(request.Host)
		if err != nil {
			return failure(err)
		}
		return Response{Listing: &listing, State: stateOf(o)}
	case OpControlPath:
		path, err := o.ControlPath(request.Host)
		if err != nil {
			return failure(err)
		}
		return Response{ControlPath: path}
	case OpLoggedIn:
		return failure(o.LoggedIn(request.Host))
	}
	return failure(fmt.Errorf("unknown operation %q", request.Op))
}

// eachForward runs operation on every forward after checking that they all
// exist, then waits up to waitMillis for those that it left starting.
func eachForward(o *owner.Owner, ids []workspace.ForwardID, operation func(workspace.ForwardID) error, waitMillis int64) Response {
	if len(ids) == 0 {
		return failure(errors.New("name at least one forward ID"))
	}
	state := o.State()
	for _, id := range ids {
		if _, ok := state.Forward(id); !ok {
			return failure(&owner.ForwardNotFoundError{ID: id})
		}
	}
	for _, id := range ids {
		if err := operation(id); err != nil {
			return failure(err)
		}
	}
	wait := min(time.Duration(waitMillis)*time.Millisecond, maxWait)
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	for _, id := range ids {
		// A deleted forward is gone, which the response shows by leaving it out.
		_, _ = o.AwaitStart(ctx, id)
	}
	return forwardsResponse(o, ids, nil)
}

func forwardsResponse(o *owner.Owner, ids []workspace.ForwardID, err error) Response {
	response := failure(err)
	state := o.State()
	for _, id := range ids {
		if forward, ok := state.Forward(id); ok {
			response.Forwards = append(response.Forwards, forward)
		}
	}
	return response
}

func stateResponse(o *owner.Owner, err error) Response {
	if err != nil {
		return failure(err)
	}
	return Response{State: stateOf(o)}
}

func stateOf(o *owner.Owner) *owner.State {
	state := o.State()
	return &state
}

// failure is the Response for err, which may be nil.
func failure(err error) Response {
	if err == nil {
		return Response{}
	}
	return Response{Error: errorOf(err)}
}

func errorOf(err error) *Error {
	var controlErr *Error
	var hostNotFound *owner.HostNotFoundError
	var forwardNotFound *owner.ForwardNotFoundError
	var connectErr *tunnel.ConnectError
	switch {
	case errors.As(err, &controlErr):
		return controlErr
	case errors.Is(err, owner.ErrNoConnections):
		return &Error{Kind: ErrorNoOwner, Message: err.Error()}
	case errors.As(err, &hostNotFound), errors.As(err, &forwardNotFound):
		return &Error{Kind: ErrorNotFound, Message: err.Error()}
	case errors.As(err, &connectErr) && connectErr.NeedsLogin:
		return &Error{Kind: ErrorNeedsLogin, Message: fmt.Sprintf("%s needs an interactive login; ssh said: %s", connectErr.Host, strings.TrimSuffix(connectErr.Reason, "."))}
	}
	return &Error{Kind: ErrorFailed, Message: err.Error()}
}
