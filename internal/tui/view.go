package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

const hostsPanelWidth = 34

var (
	colorAccent = lipgloss.Color("6")
	colorMuted  = lipgloss.Color("8")
	colorOK     = lipgloss.Color("2")
	colorError  = lipgloss.Color("1")

	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleOK       = lipgloss.NewStyle().Foreground(colorOK)
	styleError    = lipgloss.NewStyle().Foreground(colorError)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleCursor   = lipgloss.NewStyle().Bold(true)
)

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	footer := m.footer()
	bodyHeight := m.height - lipgloss.Height(footer)
	if m.form != nil {
		return lipgloss.JoinVertical(lipgloss.Left, m.formView(bodyHeight), footer)
	}
	hosts := m.panel("[1] Hosts", m.hostLines(), m.hostIndex, panelHosts, hostsPanelWidth, bodyHeight)
	forwards := m.panel("[2] Forwards: "+m.selectedHost(), m.forwardLines(), m.forwardIndex, panelForwards, m.width-hostsPanelWidth, bodyHeight)
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, hosts, forwards), footer)
}

// panel draws a bordered list that scrolls to keep the cursor row visible.
func (m Model) panel(title string, lines []string, cursor int, which panel, width, height int) string {
	border := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorMuted)
	if m.focus == which {
		border = border.BorderForeground(colorAccent)
	}
	innerWidth, rows := width-2, height-3
	start := max(0, cursor-rows+1)
	body := []string{styleTitle.Render(title)}
	for i := start; i < len(lines) && i < start+rows; i++ {
		line := lipgloss.NewStyle().Width(innerWidth).MaxWidth(innerWidth).Render(lines[i])
		switch {
		case i == cursor && m.focus == which:
			line = styleSelected.Render(line)
		case i == cursor:
			line = styleCursor.Render(line)
		}
		body = append(body, line)
	}
	return border.Width(innerWidth).Height(height - 2).Render(strings.Join(body, "\n"))
}

func (m Model) hostLines() []string {
	var lines []string
	for _, host := range m.hosts() {
		mark := "  "
		if host.IsPinned {
			mark = "* "
		}
		line := mark + host.Name
		if running := m.runningCount(host.Name); running > 0 {
			line += styleOK.Render(fmt.Sprintf(" (%d up)", running))
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, styleMuted.Render("no hosts: press a to add one"))
	}
	return lines
}

func (m Model) runningCount(host string) int {
	count := 0
	for _, forward := range m.workspace.ForwardsOf(host) {
		if m.supervisor.IsRunning(forward.ID) {
			count++
		}
	}
	return count
}

func (m Model) forwardLines() []string {
	var lines []string
	for _, forward := range m.forwards() {
		laptop := forward.LaptopAddr().String()
		link := hyperlink(forward.LaptopURL(), laptop)
		padding := strings.Repeat(" ", max(1, 22-len(laptop)))
		arrow := "->"
		if forward.Direction == workspace.Remote {
			arrow = "<-"
		}
		remote := forward.Host + ":" + forward.RemoteAddr().String()
		lines = append(lines, fmt.Sprintf("%s %s %s%s%s  %s", m.statusDot(forward), directionFlag(forward), link, padding, arrow, remote))
	}
	if len(lines) == 0 && m.selectedHost() != "" {
		lines = append(lines, styleMuted.Render("no forwards: press a (local) or A (remote) to add one"))
	}
	return lines
}

func (m Model) statusDot(forward workspace.Forward) string {
	if m.supervisor.IsRunning(forward.ID) {
		return styleOK.Render("●")
	}
	if _, ok := m.failures[forward.ID]; ok {
		return styleError.Render("✗")
	}
	return styleMuted.Render("○")
}

func directionFlag(forward workspace.Forward) string {
	if forward.Direction == workspace.Remote {
		return "R"
	}
	return "L"
}

// hyperlink wraps text in an OSC 8 escape, which most terminals make clickable.
func hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func (m Model) formView(height int) string {
	lines := []string{styleTitle.Render(m.form.title), ""}
	for i, input := range m.form.inputs {
		label := fmt.Sprintf("%-18s", m.form.labels[i])
		if i == m.form.focus {
			label = styleTitle.Foreground(colorAccent).Render(label)
		}
		lines = append(lines, label+" "+input.View())
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(1, 2)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box.Render(strings.Join(lines, "\n")))
}

// footer shows the selected forward's ssh command or failure, the last message, and the keys.
func (m Model) footer() string {
	var lines []string
	if forward, ok := m.selectedForward(); ok && m.form == nil && m.focus == panelForwards {
		if reason, failed := m.failures[forward.ID]; failed {
			lines = append(lines, styleError.Render("ssh failed: "+reason))
		} else {
			lines = append(lines, styleMuted.Render("ssh "+strings.Join(forward.SSHArgs(), " ")))
		}
	}
	if m.message != "" {
		lines = append(lines, styleError.Render(m.message))
	}
	lines = append(lines, styleMuted.Render(m.keyHelp()))
	return lipgloss.NewStyle().MaxWidth(m.width).Render(strings.Join(lines, "\n"))
}

func (m Model) keyHelp() string {
	switch {
	case m.form != nil:
		return "enter save · esc cancel · tab next field"
	case m.focus == panelHosts:
		return "j/k move · enter forwards · a add host · p pin · d remove · A remote fwd · q quit"
	default:
		return "j/k move · space start/stop · a local fwd · A remote fwd · e edit · d delete · o open · tab hosts · q quit"
	}
}
