package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/glattziehen/lazyfwd/internal/workspace"
)

func TestRefusedEditKeepsTheFormOpenAndTheForwardRunning(t *testing.T) {
	var ws workspace.Workspace
	added, err := ws.AddForwards("box", workspace.Local, []workspace.End{{Host: "localhost", Port: 8000}}, workspace.End{Host: workspace.BindHost, Port: 9000}, func(int) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	forward := added[0]
	m, _ := newTestModel(t, ws, nil)
	m = send(m, keyMsg("2"))
	m = send(m, keyMsg(" "))
	m = send(m, keyMsg("e"))
	// A socket target with an empty local end has no port to copy.
	m.form.inputs[0].SetValue("/run/app.sock")
	m.form.inputs[1].SetValue("")

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.message == "" || m.form == nil {
		t.Fatalf("edit was not refused: message %q", m.message)
	}
	if got, _ := m.state.Forward(forward.ID); got.Forward != forward || !got.Status.IsActive() {
		t.Errorf("forward is %+v, want unchanged and running", got)
	}
}
