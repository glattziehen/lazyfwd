// Package cli is lazyfwd's command line: the terminal UI without a command,
// `serve` for a headless owner, and commands that list and change hosts,
// forwards, and listening ports through the running owner.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/glattziehen/lazyfwd/internal/control"
	"github.com/glattziehen/lazyfwd/internal/sshconfig"
	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// Exit codes, which helpRoot documents.
const (
	exitOK            = 0
	exitFailed        = 1
	exitUsage         = 2
	exitNoOwner       = 3
	exitNeedsLogin    = 4
	exitStillStarting = 5
)

// exitError is an error from a command's own work, as opposed to cobra's
// usage errors, with the exit code it ends the process with.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// usageError is a bad argument that cobra could not catch.
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }

// env is what commands take from the process, so tests can replace it.
type env struct {
	stdin          io.Reader
	stdout, stderr io.Writer
	storePath      string
	sshConfigPaths []string
	// runBase is the parent of the owner's run directory.
	runBase string
}

// Main runs lazyfwd with args and returns the process's exit code.
func Main(args []string, version string) int {
	storePath, err := workspace.DefaultStorePath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazyfwd:", err)
		return exitFailed
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazyfwd:", err)
		return exitFailed
	}
	return run(args, version, env{
		stdin:          os.Stdin,
		stdout:         os.Stdout,
		stderr:         os.Stderr,
		storePath:      storePath,
		sshConfigPaths: []string{filepath.Join(home, ".ssh", "config"), sshconfig.SystemConfigPath},
		runBase:        tunnel.DefaultBaseDir(),
	})
}

func run(args []string, version string, e env) int {
	root := newRootCommand(version, e)
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return exitOK
	}
	_, _ = fmt.Fprintln(e.stderr, "lazyfwd:", err)
	var exitErr *exitError
	if errors.As(err, &exitErr) {
		return exitErr.code
	}
	// Everything else is cobra's: an unknown command or flag, or a wrong
	// number of arguments.
	_, _ = fmt.Fprintln(e.stderr, "Run 'lazyfwd --help' or 'lazyfwd COMMAND --help' for usage.")
	return exitUsage
}

// action adapts a command's work to cobra, marking its errors with their
// exit codes so that run can tell them from cobra's usage errors.
func action(work func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := work(cmd, args); err != nil {
			return &exitError{code: exitCode(err), err: err}
		}
		return nil
	}
}

func exitCode(err error) int {
	var exitErr *exitError
	var usageErr *usageError
	var controlErr *control.Error
	switch {
	case errors.As(err, &exitErr):
		return exitErr.code
	case errors.As(err, &usageErr):
		return exitUsage
	case errors.As(err, &controlErr):
		switch controlErr.Kind {
		case control.ErrorNoOwner:
			return exitNoOwner
		case control.ErrorNeedsLogin:
			return exitNeedsLogin
		}
	}
	return exitFailed
}

// connect reaches the workspace's owner, or the saved workspace when none runs.
func (e env) connect() (*control.Client, error) {
	return control.Connect(workspace.FileStore{Path: e.storePath}, e.sshConfigHosts)
}

func (e env) sshConfigHosts() ([]string, error) {
	return sshconfig.Hosts(e.sshConfigPaths...)
}

// call runs one operation on the workspace's owner.
func (e env) call(request control.Request) (control.Response, error) {
	client, err := e.connect()
	if err != nil {
		return control.Response{}, err
	}
	defer client.Close()
	response, err := client.Call(request)
	return response, explain(err, request.Host)
}

// explain turns a control error into the message an agent can act on.
func explain(err error, host string) error {
	var controlErr *control.Error
	if !errors.As(err, &controlErr) {
		return err
	}
	switch controlErr.Kind {
	case control.ErrorNoOwner:
		return &exitError{code: exitNoOwner, err: errors.New("no lazyfwd is running to hold the ssh connections; start `lazyfwd` (terminal UI) or `lazyfwd serve` (headless) first, then retry")}
	case control.ErrorNeedsLogin:
		return &exitError{code: exitNeedsLogin, err: fmt.Errorf("%w; ask a person to run `lazyfwd host login %s` in a terminal, then retry", controlErr, host)}
	}
	return err
}
