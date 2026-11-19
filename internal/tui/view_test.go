package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// A styled span inside the cursor row ends in a reset; the background must
// carry on past it to the row's end.
func TestFillKeepsBackgroundPastStyledSpans(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })

	got := fill(" "+styleRunning.Render("●")+" a", styleSelected, 6)

	want := "\x1b[44m \x1b[32m●\x1b[0m\x1b[44m a  \x1b[0m"
	if got != want {
		t.Fatalf("fill = %q, want %q", got, want)
	}
}

// Every panel but Hosts and Live shows only the selected host, so the Hosts
// cursor row keeps a background while another panel has focus.
func TestSelectedHostKeepsBackgroundWhenUnfocused(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := send(newLiveModel(t), tea.WindowSizeMsg{Width: 100, Height: 20})

	m = send(m, keyMsg("2"))

	row := strings.Split(m.View(), "\n")[1]
	if !strings.Contains(row, "48;5;") {
		t.Fatalf("selected host row has no background: %q", row)
	}
}
