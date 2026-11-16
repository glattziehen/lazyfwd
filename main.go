// lazyfwd is a keyboard-first TUI for ssh port forwards.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moritzwilksch/lazyfwd/internal/sshconfig"
	"github.com/moritzwilksch/lazyfwd/internal/tui"
	"github.com/moritzwilksch/lazyfwd/internal/tunnel"
	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("lazyfwd", version())
		return
	}
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
	supervisor, err := tunnel.NewSupervisor(tunnel.DefaultBaseDir())
	if err != nil {
		return err
	}
	defer supervisor.StopAll()
	_, err = tea.NewProgram(tui.New(store, supervisor, ws, sshConfigHosts), tea.WithAltScreen()).Run()
	return err
}

// version is the main module version the go command stamps at build time:
// the tag for a release or `go install ...@vX`, a pseudo-version from git for
// a local build, and "(devel)" for a build outside a git checkout.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "(devel)"
	}
	return info.Main.Version
}
