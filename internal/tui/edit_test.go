package tui

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

// hangingSSH puts an ssh on PATH that never connects, so a started forward
// stays running for the whole test without a network.
func hangingSSH(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRefusedEditLeavesForwardRunningAndUnchanged(t *testing.T) {
	hangingSSH(t)
	var ws workspace.Workspace
	added, err := ws.AddForwards("box", workspace.Local, []workspace.End{{Host: "localhost", Port: 8000}}, workspace.End{Host: workspace.BindHost, Port: 9000}, func(int) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	forward := added[0]
	m := New(discardStore{}, newTestSupervisor(t), ws, nil)
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
	if !m.supervisor.IsRunning(forward.ID) {
		t.Error("refused edit stopped the forward")
	}
	if got, _ := m.forward(forward.ID); got != forward {
		t.Errorf("forward changed to %+v", got)
	}
}

// Auto-picking during an edit may keep the running forward's own port,
// which the forward itself holds until the restart.
func TestEditKeepsRunningForwardsOwnPort(t *testing.T) {
	hangingSSH(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0") // stands in for the forward's own listener
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	var ws workspace.Workspace
	added, err := ws.AddForwards("box", workspace.Local, []workspace.End{{Host: "localhost", Port: port}}, workspace.End{Host: workspace.BindHost, Port: port}, func(int) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	m := New(discardStore{}, newTestSupervisor(t), ws, nil)
	m = send(m, keyMsg("2"))
	m = send(m, keyMsg(" "))
	m = send(m, keyMsg("e"))
	m.form.inputs[1].SetValue("")

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})

	if got, _ := m.forward(added[0].ID); got.Bind.Port != port {
		t.Fatalf("local end moved from %d to %d", port, got.Bind.Port)
	}
}
