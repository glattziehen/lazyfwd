package owner_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/owner/ownertest"
	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

type discardStore struct{}

func (discardStore) Save(workspace.Workspace) error { return nil }

func newOwner(t *testing.T, tunnels owner.Tunnels) *owner.Owner {
	t.Helper()
	o := owner.New(discardStore{}, tunnels, workspace.Workspace{}, []string{"box"})
	t.Cleanup(o.Close)
	return o
}

func addForward(t *testing.T, o *owner.Owner, target, bind workspace.End) workspace.Forward {
	t.Helper()
	added, err := o.AddForwards("box", workspace.Local, []workspace.End{target}, bind)
	if err != nil {
		t.Fatal(err)
	}
	return added[0]
}

func TestRefusedEditLeavesForwardRunningAndUnchanged(t *testing.T) {
	o := newOwner(t, ownertest.NewTunnels())
	forward := addForward(t, o, workspace.End{Host: "localhost", Port: 8000}, workspace.End{Host: workspace.BindHost, Port: 9000})
	if err := o.StartForward(forward.ID); err != nil {
		t.Fatal(err)
	}

	// A socket target with an automatic local end has no port to copy.
	_, err := o.EditForward(forward.ID, workspace.End{Path: "/run/app.sock"}, workspace.End{})

	got, _ := o.State().Forward(forward.ID)
	if err == nil || got.Forward != forward || !got.Status.IsActive() {
		t.Fatalf("edit error %v; forward %+v, want refused, unchanged, and running", err, got)
	}
}

// Auto-picking during an edit may keep the running forward's own port,
// which the forward itself holds until the restart.
func TestEditKeepsRunningForwardsOwnPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0") // stands in for the forward's own listener
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port
	o := newOwner(t, ownertest.NewTunnels())
	forward := addForward(t, o, workspace.End{Host: "localhost", Port: port}, workspace.End{Host: workspace.BindHost, Port: port})
	if err := o.StartForward(forward.ID); err != nil {
		t.Fatal(err)
	}

	edited, err := o.EditForward(forward.ID, forward.Target, workspace.End{})
	if err != nil || edited.Bind.Port != port {
		t.Fatalf("local end moved from %d to %d (%v)", port, edited.Bind.Port, err)
	}
}

func TestAwaitStartReturnsTheOutcome(t *testing.T) {
	for _, outcome := range []tunnel.Status{
		{State: tunnel.Running},
		{State: tunnel.Failed, Reason: "Permission denied (publickey).", NeedsLogin: true},
	} {
		tunnels := ownertest.NewTunnels()
		tunnels.SetOutcome("box", outcome)
		o := newOwner(t, tunnels)
		forward := addForward(t, o, workspace.End{Host: "localhost", Port: 8000}, workspace.End{Host: workspace.BindHost, Port: 9000})
		if err := o.StartForward(forward.ID); err != nil {
			t.Fatal(err)
		}

		got, err := o.AwaitStart(context.Background(), forward.ID)

		if err != nil || got.Status != outcome {
			t.Errorf("got %+v, %v; want %+v", got.Status, err, outcome)
		}
	}
}

func TestAwaitStartEndsWithItsContextWhileStarting(t *testing.T) {
	o := newOwner(t, ownertest.NewTunnels())
	forward := addForward(t, o, workspace.End{Host: "localhost", Port: 8000}, workspace.End{Host: workspace.BindHost, Port: 9000})
	if err := o.StartForward(forward.ID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	got, err := o.AwaitStart(ctx, forward.ID)

	if err != nil || got.Status.State != tunnel.Starting {
		t.Fatalf("got %+v, %v; want still starting", got.Status, err)
	}
}

// With no lazyfwd running, saved state changes but nothing starts.
func TestOwnerWithoutTunnelsChangesSavedStateOnly(t *testing.T) {
	o := newOwner(t, nil)
	forward := addForward(t, o, workspace.End{Host: "localhost", Port: 8000}, workspace.End{})

	if err := o.StartForward(forward.ID); !errors.Is(err, owner.ErrNoConnections) {
		t.Errorf("start: %v, want ErrNoConnections", err)
	}
	if err := o.DeleteForward(forward.ID); err != nil {
		t.Errorf("delete: %v", err)
	}
	if forwards := o.State().Forwards; len(forwards) != 0 {
		t.Errorf("forwards left: %+v", forwards)
	}
}

func TestLoggedInRestartsTheHostsFailedForwards(t *testing.T) {
	tunnels := ownertest.NewTunnels()
	o := newOwner(t, tunnels)
	forward := addForward(t, o, workspace.End{Host: "localhost", Port: 8000}, workspace.End{Host: workspace.BindHost, Port: 9000})
	if err := o.StartForward(forward.ID); err != nil {
		t.Fatal(err)
	}
	tunnels.Fail(forward.ID, "Permission denied (publickey).")

	if err := o.LoggedIn("box"); err != nil {
		t.Fatal(err)
	}

	if got, _ := o.State().Forward(forward.ID); got.Status.State != tunnel.Starting {
		t.Fatalf("status %+v after login, want starting", got.Status)
	}
}
