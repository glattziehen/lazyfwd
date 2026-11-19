package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

const (
	hostsPanelWidth    = 34
	minListenersHeight = 7
	// maxLiveRows caps the Live panel, which scrolls beyond it.
	maxLiveRows = 6
	// maxFooterLines is the footer's height at most: the selected forward's
	// command or failure, a message, and the key hints.
	maxFooterLines = 3
)

// Colours are ANSI 0-15, so the terminal theme decides the shades. Each
// colour has one meaning across the whole UI. Muted text (stopped forwards,
// hints, unfocused borders) is faint rather than ANSI 8 ("bright black"),
// which some themes, such as Solarized Dark, map to the background colour.
var (
	colorFocus    = lipgloss.Color("6") // focused panel and form field
	colorSelected = lipgloss.Color("4") // cursor row's background, as in lazygit
	colorRunning  = lipgloss.Color("2")
	colorFailed   = lipgloss.Color("1")
	colorPin      = lipgloss.Color("3")

	styleFocus   = lipgloss.NewStyle().Foreground(colorFocus)
	styleRunning = lipgloss.NewStyle().Foreground(colorRunning)
	styleFailed  = lipgloss.NewStyle().Foreground(colorFailed)
	styleMuted   = lipgloss.NewStyle().Faint(true)
	stylePin     = lipgloss.NewStyle().Foreground(colorPin)
	styleBold    = lipgloss.NewStyle().Bold(true)
	// Clickable local ends are underlined rather than coloured, so they stay
	// readable on the cursor row's background.
	styleLink = lipgloss.NewStyle().Underline(true)
	// styleSelected marks the cursor row of the focused panel and the chosen
	// button; an unfocused panel's cursor row is only bold, as in lazygit.
	styleSelected = lipgloss.NewStyle().Background(colorSelected)
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

// status shows a starting forward as running, since its connect is under way.
func (m Model) status(forward workspace.Forward) forwardStatus {
	switch m.state.Status(forward.ID).State {
	case tunnel.Starting, tunnel.Running:
		return statusRunning
	case tunnel.Failed:
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
	if m.quitDialog != nil {
		return lipgloss.JoinVertical(lipgloss.Left, m.quitView(bodyHeight), footer)
	}
	live := m.liveForwards()
	liveLines := m.forwardLines(live)
	if len(liveLines) == 0 {
		liveLines = []string{styleMuted.Render("no running or failed forwards")}
	}
	// The Live panel, at the bottom, fits its rows, up to maxLiveRows and a
	// third of the screen. The Forwards/Listening split assumes the tallest
	// Live panel and footer, so a start or stop, which changes both, moves
	// only the bottom edges above Live.
	splitHeight := m.height - maxFooterLines
	liveHeightMax := min(maxLiveRows+2, max(3, splitHeight/3))
	liveHeight := min(len(liveLines)+2, liveHeightMax)
	upperHeight := bodyHeight - liveHeight
	forwardsHeight := splitHeight - liveHeightMax - max(minListenersHeight, (splitHeight-liveHeightMax)/3)
	rightWidth := m.width - hostsPanelWidth
	host := m.selectedHost()
	hosts := m.panel(panelHosts, "", m.hostLines(), len(m.hosts()), m.hostIndex, hostsPanelWidth, upperHeight)
	forwards := m.panel(panelForwards, host, m.hostForwardLines(), len(m.forwards()), m.forwardIndex, rightWidth, forwardsHeight)
	listening := m.panel(panelListeners, m.listenersDetail(), m.listenerLines(), len(m.visibleListeners()), m.listenerIndex, rightWidth, upperHeight-forwardsHeight)
	livePanel := m.panel(panelLive, "all hosts", liveLines, len(live), m.liveCursor(live), m.width, liveHeight)
	right := lipgloss.JoinVertical(lipgloss.Left, forwards, listening)
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, hosts, right), livePanel, footer)
}

// panelNames are the panels' names, indexed by number key minus one.
var panelNames = [panelCount]string{"Hosts", "Forwards", "Listening", "Live"}

