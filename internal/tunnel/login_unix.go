//go:build unix

package tunnel

import (
	"errors"
	"io"
	"os"
	"os/exec"
)

// errLoginCancelled reports that ssh ended on a signal, such as ctrl+c at
// its prompt.
var errLoginCancelled = errors.New("cancelled")

// Login is an interactive ssh that opens a host's master on the terminal, so
// the user can accept a host key or answer a password, passphrase, or 2FA
// prompt. Forwards and fetches on that host then reuse the master. It
// satisfies bubbletea's ExecCommand.
type Login struct {
	Host       string
	socketPath string
	logPath    string
	cmd        *exec.Cmd
}

// NewLogin opens host's master on controlPath, a Supervisor's ControlPath:
// the Supervisor may run in another lazyfwd process of the same user.
func NewLogin(host, controlPath string) *Login {
	logPath := controlPath + logExt
	return &Login{
		Host:       host,
		socketPath: controlPath,
		logPath:    logPath,
		cmd:        exec.Command("ssh", loginArgs(controlPath, logPath, host)...),
	}
}

// Run opens the master unless one already serves the socket. ssh moves to
// the background once authenticated, so Run returns then.
func (l *Login) Run() error {
	if _, ok := checkMaster(l.socketPath, l.Host); ok {
		return nil
	}
	// A stale socket would make the new master run without one.
	_ = os.Remove(l.socketPath)
	_ = os.Remove(l.logPath)
	err := l.cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && !exitErr.Exited() {
		return errLoginCancelled
	}
	if err != nil {
		return connectError(l.Host, l.logPath, err)
	}
	return nil
}

func (l *Login) SetStdin(r io.Reader)  { l.cmd.Stdin = r }
func (l *Login) SetStdout(w io.Writer) { l.cmd.Stdout = w }
func (l *Login) SetStderr(w io.Writer) { l.cmd.Stderr = w }
