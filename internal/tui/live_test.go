package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/owner/ownertest"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

type discardStore struct{}

func (discardStore) Save(workspace.Workspace) error { return nil }

// newLiveModel has forwards 1 and 2 on host a and 3 on host b, focuses the
// Live panel, and fails the given forwards. Failed forwards need no ssh.
func newLiveModel(t *testing.T, failed ...workspace.ForwardID) Model {
	t.Helper()
	m, _ := newLiveModelWithTunnels(t, failed...)
	return m
}

func newLiveModelWithTunnels(t *testing.T, failed ...workspace.ForwardID) (Model, *ownertest.Tunnels) {
	t.Helper()
	var ws workspace.Workspace
	for _, host := range []string{"a", "a", "b"} {
		target := workspace.End{Host: "localhost", Port: 8000 + len(ws.Forwards)}
		if _, err := ws.AddForwards(host, workspace.Local, []workspace.End{target}, workspace.End{Host: workspace.BindHost, Port: 9000 + len(ws.Forwards)}, func(int) bool { return true }); err != nil {
			t.Fatal(err)
		}
	}
	m, tunnels := newTestModel(t, ws, nil)
	m = send(m, keyMsg("4"))
	for _, id := range failed {
		m = fail(m, tunnels, id, "boom")
	}
	return m, tunnels
}

// newTestModel shows ws through an owner whose forwards run on fake tunnels.
func newTestModel(t *testing.T, ws workspace.Workspace, sshConfigHosts []string) (Model, *ownertest.Tunnels) {
	t.Helper()
	tunnels := ownertest.NewTunnels()
	o := owner.New(discardStore{}, tunnels, ws, sshConfigHosts)
	t.Cleanup(o.Close)
	return New(o), tunnels
}

// fail ends a forward as ssh would and tells the model.
func fail(m Model, tunnels *ownertest.Tunnels, id workspace.ForwardID, reason string) Model {
	tunnels.Fail(id, reason)
	return send(m, eventMsg{})
}

func send(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func selectedLiveID(m Model) workspace.ForwardID {
	forward, _ := m.selectedLive()
	return forward.ID
}

func TestLiveCursorStaysOnItsForwardWhenOneAboveFails(t *testing.T) {
	m, tunnels := newLiveModelWithTunnels(t, 2, 3)
	m = send(m, keyMsg("j"))

	m = fail(m, tunnels, 1, "boom")

	if got := selectedLiveID(m); got != 3 {
		t.Fatalf("selected forward %d, want 3", got)
	}
}

func TestLiveCursorKeepsItsRowWhenItsForwardLeaves(t *testing.T) {
	m := newLiveModel(t, 1, 2, 3)
	m = send(m, keyMsg("j"))

	m = send(m, keyMsg("d"))

	if got := selectedLiveID(m); got != 3 {
		t.Fatalf("selected forward %d, want 3", got)
	}
}

func TestLiveEnterShowsTheForwardInItsHost(t *testing.T) {
	m := newLiveModel(t, 1, 3)
	m = send(m, keyMsg("j"))

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	forward, _ := m.selectedForward()
	if m.focus != panelForwards || m.selectedHost() != "b" || forward.ID != 3 {
		t.Fatalf("focus %d, host %q, forward %d; want Forwards, b, 3", m.focus, m.selectedHost(), forward.ID)
	}
}
