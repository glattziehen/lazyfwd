//go:build unix

package e2e

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// shortTempRoot keeps socket paths under Unix's 104-byte limit.
func shortTempRoot() string { return "/tmp" }

func ownProcessGroup() *syscall.SysProcAttr { return nil }

// interrupt sends SIGINT, as ctrl+c in a terminal does.
func interrupt(process *os.Process) error {
	return process.Signal(os.Interrupt)
}

// leftoverSSH lists the ssh processes whose command line names runBase: the
// masters of one lazyfwd world.
func leftoverSSH(runBase string) []string {
	output, _ := exec.Command("pgrep", "-fl", runBase).Output()
	return strings.Fields(strings.TrimSpace(string(output)))
}
