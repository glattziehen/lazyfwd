package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

type discardStore struct{}

func (discardStore) Save(workspace.Workspace) error { return nil }

// newLiveModel has forwards 1 and 2 on host a and 3 on host b, focuses the
// Live panel, and fails the given forwards. Failed forwards need no ssh.
func newLiveModel(t *testing.T, failed ...workspace.ForwardID) Model {
	t.Helper()
	var ws workspace.Workspace
	for _, host := range []string{"a", "a", "b"} {
		target := workspace.End{Host: "localhost", Port: 8000 + len(ws.Forwards)}
		if _, err := ws.AddForwards(host, workspace.Local, []workspace.End{target}, workspace.End{Host: workspace.BindHost, Port: 9000 + len(ws.Forwards)}, func(int) bool { return true }); err != nil {
			t.Fatal(err)
		}
	}
	m := New(discardStore{}, newTestSupervisor(t), ws, nil)
	m = send(m, keyMsg("0"))
	for _, id := range failed {
		m = send(m, exitedMsg{ID: id, Reason: "boom"})
	}
	return m
}

// newTestSupervisor keeps its run directory in the test's temp directory.
func newTestSupervisor(t *testing.T) *tunnel.Supervisor {
	t.Helper()
	supervisor, err := tunnel.NewSupervisor(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(supervisor.StopAll)
	return supervisor
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
	m := newLiveModel(t, 2, 3)
	m = send(m, keyMsg("j"))

	m = send(m, exitedMsg{ID: 1, Reason: "boom"})

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
