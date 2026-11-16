package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/glattziehen/lazyfwd/internal/control"
	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

const hostJSONHelp = `With --json, it prints an array of hosts, pinned first, then by name:
  [{"name": "box", "pinned": true, "in_ssh_config": true, "added": false, "forwards": 3}]
  name           the ssh destination, as ssh takes it
  pinned         listed first
  in_ssh_config  named in a Host line of ~/.ssh/config or /etc/ssh/ssh_config
  added          added with "lazyfwd host add" or in the terminal UI
  forwards       how many forwards go through the host; a host that is neither
                 in the ssh config nor added is listed because of its forwards`

func newHostCommand(e env) *cobra.Command {
	host := &cobra.Command{
		Use:   "host",
		Short: "List, add, remove, pin, and log in to hosts",
		Long: `List, add, remove, pin, and log in to hosts.

The host list holds the hosts of your ssh config, hosts added by hand, and hosts
that forwards go through. Pinned hosts come first.`,
		Args: cobra.NoArgs,
	}
	host.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List hosts with their pin and source",
			Long:  "List hosts with their pin and source, pinned first, then by name.\n\n" + hostJSONHelp,
			Example: `  lazyfwd host list
  lazyfwd host list --json | jq -r '.[] | select(.pinned) | .name'`,
			Args: cobra.NoArgs,
			RunE: action(func(cmd *cobra.Command, _ []string) error {
				response, err := e.call(control.Request{Op: control.OpState})
				if err != nil {
					return err
				}
				return printHosts(cmd, *response.State, response.State.Hosts)
			}),
		},
		&cobra.Command{
			Use:   "add HOST",
			Short: "Add a host by hand and pin it",
			Long: `Add a host by hand and pin it. HOST is an ssh destination: a Host alias from
your ssh config, box, user@box, or ssh://user@box:2222. Adding a host that is
already listed pins it.

It prints the host as "lazyfwd host list" does.`,
			Example: `  lazyfwd host add box
  lazyfwd host add deploy@10.0.0.7
  lazyfwd host add ssh://box:2222`,
			Args: cobra.ExactArgs(1),
			RunE: action(func(cmd *cobra.Command, args []string) error {
				return changeHost(cmd, e, control.OpAddHost, args[0])
			}),
		},
		&cobra.Command{
			Use:   "remove HOST",
			Short: "Remove a host added by hand",
			Long: `Remove a host added by hand, with its pin. It refuses a host that still has
forwards (delete them first) and a host from your ssh config, which lazyfwd never
edits. It prints nothing.`,
			Example: `  lazyfwd host remove box
  lazyfwd forward delete $(lazyfwd forward list box --json | jq -r '.[].id') && lazyfwd host remove box`,
			Args: cobra.ExactArgs(1),
			RunE: action(func(_ *cobra.Command, args []string) error {
				_, err := e.call(control.Request{Op: control.OpRemoveHost, Host: args[0]})
				return err
			}),
		},
		&cobra.Command{
			Use:   "pin HOST",
			Short: "Pin a host, so it is listed first",
			Long: `Pin a listed host, so it is listed first. It prints the host as
"lazyfwd host list" does.`,
			Example: `  lazyfwd host pin box
  lazyfwd host pin box --json`,
			Args: cobra.ExactArgs(1),
			RunE: action(func(cmd *cobra.Command, args []string) error {
				return changeHost(cmd, e, control.OpPinHost, args[0])
			}),
		},
		&cobra.Command{
			Use:   "unpin HOST",
			Short: "Unpin a host, so it is listed by name",
			Long: `Unpin a listed host, so it is listed by name. It prints the host as
"lazyfwd host list" does.`,
			Example: `  lazyfwd host unpin box
  lazyfwd host unpin box --json`,
			Args: cobra.ExactArgs(1),
			RunE: action(func(cmd *cobra.Command, args []string) error {
				return changeHost(cmd, e, control.OpUnpinHost, args[0])
			}),
		},
		newLoginCommand(e),
	)
	return host
}

func newLoginCommand(e env) *cobra.Command {
	return &cobra.Command{
		Use:   "login HOST",
		Short: "Log in to a host in this terminal for the running owner",
		Long: `Log in to a host in this terminal, for the running owner (terminal UI or serve).

The owner connects without prompts, so a host that needs a password, a key
passphrase, a host key decision, or 2FA fails with exit code 4. A person runs this
command in a terminal and answers ssh's prompts here. Once ssh connects, the
owner's forwards and port lists on HOST use this connection, and HOST's failed
forwards start again. The connection ends when the owner quits.

On Windows each forward connects on its own, so a login there only lets you
accept HOST's key; it asks for no password or 2FA, which the forwards could not
reuse.

It needs a terminal and a running owner (exit code 3 without one).`,
		Example: `  # After "lazyfwd forward start 3" exited with code 4:
  lazyfwd host login box
  lazyfwd forward list box`,
		Args: cobra.ExactArgs(1),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			return login(cmd, e, args[0])
		}),
	}
}

func login(cmd *cobra.Command, e env, host string) error {
	stdin, ok := e.stdin.(*os.File)
	if !ok || !isatty.IsTerminal(stdin.Fd()) {
		return &usageError{errors.New("host login asks for a password or other answers, so it needs a terminal: ask a person to run it")}
	}
	client, err := e.connect()
	if err != nil {
		return err
	}
	defer client.Close()
	response, err := client.Call(control.Request{Op: control.OpControlPath, Host: host})
	if err != nil {
		return explain(err, host)
	}
	_, _ = fmt.Fprintf(e.stderr, "lazyfwd: logging in to %s for the running lazyfwd; ctrl+c cancels.\n\n", host)
	sshLogin := tunnel.NewLogin(host, response.ControlPath)
	sshLogin.SetStdin(e.stdin)
	sshLogin.SetStdout(e.stdout)
	sshLogin.SetStderr(e.stderr)
	if err := sshLogin.Run(); err != nil {
		var connectErr *tunnel.ConnectError
		if errors.As(err, &connectErr) {
			_, _ = fmt.Fprintln(e.stderr, connectErr.Log)
		}
		return fmt.Errorf("login to %s failed: %w", host, err)
	}
	if _, err := client.Call(control.Request{Op: control.OpLoggedIn, Host: host}); err != nil {
		return explain(err, host)
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "lazyfwd: logged in to %s; its failed forwards start again.\n", host)
	return nil
}

func changeHost(cmd *cobra.Command, e env, op control.Op, name string) error {
	response, err := e.call(control.Request{Op: op, Host: name})
	if err != nil {
		return err
	}
	state := *response.State
	name = strings.TrimSpace(name) // as AddHost does
	i := slices.IndexFunc(state.Hosts, func(h workspace.Host) bool { return h.Name == name })
	if i < 0 {
		return nil
	}
	return printHosts(cmd, state, state.Hosts[i:i+1])
}

func printHosts(cmd *cobra.Command, state owner.State, hosts []workspace.Host) error {
	out := hostsJSON(state, hosts)
	if isJSON(cmd) {
		return writeJSON(cmd.OutOrStdout(), out)
	}
	return writeHosts(cmd.OutOrStdout(), out)
}
