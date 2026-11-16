package control

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/owner/ownertest"
	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// shortTempDir keeps socket paths under the 104-byte limit of macOS.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "lazyfwd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func noSSHConfigHosts() ([]string, error) { return nil, nil }

// startOwner runs an owner of store on fake tunnels, as the TUI or serve does.
func startOwner(t *testing.T, store workspace.FileStore) (*owner.Owner, *ownertest.Tunnels) {
	t.Helper()
	lock, err := Claim(store)
	if err != nil {
		t.Fatal(err)
	}
	tunnels := ownertest.NewTunnels()
	o := owner.New(store, tunnels, workspace.Workspace{}, nil)
	server, err := Serve(o, lock, shortTempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		o.Close()
		server.Close()
		lock.Release()
	})
	return o, tunnels
}

func connect(t *testing.T, store workspace.FileStore) *Client {
	t.Helper()
	client, err := Connect(store, noSSHConfigHosts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func newStore(t *testing.T) workspace.FileStore {
	return workspace.FileStore{Path: filepath.Join(t.TempDir(), "workspace.json")}
}

// A command's change must reach the running owner's memory, which the TUI
// shows, and a start must wait for the connection.
func TestCommandsChangeTheRunningOwner(t *testing.T) {
	store := newStore(t)
	o, tunnels := startOwner(t, store)
	tunnels.SetOutcome("box", tunnel.Status{State: tunnel.Running})
	client := connect(t, store)

	added, err := client.Call(Request{Op: OpAddForwards, Host: "box", Direction: workspace.Local, Targets: []workspace.End{{Host: "localhost", Port: 4966}}})
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.Call(Request{Op: OpStartForwards, IDs: []workspace.ForwardID{added.Forwards[0].Forward.ID}, WaitMillis: 5000})
	if err != nil {
		t.Fatal(err)
	}

	if len(started.Forwards) != 1 || started.Forwards[0].Status.State != tunnel.Running {
		t.Fatalf("start returned %+v, want one running forward", started.Forwards)
	}
	if forwards := o.State().Forwards; len(forwards) != 1 || forwards[0].Status.State != tunnel.Running {
		t.Fatalf("owner has %+v, want the running forward", forwards)
	}
}

// With no owner running, saved state changes on disk, and anything that
// needs a connection fails as such.
func TestCommandsWithoutOwnerChangeTheSavedWorkspace(t *testing.T) {
	store := newStore(t)
	client := connect(t, store)

	added, err := client.Call(Request{Op: OpAddForwards, Host: "box", Direction: workspace.Local, Targets: []workspace.End{{Host: "localhost", Port: 4966}}})
	if err != nil {
		t.Fatal(err)
	}
	_, startErr := client.Call(Request{Op: OpStartForwards, IDs: []workspace.ForwardID{added.Forwards[0].Forward.ID}})
	client.Close()

	var controlErr *Error
	if !errors.As(startErr, &controlErr) || controlErr.Kind != ErrorNoOwner {
		t.Errorf("start without owner: %v, want %s", startErr, ErrorNoOwner)
	}
	ws, err := store.Load()
	if err != nil || len(ws.Forwards) != 1 {
		t.Fatalf("saved forwards %+v (%v), want the added one", ws.Forwards, err)
	}
}

// An owner killed with -9 leaves its address in the lock file but drops the
// lock; the next command must work without it.
func TestCrashedOwnersAddressIsIgnored(t *testing.T) {
	store := newStore(t)
	lock, err := store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.SetHolderAddress(filepath.Join(shortTempDir(t), socketName)); err != nil {
		t.Fatal(err)
	}
	lock.Release()

	client := connect(t, store)

	if client.local == nil {
		t.Fatal("client reached for the crashed owner")
	}
}

func TestSecondOwnerIsRefused(t *testing.T) {
	store := newStore(t)
	startOwner(t, store)

	if _, err := Claim(store); err == nil {
		t.Fatal("a second owner claimed the workspace")
	}
}
