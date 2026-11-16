// lazyfwd is a keyboard-first TUI for ssh port forwards.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moritzwilksch/lazyfwd/internal/sshconfig"
	"github.com/moritzwilksch/lazyfwd/internal/tui"
	"github.com/moritzwilksch/lazyfwd/internal/tunnel"
	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lazyfwd:", err)
		os.Exit(1)
	}
}

func run() error {
	storePath, err := workspace.DefaultStorePath()
	if err != nil {
		return err
	}
	store := workspace.FileStore{Path: storePath}
	release, err := store.Lock()
	if err != nil {
		return err
	}
	defer release()
	ws, err := store.Load()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sshConfigHosts, err := sshconfig.Hosts(filepath.Join(home, ".ssh", "config"), sshconfig.SystemConfigPath)
	if err != nil {
		return err
	}
	supervisor := tunnel.NewSupervisor()
	defer supervisor.StopAll()
	_, err = tea.NewProgram(tui.New(store, supervisor, ws, sshConfigHosts), tea.WithAltScreen()).Run()
	return err
}
