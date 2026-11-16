package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Key repeat over a slow connection delivers several keys in one message.
func TestBurstOfMovementKeysMovesOncePerKey(t *testing.T) {
	m := newLiveModel(t, 1, 2, 3)

	m = send(m, keyMsg("jj"))

	if got := selectedLiveID(m); got != 3 {
		t.Fatalf("selected forward %d, want 3", got)
	}
}

// A terminal without bracketed paste delivers a paste as a burst like fast
// typing, so a burst never runs a key that acts.
func TestBurstWithActingKeysDoesNothing(t *testing.T) {
	for _, burst := range []tea.KeyMsg{
		keyMsg("dd"),
		keyMsg("jd"),
		{Type: tea.KeyRunes, Runes: []rune("jj"), Paste: true},
	} {
		m := newLiveModel(t, 1, 2, 3)

		m = send(m, burst)

		if len(m.workspace.Forwards) != 3 || selectedLiveID(m) != 1 {
			t.Errorf("%q (paste %v): %d forwards left, selected %d", string(burst.Runes), burst.Paste, len(m.workspace.Forwards), selectedLiveID(m))
		}
	}
}
