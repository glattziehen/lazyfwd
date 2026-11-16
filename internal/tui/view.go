package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

const (
	hostsPanelWidth    = 34
	minListenersHeight = 7
	// maxLiveRows caps the Live panel, which scrolls beyond it.
	maxLiveRows = 6
)

// Colours are ANSI 0-15, so the terminal theme decides the shades. Each
// colour has one meaning across the whole UI. Muted text (stopped forwards,
// hints, unfocused borders) is faint rather than ANSI 8 ("bright black"),
// which some themes, such as Solarized Dark, map to the background colour.
var (
	colorFocus   = lipgloss.Color("6") // focused panel, cursor, and form field
	colorRunning = lipgloss.Color("2")
	colorFailed  = lipgloss.Color("1")
	colorLink    = lipgloss.Color("4") // clickable laptop ends
	colorPin     = lipgloss.Color("3")

	styleFocus   = lipgloss.NewStyle().Foreground(colorFocus)
	styleRunning = lipgloss.NewStyle().Foreground(colorRunning)
	styleFailed  = lipgloss.NewStyle().Foreground(colorFailed)
	styleMuted   = lipgloss.NewStyle().Faint(true)
	styleLink    = lipgloss.NewStyle().Foreground(colorLink)
	stylePin     = lipgloss.NewStyle().Foreground(colorPin)
	styleBold    = lipgloss.NewStyle().Bold(true)
)

// forwardStatus is what the UI shows for a forward; each status has one
// symbol and one colour.
type forwardStatus int

const (
	statusStopped forwardStatus = iota
	statusRunning
	statusFailed
)

func (s forwardStatus) style() lipgloss.Style {
	switch s {
	case statusRunning:
		return styleRunning
	case statusFailed:
		return styleFailed
	}
	return styleMuted
}

func (s forwardStatus) symbol() string {
	switch s {
	case statusRunning:
		return "●"
	case statusFailed:
		return "✗"
	}
	return "○"
}

func (m Model) status(forward workspace.Forward) forwardStatus {
	if m.supervisor.IsRunning(forward.ID) {
		return statusRunning
	}
	if _, ok := m.failures[forward.ID]; ok {
		return statusFailed
	}
	return statusStopped
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	footer := m.footer()
	bodyHeight := m.height - lipgloss.Height(footer)
	if m.form != nil {
		return lipgloss.JoinVertical(lipgloss.Left, m.formView(bodyHeight), footer)
	}
	if m.isShowingHelp {
		return lipgloss.JoinVertical(lipgloss.Left, m.helpView(bodyHeight), footer)
	}
	live := m.liveForwards()
	liveLines := m.forwardLines(live)
	if len(liveLines) == 0 {
		liveLines = []string{styleMuted.Render("no running or failed forwards")}
	}
	// The Live panel fits its rows, up to maxLiveRows and a third of the screen.
	liveHeight := min(min(len(liveLines), maxLiveRows)+2, max(3, bodyHeight/3))
	lowerHeight := bodyHeight - liveHeight
	rightWidth := m.width - hostsPanelWidth
	listenersHeight := max(minListenersHeight, lowerHeight/3)
	host := m.selectedHost()
	livePanel := m.panel(panelLive, "all hosts", liveLines, len(live), m.liveCursor(live), m.width, liveHeight)
	hosts := m.panel(panelHosts, "", m.hostLines(), len(m.hosts()), m.hostIndex, hostsPanelWidth, lowerHeight)
	forwards := m.panel(panelForwards, host, m.hostForwardLines(), len(m.forwards()), m.forwardIndex, rightWidth, lowerHeight-listenersHeight)
	listening := m.panel(panelListeners, m.listenersDetail(), m.listenerLines(), len(m.visibleListeners()), m.listenerIndex, rightWidth, listenersHeight)
	right := lipgloss.JoinVertical(lipgloss.Left, forwards, listening)
	return lipgloss.JoinVertical(lipgloss.Left, livePanel, lipgloss.JoinHorizontal(lipgloss.Top, hosts, right), footer)
}

var panelNames = [panelCount]string{"Live", "Hosts", "Forwards", "Listening"}

// panel draws a framed list that scrolls to keep the cursor row visible. Its
// title reads "[n]─Name · detail", where n is the panel's number key, and the bottom edge shows the cursor
// position among count items. The cursor is a marker column, since reverse
// video breaks at the first styled cell inside a row; lines beyond count, such
// as an empty-list hint, get no marker.
func (m Model) panel(which panel, detail string, lines []string, count, cursor, width, height int) string {
	isFocused := m.focus == which
	number, name, chrome := styleMuted, lipgloss.NewStyle(), styleMuted
	if isFocused {
		number, name, chrome = styleFocus, styleFocus.Bold(true), styleFocus
	}
	title := number.Render(fmt.Sprintf("[%d]", which)) + chrome.Render("─") + name.Render(panelNames[which])
	if detail != "" {
		title += styleMuted.Render(" · " + detail)
	}
	note := ""
	if count > 0 {
		note = styleMuted.Render(fmt.Sprintf("%d of %d", cursor+1, count))
	}
	rows := height - 2
	start := max(0, cursor-rows+1)
	var body []string
	for i := start; i < len(lines) && i < start+rows; i++ {
		marker := "  "
		if i == cursor && count > 0 {
			marker = chrome.Render("▸ ")
		}
		body = append(body, marker+lines[i])
	}
	return frame(title, note, body, chrome, width, height)
}

