package tui

import (
	"testing"

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
