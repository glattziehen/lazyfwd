package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/glattziehen/lazyfwd/internal/listeners"
	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// detailsView is the pane right of the side column. It describes what the
// cursor of the focused panel is on, as lazygit's main view does, and falls
// back to the selected host when that panel's list is empty.
func (m Model) detailsView(width, height int) string {
	// One space of margin on each side of the border's inside.
	textWidth := width - 4
	title, lines := m.details(textWidth)
	padded := make([]string, len(lines))
	for i, line := range lines {
		padded[i] = " " + cut(line, textWidth)
	}
	return frame(styleBold.Render(title), "", padded, styleMuted, width, height)
}

// details is the Details pane's title and lines, wrapped to width.
func (m Model) details(width int) (string, []string) {
	switch m.focus {
	case panelLive:
		if forward, ok := m.selectedLive(); ok {
			return "Forward", m.forwardDetails(forward, width)
		}
		return "Live", []string{styleMuted.Render("no running or failed forwards")}
	case panelForwards:
		if forward, ok := m.selectedForward(); ok {
			return "Forward", m.forwardDetails(forward, width)
		}
	case panelListeners:
		if listener, ok := m.selectedListener(); ok {
			return "Port", m.listenerDetails(listener)
		}
	}
	host, ok := m.selectedHostEntry()
	if !ok {
		return "Host", []string{styleMuted.Render("no hosts: press a to add one")}
	}
	return "Host", m.hostDetails(host)
}

func (m Model) forwardDetails(forward workspace.Forward, width int) []string {
	status := m.state.Status(forward.ID)
	shown := m.status(forward)
	direction := "local (-L), listens on this machine"
	if forward.Direction == workspace.Remote {
		direction = "remote (-R), listens on " + forward.Host
	}
	lines := []string{
		shown.style().Bold(true).Render(shown.symbol() + " " + string(status.State)),
		"",
		field("local", localLink(forward)),
		field("remote", styleMuted.Render(forward.Host+":")+forward.RemoteEnd().String()),
		field("direction", direction),
	}
	if url, isWeb := forward.LocalURL(); isWeb {
		lines = append(lines, field("open", hyperlink(url, styleLink.Render(url))))
	}
	if status.State == tunnel.Failed {
		lines = append(lines, "")
		lines = append(lines, wrap(styleFailed, status.Reason, width)...)
	}
	lines = append(lines, "")
	lines = append(lines, wrap(styleMuted, "$ ssh -N "+strings.Join(forward.SSHArgs(), " ")+" -- "+forward.Host, width)...)
	return lines
}

func (m Model) listenerDetails(listener listeners.Listener) []string {
	process := listener.Process
	if process == "" {
		process = styleMuted.Render("another user's, or unnamed")
	}
	forwarded := styleMuted.Render("no: space forwards it")
	if forward, ok := m.forwardTo(listener.Port); ok {
		status := m.status(forward)
		forwarded = status.style().Render(status.symbol()) + " " + localLink(forward)
	}
	return []string{
		styleBold.Render(fmt.Sprintf("%d", listener.Port)) + styleMuted.Render(" on "+m.selectedHost()),
		"",
		field("process", process),
		field("forwarded", forwarded),
	}
}

func (m Model) hostDetails(host workspace.Host) []string {
	var sources []string
	if host.IsInSSHConfig {
		sources = append(sources, "ssh config")
	}
	if host.IsManual {
		sources = append(sources, "added in lazyfwd")
	}
	name := styleBold.Render(host.Name)
	if host.IsPinned {
		name += " " + stylePin.Render("* pinned")
	}
	counts := map[forwardStatus]int{}
	forwards := m.state.ForwardsOf(host.Name)
	for _, forward := range forwards {
		counts[m.status(forward)]++
	}
	summary := styleMuted.Render("none")
	if len(forwards) > 0 {
		var parts []string
		for _, status := range []forwardStatus{statusRunning, statusFailed, statusStopped} {
			parts = append(parts, status.style().Render(fmt.Sprintf("%s %d", status.symbol(), counts[status])))
		}
		summary = strings.Join(parts, "  ")
	}
	lines := []string{name, ""}
	if len(sources) > 0 {
		lines = append(lines, field("from", strings.Join(sources, ", ")))
	}
	return append(lines, field("forwards", summary), field("listening", m.listeningSummary(host.Name)))
}

func (m Model) listeningSummary(host string) string {
	result, ok := m.listenerResults[host]
	switch {
	case !ok:
		return styleMuted.Render("not asked yet: press r")
	case result.isLoading:
		return styleMuted.Render("asking...")
	case result.err != nil:
		return styleFailed.Render("could not list")
	}
	return fmt.Sprintf("%d ports, %d yours", len(result.listeners), len(listeners.Owned(result.listeners)))
}

// field is one "label  value" line of the Details pane.
func field(label, value string) string {
	return styleMuted.Render(fmt.Sprintf("%-10s", label)) + " " + value
}

// wrap breaks text into lines of at most width cells, styled.
func wrap(style lipgloss.Style, text string, width int) []string {
	return strings.Split(style.Width(max(1, width)).Render(text), "\n")
}