// frame draws a rounded border of width x height cells around lines, with
// title set into the top edge and note into the bottom edge, as lazygit does.
// Lines and title are cut to fit.
func frame(title, note string, lines []string, chrome lipgloss.Style, width, height int) string {
	innerWidth := width - 2
	title = cut(title, innerWidth-2)
	note = cut(note, innerWidth-2)
	out := []string{chrome.Render("╭─") + title + chrome.Render(repeat("─", innerWidth-1-lipgloss.Width(title))+"╮")}
	for i := range height - 2 {
		line := ""
		if i < len(lines) {
			line = cut(lines[i], innerWidth)
		}
		out = append(out, chrome.Render("│")+line+repeat(" ", innerWidth-lipgloss.Width(line))+chrome.Render("│"))
	}
	out = append(out, chrome.Render("╰"+repeat("─", innerWidth-1-lipgloss.Width(note)))+note+chrome.Render("─╯"))
	return strings.Join(out, "\n")
}

// repeat is strings.Repeat that yields "" for a negative count, which frame
// computes on a terminal narrower than its panels.
func repeat(s string, count int) string {
	return strings.Repeat(s, max(0, count))
}

// cut truncates a styled string to width cells.
func cut(s string, width int) string {
	return lipgloss.NewStyle().MaxWidth(max(0, width)).Render(s)
}

func (m Model) hostLines() []string {
	var lines []string
	for _, host := range m.hosts() {
		mark := "  "
		if host.IsPinned {
			mark = stylePin.Render("*") + " "
		}
		line := mark + host.Name
		counts := map[forwardStatus]int{}
		for _, forward := range m.workspace.ForwardsOf(host.Name) {
			counts[m.status(forward)]++
		}
		for _, status := range []forwardStatus{statusRunning, statusFailed} {
			if counts[status] > 0 {
				line += " " + status.style().Render(fmt.Sprintf("%s%d", status.symbol(), counts[status]))
			}
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, styleMuted.Render("no hosts: press a to add one"))
	}
	return lines
}

// forwardLines renders one row per forward and aligns the arrows after the
// widest laptop end; the arrow points from the listening end to the target.
func (m Model) forwardLines(forwards []workspace.Forward) []string {
	laptopWidth := 0
	for _, forward := range forwards {
		laptopWidth = max(laptopWidth, len(forward.LaptopAddr().String()))
	}
	var lines []string
	for _, forward := range forwards {
		status := m.status(forward)
		laptop := forward.LaptopAddr().String()
		link := laptop
		if url, isWeb := forward.LaptopURL(); isWeb {
			link = hyperlink(url, styleLink.Render(laptop))
		}
		padding := strings.Repeat(" ", laptopWidth-len(laptop))
		arrow := "→"
		if forward.Direction == workspace.Remote {
			arrow = "←"
		}
		remote := styleMuted.Render(forward.Host+":") + forward.RemoteAddr().String()
		lines = append(lines, fmt.Sprintf("%s %s%s %s %s", status.style().Render(status.symbol()), link, padding, styleMuted.Render(arrow), remote))
	}
	return lines
}

func (m Model) hostForwardLines() []string {
	lines := m.forwardLines(m.forwards())
	if len(lines) == 0 && m.selectedHost() != "" {
		lines = append(lines, styleMuted.Render("no forwards: press a (local) or A (remote) to add one"))
	}
	return lines
}

func (m Model) listenersDetail() string {
	scope := "yours"
	if m.isShowingAllListeners {
		scope = "all"
	}
	if m.selectedHost() == "" {
		return scope
	}
	return m.selectedHost() + " · " + scope
}

