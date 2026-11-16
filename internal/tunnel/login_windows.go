package tunnel

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
)

// errLoginCancelled reports that the user ended ssh at its prompt.
var errLoginCancelled = errors.New("cancelled")

// statusControlCExit is the exit status of a Windows console process that
// ctrl+c ended.
const statusControlCExit = 0xC000013A

// Login is an interactive ssh on the terminal that lets the user accept a
// host's key, then exits. On Windows each forward connects on its own, so a
// login cannot answer a password or 2FA prompt for them, and it asks none.
// It satisfies bubbletea's ExecCommand.
type Login struct {
	Host   string
	cmd    *exec.Cmd
	stderr io.Writer
}

// NewLogin connects to host on the terminal. Windows keeps no connection
// open, so controlPath goes unused.
func NewLogin(host, _ string) *Login {
	return &Login{Host: host, cmd: exec.Command("ssh", hostKeyLoginArgs(host)...)}
}

// Run returns once ssh has authenticated and exited.
func (l *Login) Run() error {
	var stderr bytes.Buffer
	l.cmd.Stderr = &stderr
	if l.stderr != nil {
		l.cmd.Stderr = io.MultiWriter(l.stderr, &stderr)
	}
	err := l.cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && uint32(exitErr.ExitCode()) == statusControlCExit {
		return errLoginCancelled
	}
	if err != nil {
		reason, _ := explainFailure(stderr.String(), err)
		// ssh's messages went to the terminal already.
		return &ConnectError{Host: l.Host, Reason: reason}
	}
	return nil
}

func (l *Login) SetStdin(r io.Reader)  { l.cmd.Stdin = r }
func (l *Login) SetStdout(w io.Writer) { l.cmd.Stdout = w }
func (l *Login) SetStderr(w io.Writer) { l.stderr = w }
