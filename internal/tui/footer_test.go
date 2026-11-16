package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// lazyfwd's own reasons, such as a port in use, are not ssh's: the footer
// must not blame ssh for them.
func TestFooterShowsFailureReasonWithoutBlamingSSH(t *testing.T) {
	m := newLiveModel(t)
	m = send(m, tea.WindowSizeMsg{Width: 120, Height: 30})

	m = send(m, exitedMsg{ID: 1, Reason: "localhost:8080 is already in use"})

	footer := m.footer()
	if !strings.Contains(footer, "✗ localhost:8080 is already in use") || strings.Contains(footer, "ssh failed") {
		t.Fatalf("footer:\n%s", footer)
	}
}
