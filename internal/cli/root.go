package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/glattziehen/lazyfwd/internal/control"
	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/tui"
	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

const rootHelp = `lazyfwd manages SSH port forwards. Run it without a command for the terminal UI,
or use the commands below from a shell, a script, or an agent.

How it works:
  One lazyfwd process, the owner, holds the ssh connections: the terminal UI
  ("lazyfwd") or the headless "lazyfwd serve". Forwards run only while the owner
  runs, and stop when it quits. Commands go through the running owner, so the
  terminal UI shows their changes at once and they see the UI's.

  With no owner running, commands that only list or change saved hosts and
  forwards still work, and every forward is stopped. Commands that need a
  connection (forward start, stop, toggle, add --start; port list; host login)
  exit with code 3.

  lazyfwd uses your system ssh, so your ssh config, agent, and ProxyJump apply.

Forwards:
  Each forward has a numeric ID, shown by "lazyfwd forward list", that stays the
  same until the forward is deleted. Commands take forwards by ID.

  A local forward (ssh -L) listens on this machine, at its local end, and
  connects from the host to its remote end. A remote forward (ssh -R) listens on
  the host, at its remote end, and connects from this machine to its local end.
  An end is a port (4966, which means localhost:4966), host:port, or an absolute
  unix socket path (/run/app.sock).

Output:
  Commands print aligned text, or JSON with --json; "lazyfwd forward list --help"
  and the other list commands describe the fields. Errors go to stderr.

Exit codes:
  0  success
  1  the operation failed; stderr says why, in ssh's words where ssh failed
  2  usage error: unknown command or flag, or a bad argument
  3  no owner is running: start "lazyfwd" or "lazyfwd serve" first
  4  a host needs an interactive login (password, passphrase, host key, or 2FA):
     ask a person to run "lazyfwd host login HOST" in a terminal, then retry
  5  a forward was still starting when the command stopped waiting; it keeps
     starting, and "lazyfwd forward list" shows how it ends

Hosts, pins, and forwards are saved in workspace.json in your config directory
($XDG_CONFIG_HOME/lazyfwd on Linux, ~/Library/Application Support/lazyfwd on macOS).`

func newRootCommand(version string, e env) *cobra.Command {
	root := &cobra.Command{
		Use:     "lazyfwd",
		Short:   "Manage SSH port forwards from a terminal UI or the command line",
		Long:    rootHelp,
		Version: version,
		Example: `  # Open the terminal UI, which owns the connections while it runs
  lazyfwd

  # Or own them headless, then forward remote ports 4966-4968 to the same local ports
  lazyfwd serve &
  lazyfwd forward add box 4966-4968 --start

  # See every forward and its status as JSON
  lazyfwd forward list --json`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: action(func(*cobra.Command, []string) error {
			return runTUI(e)
		}),
	}
	root.SetVersionTemplate("lazyfwd {{.Version}}\n")
	root.SetOut(e.stdout)
	root.SetErr(e.stderr)
	root.SetIn(e.stdin)
	root.PersistentFlags().Bool("json", false, "print JSON instead of text")
	root.Flags().BoolP("version", "v", false, "print the version and exit")
	root.AddCommand(newServeCommand(e), newHostCommand(e), newForwardCommand(e), newPortCommand(e))
	root.CompletionOptions.HiddenDefaultCmd = true
	return root
}

func isJSON(cmd *cobra.Command) bool {
	isJSON, _ := cmd.Flags().GetBool("json")
	return isJSON
}

// ownerProcess is this process as the workspace's owner, serving commands.
type ownerProcess struct {
	owner  *owner.Owner
	server *control.Server
	lock   *workspace.Lock
}

// becomeOwner claims the workspace, opens the ssh connections' run
// directory, and answers commands on a control socket in it.
func becomeOwner(e env) (*ownerProcess, error) {
	store := workspace.FileStore{Path: e.storePath}
	lock, err := control.Claim(store)
	if err != nil {
		return nil, err
	}
	ws, err := store.Load()
	if err != nil {
		lock.Release()
		return nil, err
	}
	sshConfigHosts, err := e.sshConfigHosts()
	if err != nil {
		lock.Release()
		return nil, err
	}
	supervisor, err := tunnel.NewSupervisor(e.runBase)
	if err != nil {
		lock.Release()
		return nil, err
	}
	o := owner.New(store, supervisor, ws, sshConfigHosts)
	server, err := control.Serve(o, lock, supervisor.RunDir())
	if err != nil {
		o.Close()
		lock.Release()
		return nil, err
	}
	return &ownerProcess{owner: o, server: server, lock: lock}, nil
}

// close stops every forward, then the control socket, then releases the
// workspace.
func (p *ownerProcess) close() {
	p.owner.Close()
	p.server.Close()
	p.lock.Release()
}

func runTUI(e env) error {
	process, err := becomeOwner(e)
	if err != nil {
		return err
	}
	defer process.close()
	_, err = tea.NewProgram(tui.New(process.owner), tea.WithAltScreen()).Run()
	return err
}

func newServeCommand(e env) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Own the ssh connections without the terminal UI",
		Long: `Own the workspace and its ssh connections without the terminal UI, so that
commands can start and stop forwards.

serve runs in the foreground until SIGINT (ctrl+c) or SIGTERM, and logs one line
per event to stderr: each change, start, failure, and stop. When it ends, it
stops every forward, like quitting the terminal UI. Forwards start stopped:
start them with "lazyfwd forward start".

serve fails while another owner (serve or the terminal UI) runs on the same
workspace. A host that needs a password or another login step cannot log in
here: commands that start its forwards exit with code 4, and a person runs
"lazyfwd host login HOST" in a terminal.`,
		Example: `  # Own the connections in this terminal; ctrl+c stops every forward
  lazyfwd serve

  # Own them in the background and keep a log
  lazyfwd serve 2>>/tmp/lazyfwd.log &
  lazyfwd forward start 3`,
		Args: cobra.NoArgs,
		RunE: action(func(*cobra.Command, []string) error {
			return serve(e)
		}),
	}
}

func serve(e env) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	process, err := becomeOwner(e)
	if err != nil {
		return err
	}
	defer process.close()
	events, cancel := process.owner.Subscribe()
	defer cancel()
	logLine(e, fmt.Sprintf("serving %s on %s", e.storePath, process.server.Address()))
	for {
		select {
		case event := <-events:
			logLine(e, event.Text)
		case <-ctx.Done():
			logLine(e, "stopping every forward")
			return nil
		}
	}
}

func logLine(e env, text string) {
	_, _ = fmt.Fprintln(e.stderr, time.Now().Format(time.RFC3339), text)
}
