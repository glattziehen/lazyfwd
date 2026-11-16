package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moritzwilksch/lazyfwd/internal/listeners"
	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

// Docker's published ports belong to root's docker-proxy, so the default
// filter hides them; the panel must say how to see them.
func TestListeningPanelNamesPortsHiddenByYoursFilter(t *testing.T) {
	m := New(discardStore{}, newTestSupervisor(t), workspace.Workspace{}, []string{"box"})
	m = send(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	found := []listeners.Listener{{Port: 3000, Process: "node"}, {Port: 5432}, {Port: 8080}}
	m = send(m, listenersMsg{host: "box", result: listenerResult{listeners: found}})
	if view := m.View(); !strings.Contains(view, "2 more of other users, such as Docker: t shows all") {
		t.Fatalf("hint missing from view:\n%s", view)
	}
}

// newListeningModel focuses the Listening panel of host box, which listens on
// 5432 (forwarded) and 8080 (not forwarded).
func newListeningModel(t *testing.T) Model {
	t.Helper()
	var ws workspace.Workspace
	target := workspace.End{Host: workspace.DefaultTargetHost, Port: 5432}
	if _, err := ws.AddForwards("box", workspace.Local, []workspace.End{target}, workspace.End{}, func(int) bool { return true }); err != nil {
		t.Fatal(err)
	}
	m := New(discardStore{}, newTestSupervisor(t), ws, nil)
	found := []listeners.Listener{{Port: 5432, Process: "postgres"}, {Port: 8080, Process: "node"}}
	m = send(m, listenersMsg{host: "box", result: listenerResult{listeners: found}})
	return send(m, keyMsg("3"))
}

// o on a forwarded port runs the Forwards panel's open, which refuses a
// non-web port before it reaches the browser.
func TestListeningOpenUsesThePortsForward(t *testing.T) {
	m := send(newListeningModel(t), keyMsg("o"))
	if !strings.Contains(m.message, "is not a web port") {
		t.Fatalf("message = %q", m.message)
	}
}

func TestListeningOpenOnUnforwardedPortAsksToForwardFirst(t *testing.T) {
	m := newListeningModel(t)
	m = send(send(m, keyMsg("j")), keyMsg("o"))
	if m.message != "port 8080 has no forward yet; press enter to forward it" || len(m.workspace.Forwards) != 1 {
		t.Fatalf("message = %q, forwards = %d", m.message, len(m.workspace.Forwards))
	}
}
