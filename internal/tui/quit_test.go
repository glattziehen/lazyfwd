package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// sendForCmd sends msg and returns the model and the message its command
// produces, or nil.
func sendForCmd(m Model, msg tea.Msg) (Model, tea.Msg) {
	next, cmd := m.Update(msg)
	if cmd == nil {
		return next.(Model), nil
	}
	return next.(Model), cmd()
}

func isQuit(msg tea.Msg) bool {
	_, ok := msg.(tea.QuitMsg)
	return ok
}

// runningModel has forwards 1 and 2 running on a host whose ssh never
// connects, so they stay running without a network.
func runningModel(t *testing.T) Model {
	t.Helper()
	hangingSSH(t)
	m := newLiveModel(t)
	m = send(m, keyMsg("2"))
	m = send(m, keyMsg(" "))
	m = send(m, keyMsg("j"))
	m = send(m, keyMsg(" "))
	return m
}

func TestQuitAsksFirstAndEnterQuits(t *testing.T) {
	m := runningModel(t)

	m, msg := sendForCmd(m, keyMsg("q"))
	if isQuit(msg) || m.quitDialog == nil {
		t.Fatal("q quit without asking")
	}
	_, msg = sendForCmd(m, tea.KeyMsg{Type: tea.KeyEnter})

	if !isQuit(msg) {
		t.Fatal("enter on the default choice did not quit")
	}
}

func TestQuitDialogNamesRunningForwards(t *testing.T) {
	m := send(runningModel(t), tea.WindowSizeMsg{Width: 120, Height: 35})

	m = send(m, keyMsg("q"))

	if view := m.View(); !strings.Contains(view, "2 running forwards will stop") {
		t.Fatalf("dialog does not name the running forwards:\n%s", view)
	}
}

func TestQuitDialogCancelKeepsForwardsRunning(t *testing.T) {
	cancels := map[string]tea.KeyMsg{
		"n":           keyMsg("n"),
		"esc":         {Type: tea.KeyEsc},
		"enter on No": {Type: tea.KeyEnter},
	}
	for name, cancel := range cancels {
		m := send(runningModel(t), keyMsg("q"))
		if name == "enter on No" {
			m = send(m, keyMsg("l"))
		}

		m, msg := sendForCmd(m, cancel)

		if isQuit(msg) || m.quitDialog != nil {
			t.Errorf("%s: quit %v, dialog open %v", name, isQuit(msg), m.quitDialog != nil)
		}
		if !m.supervisor.IsRunning(1) || !m.supervisor.IsRunning(2) {
			t.Errorf("%s: a forward stopped", name)
		}
	}
}

// ctrl+c asks like q, and a second ctrl+c quits, so it never traps anyone.
func TestSecondCtrlCQuits(t *testing.T) {
	m := newLiveModel(t)
	ctrlC := tea.KeyMsg{Type: tea.KeyCtrlC}

	m, msg := sendForCmd(m, ctrlC)
	if isQuit(msg) {
		t.Fatal("first ctrl+c quit without asking")
	}
	_, msg = sendForCmd(m, ctrlC)

	if !isQuit(msg) {
		t.Fatal("second ctrl+c did not quit")
	}
}

// A paste must not answer the dialog, even one that holds y.
func TestQuitDialogIgnoresBursts(t *testing.T) {
	for _, burst := range []tea.KeyMsg{keyMsg("yy"), {Type: tea.KeyRunes, Runes: []rune("y"), Paste: true}} {
		m := send(newLiveModel(t), keyMsg("q"))

		m, msg := sendForCmd(m, burst)

		if isQuit(msg) || m.quitDialog == nil {
			t.Errorf("%q (paste %v) answered the dialog", string(burst.Runes), burst.Paste)
		}
	}
}

// A form takes q as text, as before.
func TestFormTakesQAsText(t *testing.T) {
	m := send(newLiveModel(t), keyMsg("2"))
	m = send(m, keyMsg("a"))

	m = send(m, keyMsg("q"))

	if m.quitDialog != nil || m.form == nil || m.form.value(0) != "q" {
		t.Fatal("q in a form did not go to the form")
	}
}
