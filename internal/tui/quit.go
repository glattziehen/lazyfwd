package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// quitDialog asks before quitting, with Yes selected unless isNoSelected, so
// q then enter quits.
type quitDialog struct {
	isNoSelected bool
}

// updateQuitDialog answers the dialog. Only single keys count: a burst or a
// paste, even one holding y, leaves it open.
func (m Model) updateQuitDialog(msg tea.KeyMsg) (Model, tea.Cmd) {
	if msg.Paste || len(msg.Runes) > 1 {
		return m, nil
	}
	switch msg.String() {
	case "y", "q", "ctrl+c":
		return m.quit()
	case "n", "esc":
		m.quitDialog = nil
	case "enter":
		if m.quitDialog.isNoSelected {
			m.quitDialog = nil
			return m, nil
		}
		return m.quit()
	case "h", "l", "left", "right", "tab", "shift+tab":
		m.quitDialog = &quitDialog{isNoSelected: !m.quitDialog.isNoSelected}
	}
	return m, nil
}

// quit stops every forward, which closes every ssh connection, and ends the
// program.
func (m Model) quit() (Model, tea.Cmd) {
	m.supervisor.StopAll()
	return m, tea.Quit
}

func (m Model) quitView(height int) string {
	lines := []string{""}
	switch running := m.runningCount(); running {
	case 0:
		lines = append(lines, "  No forwards are running.  ")
	case 1:
		lines = append(lines, "  1 running forward will stop.  ")
	default:
		lines = append(lines, fmt.Sprintf("  %d running forwards will stop.  ", running))
	}
	yes, no := styleFocus.Bold(true).Render("▸ Yes"), "  No"
	if m.quitDialog.isNoSelected {
		yes, no = "  Yes", styleFocus.Bold(true).Render("▸ No")
	}
	lines = append(lines, "", "  "+yes+"    "+no+"  ", "")
	return m.modal(styleFocus.Bold(true).Render("Quit lazyfwd?"), lines, height)
}

func (m Model) runningCount() int {
	count := 0
	for _, forward := range m.workspace.Forwards {
		if m.supervisor.IsRunning(forward.ID) {
			count++
		}
	}
	return count
}