// panel draws a framed list that scrolls to keep the cursor row visible. Its
// title reads "[n]─Name · detail", where n is the panel's number key, and the
// bottom edge shows the cursor position among count items. The cursor row
// gets a background in the focused panel and bold in the others; lines beyond
// count, such as an empty-list hint, are never the cursor.
func (m Model) panel(which panel, detail string, lines []string, count, cursor, width, height int) string {
	isFocused := m.focus == which
	number, name, chrome := styleMuted, lipgloss.NewStyle(), styleMuted
	if isFocused {
		number, name, chrome = styleFocus, styleFocus.Bold(true), styleFocus
	}
	title := number.Render(fmt.Sprintf("[%d]", which)) + chrome.Render("─") + name.Render(panelNames[which-1])
	if detail != "" {
		title += styleMuted.Render(" · " + detail)
	}
	note := ""
	if count > 0 {
		note = styleMuted.Render(fmt.Sprintf("%d of %d", cursor+1, count))
	}
	rows := height - 2
	start := max(0, cursor-rows+1)
	selected := styleBold
	if isFocused {
		selected = styleSelected
	}
	var body []string
	for i := start; i < len(lines) && i < start+rows; i++ {
		line := " " + lines[i]
		if i == cursor && count > 0 {
			line = fill(line, selected, width-2)
		}
		body = append(body, line)
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

// fill pads line to width cells and paints it with style. Styled spans inside
// line end in an SGR reset, which would drop style from there on, so style is
// set again after each reset.
func fill(line string, style lipgloss.Style, width int) string {
	line = cut(line, width)
	line += repeat(" ", width-lipgloss.Width(line))
	open, _, _ := strings.Cut(style.Render(" "), " ")
	if open == "" {
		return line
	}
	return open + strings.ReplaceAll(line, sgrReset, sgrReset+open) + sgrReset
}

const sgrReset = "\x1b[0m"

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
		for _, forward := range m.state.ForwardsOf(host.Name) {
			counts[m.status(forward)]++
		}
		// "nuc ● 2  ✗ 1": two spaces keep the counts apart as groups.
		gap := " "
		for _, status := range []forwardStatus{statusRunning, statusFailed} {
			if counts[status] > 0 {
				line += gap + status.style().Render(fmt.Sprintf("%s %d", status.symbol(), counts[status]))
				gap = "  "
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
// widest local end; the arrow points from the listening end to the target.
func (m Model) forwardLines(forwards []workspace.Forward) []string {
	localWidth := 0
	for _, forward := range forwards {
		localWidth = max(localWidth, len(forward.LocalEnd().String()))
	}
	var lines []string
	for _, forward := range forwards {
		status := m.status(forward)
		local := forward.LocalEnd().String()
		link := local
		if url, isWeb := forward.LocalURL(); isWeb {
			link = hyperlink(url, styleLink.Render(local))
		}
		padding := strings.Repeat(" ", localWidth-len(local))
		arrow := "→"
		if forward.Direction == workspace.Remote {
			arrow = "←"
		}
		remote := styleMuted.Render(forward.Host+":") + forward.RemoteEnd().String()
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
			line += "  " + status.style().Render(status.symbol()+" → "+forward.LocalEnd().String())
		}
		lines = append(lines, line)
	}
	// Other users' ports include Docker's published ports, which belong to
	// root's docker-proxy, so the hidden count names Docker.
	if result.isOwnOnly && m.isShowingAllListeners {
		lines = append(lines, styleMuted.Render(host+" shows you only your own ports; other users' need root there"))
	} else if hidden := len(result.listeners) - len(m.visibleListeners()); hidden > 0 {
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
		label := styleMuted.Render(fmt.Sprintf("%-18s", m.form.labels[i]))
		if i == m.form.focus {
			label = styleFocus.Bold(true).Render(fmt.Sprintf("%-18s", m.form.labels[i]))
		}
		lines = append(lines, "  "+label+" "+input.View()+" ")
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
	if forward, ok := m.focusedForward(); ok && m.form == nil && m.quitDialog == nil {
		if status := m.state.Status(forward.ID); status.State == tunnel.Failed {
			lines = append(lines, styleFailed.Render("✗ "+status.Reason))
		} else {
			lines = append(lines, styleMuted.Render("$ ssh -N "+strings.Join(forward.SSHArgs(), " ")+" -- "+forward.Host))
		}
	}
	if m.message != "" {
		lines = append(lines, styleFailed.Render(m.message))
	}
	var hints []string
	for _, hint := range m.keyHints() {
		hints = append(hints, styleBold.Render(hint.key)+" "+styleMuted.Render(hint.action))
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
	case m.quitDialog != nil:
		return []keyHint{{"enter", "choose"}, {"y", "quit"}, {"n/esc", "cancel"}, {"h/l", "move"}}
	case m.focus == panelLive:
		return []keyHint{{"?", "help"}, {"j/k", "move"}, {"space", "start/stop"}, {"enter", "go to"}, {"o", "open"}, {"e", "edit"}, {"d", "delete"}, {"h/l", "panels"}, {"q", "quit"}}
	case m.focus == panelHosts:
		return []keyHint{{"?", "help"}, {"j/k", "move"}, {"enter", "forwards"}, {"a", "add host"}, {"p", "pin"}, {"d", "remove"}, {"h/l", "panels"}, {"q", "quit"}}
	case m.focus == panelListeners:
		return []keyHint{{"?", "help"}, {"j/k", "move"}, {"space", "forward/start/stop"}, {"o", "open"}, {"t", "yours/all"}, {"r", "refresh"}, {"a", "local fwd"}, {"h/l", "panels"}, {"q", "quit"}}
	default:
		return []keyHint{{"?", "help"}, {"j/k", "move"}, {"space", "start/stop"}, {"a", "local fwd"}, {"A", "remote fwd"}, {"e", "edit"}, {"d", "delete"}, {"o", "open"}, {"q", "quit"}}
	}
}
