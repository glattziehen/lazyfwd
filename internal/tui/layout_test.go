package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTabCyclesHostsForwardsListeningLive(t *testing.T) {
	m := newLiveModel(t)
	m = send(m, keyMsg("1"))
	var order []panel
	for range 4 {
		m = send(m, tea.KeyMsg{Type: tea.KeyTab})
		order = append(order, m.focus)
	}
	want := []panel{panelForwards, panelListeners, panelLive, panelHosts}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("tab order %v, want %v", order, want)
		}
	}
}

func TestKeyFourFocusesLiveAndZeroDoesNothing(t *testing.T) {
	m := newLiveModel(t)
	m = send(m, keyMsg("1"))

	m = send(m, keyMsg("0"))
	if m.focus != panelHosts {
		t.Fatalf("0 moved focus to %d", m.focus)
	}
	m = send(m, keyMsg("4"))
	if m.focus != panelLive {
		t.Fatalf("4 focused %d, want Live", m.focus)
	}
}

// escapeSequence matches SGR styling and OSC 8 hyperlinks.
var escapeSequence = regexp.MustCompile(`\x1b\[[0-9;]*m|\x1b\]8;;[^\x1b]*\x1b\\`)

func stripANSI(text string) string {
	return escapeSequence.ReplaceAllString(text, "")
}

// panelTops is the screen row of each numbered panel's top edge.
func panelTops(view string) map[string]int {
	tops := map[string]int{}
	for row, line := range strings.Split(view, "\n") {
		for _, title := range []string{"[1]─Hosts", "[2]─Forwards", "[3]─Listening", "[4]─Live"} {
			if strings.Contains(stripANSI(line), title) {
				tops[title] = row
			}
		}
	}
	return tops
}

// Starting or stopping a forward changes Live's height; the panels above it
// must not jump.
func TestLiveHeightMovesNoOtherPanelsTop(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 35}, {Width: 80, Height: 24}} {
		empty := send(newLiveModel(t), size)
		full := send(newLiveModel(t, 1, 2, 3), size)

		emptyTops, fullTops := panelTops(empty.View()), panelTops(full.View())

		for _, title := range []string{"[1]─Hosts", "[2]─Forwards", "[3]─Listening"} {
			if _, ok := emptyTops[title]; !ok || emptyTops[title] != fullTops[title] {
				t.Errorf("%dx%d %s: top at row %d with no live forwards, %d with three", size.Width, size.Height, title, emptyTops[title], fullTops[title])
			}
		}
		if emptyTops["[4]─Live"] <= emptyTops["[3]─Listening"] {
			t.Errorf("%dx%d: Live is not below Listening: %v", size.Width, size.Height, emptyTops)
		}
	}
}