func (m Model) listenerLines() []string {
	host := m.selectedHost()
	result, ok := m.listenerResults[host]
	switch {
	case host == "":
		return nil
	case !ok:
		return []string{styleMuted.Render("press r to list listening ports")}
	case result.isLoading:
		return []string{styleMuted.Render("asking " + host + "...")}
	case result.err != nil:
		return []string{styleFailed.Render(result.err.Error()), styleMuted.Render("add forwards by hand with a")}
	}
	var lines []string
	for _, listener := range m.visibleListeners() {
		process := listener.Process
		if process == "" {
			process = styleMuted.Render("-")
		}
		line := fmt.Sprintf("%-6d %s", listener.Port, process)
		if forward, ok := m.forwardTo(listener.Port); ok {
			status := m.status(forward)
			line += "  " + status.style().Render(status.symbol()+" → "+forward.LaptopAddr().String())
		}
		lines = append(lines, line)
	}
	// Other users' ports include Docker's published ports, which belong to
	// root's docker-proxy, so the hidden count names Docker.
	if hidden := len(result.listeners) - len(m.visibleListeners()); hidden > 0 {
		lines = append(lines, styleMuted.Render(fmt.Sprintf("%d more of other users, such as Docker: t shows all", hidden)))
	} else if len(lines) == 0 {
		lines = append(lines, styleMuted.Render("no listening ports"))
	}
	return lines
}

// hyperlink wraps text in an OSC 8 escape, which most terminals make clickable.
func hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func (m Model) formView(height int) string {
	lines := []string{""}
	for i, input := range m.form.inputs {
		marker, label := "  ", styleMuted.Render(fmt.Sprintf("%-18s", m.form.labels[i]))
		if i == m.form.focus {
			marker, label = styleFocus.Render("▸ "), styleFocus.Bold(true).Render(fmt.Sprintf("%-18s", m.form.labels[i]))
		}
		lines = append(lines, " "+marker+label+" "+input.View()+" ")
	}
	lines = append(lines, "")
	return m.modal(styleFocus.Bold(true).Render(m.form.title), lines, height)
}

func (m Model) helpView(height int) string {
	var lines []string
	panel := ""
	for _, row := range keyHelpRows {
		label := ""
		if row.panel != panel {
			panel, label = row.panel, row.panel
		}
		lines = append(lines, fmt.Sprintf(" %s  %s  %s ", styleMuted.Render(fmt.Sprintf("%-9s", label)), styleBold.Render(fmt.Sprintf("%-22s", row.keys)), row.action))
	}
	return m.modal(styleFocus.Bold(true).Render("Keys"), lines, height)
}

// modal centres a focused frame that fits lines.
func (m Model) modal(title string, lines []string, height int) string {
	width := lipgloss.Width(title) + 4
	for _, line := range lines {
		width = max(width, lipgloss.Width(line)+2)
	}
	box := frame(title, "", lines, styleFocus, min(width, m.width), len(lines)+2)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

// footer shows the selected forward's ssh command or failure, the last message, and the keys.
func (m Model) footer() string {
	var lines []string
	if forward, ok := m.focusedForward(); ok && m.form == nil {
		if reason, failed := m.failures[forward.ID]; failed {
			lines = append(lines, styleFailed.Render("✗ ssh failed: "+reason))
		} else {
			lines = append(lines, styleMuted.Render("$ ssh -N "+strings.Join(forward.SSHArgs(), " ")+" -- "+forward.Host))
		}
	}
	if m.message != "" {
		lines = append(lines, styleFailed.Render(m.message))
	}
	var hints []string
	for _, hint := range m.keyHints() {
		hints = append(hints, hint.key+" "+styleMuted.Render(hint.action))
	}
	lines = append(lines, strings.Join(hints, styleMuted.Render(" · ")))
	return lipgloss.NewStyle().MaxWidth(m.width).Render(strings.Join(lines, "\n"))
}

// keyHint is one entry of the footer's key line. "? help" leads each line, so
// it stays visible when a narrow terminal cuts the end.
type keyHint struct {
	key    string
	action string
}

func (m Model) keyHints() []keyHint {
	switch {
	case m.form != nil:
		return []keyHint{{"enter", "save"}, {"esc", "cancel"}, {"tab", "next field"}}
	case m.isShowingHelp:
		return []keyHint{{"any key", "closes the help"}}
	case m.focus == panelLive:
		return []keyHint{{"?", "help"}, {"j/k", "move"}, {"space", "start/stop"}, {"enter", "go to"}, {"o", "open"}, {"e", "edit"}, {"d", "delete"}, {"h/l", "panels"}, {"q", "quit"}}
	case m.focus == panelHosts:
		return []keyHint{{"?", "help"}, {"j/k", "move"}, {"enter", "forwards"}, {"a", "add host"}, {"p", "pin"}, {"d", "remove"}, {"h/l", "panels"}, {"q", "quit"}}
	case m.focus == panelListeners:
		return []keyHint{{"?", "help"}, {"j/k", "move"}, {"space", "forward/start/stop"}, {"t", "yours/all"}, {"r", "refresh"}, {"a", "local fwd"}, {"h/l", "panels"}, {"q", "quit"}}
	default:
		return []keyHint{{"?", "help"}, {"j/k", "move"}, {"space", "start/stop"}, {"a", "local fwd"}, {"A", "remote fwd"}, {"e", "edit"}, {"d", "delete"}, {"o", "open"}, {"q", "quit"}}
	}
}
