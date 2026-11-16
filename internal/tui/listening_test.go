package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moritzwilksch/lazyfwd/internal/listeners"
	"github.com/moritzwilksch/lazyfwd/internal/tunnel"
	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

// Docker's published ports belong to root's docker-proxy, so the default
// filter hides them; the panel must say how to see them.
func TestListeningPanelNamesPortsHiddenByYoursFilter(t *testing.T) {
	m := New(discardStore{}, tunnel.NewSupervisor(), workspace.Workspace{}, []string{"box"})
	m = send(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	found := []listeners.Listener{{Port: 3000, Process: "node"}, {Port: 5432}, {Port: 8080}}
	m = send(m, listenersMsg{host: "box", result: listenerResult{listeners: found}})
	if view := m.View(); !strings.Contains(view, "2 more of other users, such as Docker: t shows all") {
		t.Fatalf("hint missing from view:\n%s", view)
	}
}
