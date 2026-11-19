package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// lazyfwd's own reasons, such as a port in use, are not ssh's: the Details
// pane must not blame ssh for them.
func TestDetailsShowFailureReasonWithoutBlamingSSH(t *testing.T) {
	m, tunnels := newLiveModelWithTunnels(t)
	m = send(m, tea.WindowSizeMsg{Width: 120, Height: 30})

	m = fail(m, tunnels, 1, "localhost:8080 is already in use")
	m = send(m, keyMsg("4"))

	title, lines := m.details(80)
	details := stripANSI(strings.Join(lines, "\n"))
	if title != "Forward" || !strings.Contains(details, "localhost:8080 is already in use") || strings.Contains(details, "ssh failed") {
		t.Fatalf("%s:\n%s", title, details)
	}
}

// h and l step through the panels in the order they stack, top to bottom.
func TestPanelsStackInKeyOrder(t *testing.T) {
	m := send(newLiveModel(t), tea.WindowSizeMsg{Width: 120, Height: 35})

	tops := panelTops(m.View())

	order := []string{"[1]─Hosts", "[2]─Forwards", "[3]─Listening", "[4]─Live"}
	for i := 1; i < len(order); i++ {
		if tops[order[i]] <= tops[order[i-1]] {
			t.Fatalf("%s is not below %s: %v", order[i], order[i-1], tops)
		}
	}
}
