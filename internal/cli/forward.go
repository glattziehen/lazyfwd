package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/glattziehen/lazyfwd/internal/browser"
	"github.com/glattziehen/lazyfwd/internal/control"
	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// startWait bounds how long a command waits for forwards to start. Opening
// a connection gives up after 20 seconds and checking and requesting the
// forward after about 25 more, so most starts end well within it.
const startWait = 60 * time.Second

// listenAuto is the --listen value that picks the listening end again.
const listenAuto = "auto"

const forwardJSONHelp = `With --json, it prints an array of forwards in creation order:
  [{"id": 3, "host": "box", "direction": "local",
    "local_end": "localhost:4966", "remote_end": "localhost:4966",
    "status": "running", "reason": "", "needs_login": false,
    "url": "http://localhost:4966"}]
  id           the forward's ID, which commands take
  host         the ssh destination the forward goes through
  direction    "local" (ssh -L, listens at local_end) or
               "remote" (ssh -R, listens at remote_end)
  local_end    the end on this machine: host:port or a unix socket path
  remote_end   the end on the host: host:port or a unix socket path
  status       "stopped", "starting", "running", or "failed"
  reason       why a failed forward failed, in ssh's words where ssh failed;
               "" otherwise
  needs_login  true when the failure needs "lazyfwd host login HOST"
  url          the local end's web address, or "" for a socket or a known
               non-web port such as 5432`

const endsHelp = `An end is a port (4966 means localhost:4966), host:port as the connecting side
resolves it (db:5432), or an absolute unix socket path (/run/app.sock).`

func newForwardCommand(e env) *cobra.Command {
	forward := &cobra.Command{
		Use:   "forward",
		Short: "List, add, edit, delete, start, stop, and open forwards",
		Long: `List, add, edit, delete, start, stop, and open forwards.

A local forward (ssh -L) listens on this machine at its local end and connects
from the host to its remote end. A remote forward (ssh -R) listens on the host
at its remote end and connects from this machine to its local end. Commands take
forwards by the ID that "lazyfwd forward list" shows.`,
		Args: cobra.NoArgs,
	}
	forward.AddCommand(
		newForwardListCommand(e),
		newForwardAddCommand(e),
		newForwardEditCommand(e),
		&cobra.Command{
			Use:   "delete ID...",
			Short: "Stop and delete forwards",
			Long:  "Stop and delete forwards. It checks every ID before it deletes any, and prints nothing.",
			Example: `  lazyfwd forward delete 3
  lazyfwd forward delete 3 4 5`,
			Args: cobra.MinimumNArgs(1),
			RunE: action(func(_ *cobra.Command, args []string) error {
				ids, err := parseIDs(args)
				if err != nil {
					return err
				}
				_, err = e.call(control.Request{Op: control.OpDeleteForwards, IDs: ids})
				return err
			}),
		},
		newForwardRunCommand(e, "start", control.OpStartForwards,
			"Start forwards and wait until they run",
			"Start stopped or failed forwards; a forward that runs already stays as it is.",
			`  lazyfwd forward start 3
  lazyfwd forward start 3 4 5 --json
  lazyfwd forward start 3 --no-wait`),
		newForwardRunCommand(e, "stop", control.OpStopForwards,
			"Stop forwards",
			"Stop forwards and clear their failures; a stopped forward stays as it is.",
			`  lazyfwd forward stop 3
  lazyfwd forward stop $(lazyfwd forward list box --json | jq -r '.[].id')`),
		newForwardRunCommand(e, "toggle", control.OpToggleForwards,
			"Stop running forwards and start the others",
			"Stop each starting or running forward, and start each stopped or failed one, as space does in the terminal UI.",
			`  lazyfwd forward toggle 3
  lazyfwd forward toggle 3 4 --no-wait`),
		newForwardOpenCommand(e),
	)
	return forward
}

func newForwardListCommand(e env) *cobra.Command {
	return &cobra.Command{
		Use:   "list [HOST]",
		Short: "List forwards with their status",
		Long: `List the forwards of every host, or of HOST, with their status and the reason
for a failure. With no owner running, every forward is stopped.

` + forwardJSONHelp,
		Example: `  lazyfwd forward list
  lazyfwd forward list box
  lazyfwd forward list --json | jq '.[] | select(.status == "failed")'`,
		Args: cobra.MaximumNArgs(1),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			response, err := e.call(control.Request{Op: control.OpState})
			if err != nil {
				return err
			}
			forwards := response.State.Forwards
			if len(args) == 1 {
				if !hasHost(*response.State, args[0]) {
					return &owner.HostNotFoundError{Host: args[0]}
				}
				forwards = forwardsOf(*response.State, args[0])
			}
			return printForwards(cmd, forwards)
		}),
	}
}

func newForwardAddCommand(e env) *cobra.Command {
	var direction, listen string
	var isStarting, isNotWaiting bool
	add := &cobra.Command{
		Use:   "add HOST TARGET...",
		Short: "Add local or remote forwards through a host",
		Long: `Add one forward per target through HOST, an ssh destination. The forwards are
stopped unless --start is given.

TARGET is where the forward connects: the remote end of a local forward, the
local end of a remote forward. A port range (4966-4968) or a comma-separated list
adds one forward per port; several TARGET arguments work the same.

` + endsHelp + `

Without --listen, a TCP forward picks its listening end: a local forward listens
on the target's port when it is free on this machine, else on the nearest free
port; a remote forward listens on the target's port. A socket target needs
--listen, and --listen takes a single target.

It prints the added forwards as "lazyfwd forward list" does. With --start it
needs a running owner and exits like "lazyfwd forward start".`,
		Example: `  # Forward remote ports 4966-4968 to the same local ports, and start them
  lazyfwd forward add box 4966-4968 --start

  # Reach PostgreSQL on db, as box resolves it, at local port 5433
  lazyfwd forward add box db:5432 --listen 5433

  # Reach the Docker socket on box through a local unix socket
  lazyfwd forward add box /var/run/docker.sock --listen /tmp/box-docker.sock

  # Let box reach your local port 3000 at its own port 3000 (ssh -R)
  lazyfwd forward add box 3000 --direction remote`,
		Args: cobra.MinimumNArgs(2),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			request := control.Request{Op: control.OpAddForwards, Host: args[0], Direction: workspace.Direction(direction)}
			if request.Direction != workspace.Local && request.Direction != workspace.Remote {
				return &usageError{fmt.Errorf("--direction must be %s or %s, got %q", workspace.Local, workspace.Remote, direction)}
			}
			var err error
			if request.Targets, err = workspace.ParseTargets(strings.Join(args[1:], ",")); err != nil {
				return &usageError{err}
			}
			if request.Bind, err = workspace.ParseBind(listen); err != nil {
				return &usageError{fmt.Errorf("--listen: %w", err)}
			}
			client, err := e.connect()
			if err != nil {
				return err
			}
			defer client.Close()
			if isStarting && !client.HasOwner() {
				return explain(&control.Error{Kind: control.ErrorNoOwner}, "")
			}
			response, err := client.Call(request)
			if err != nil {
				return explain(err, request.Host)
			}
			if !isStarting {
				return printForwards(cmd, response.Forwards)
			}
			return startForwards(cmd, client, control.OpStartForwards, forwardIDs(response.Forwards), !isNotWaiting)
		}),
	}
	add.Flags().StringVar(&direction, "direction", string(workspace.Local), `"local" (ssh -L: listen on this machine) or "remote" (ssh -R: listen on HOST)`)
	add.Flags().StringVar(&listen, "listen", "", "the listening end: a port or a socket path (default: picked from the target's port)")
	add.Flags().BoolVar(&isStarting, "start", false, "start the added forwards and wait until they run")
	add.Flags().BoolVar(&isNotWaiting, "no-wait", false, "with --start, return while the forwards are still starting")
	return add
}

func newForwardEditCommand(e env) *cobra.Command {
	var target, listen string
	edit := &cobra.Command{
		Use:   "edit ID",
		Short: "Change a forward's target or listening end",
		Long: `Change where a forward connects (--target) or listens (--listen); an omitted
flag keeps its value. The direction and host stay; to change them, delete the
forward and add a new one. --listen auto picks the listening end as
"lazyfwd forward add" does without --listen.
` + endsHelp + `

A starting or running forward restarts with the change, and the command waits
until it runs, as "lazyfwd forward start" does. A refused change leaves the
forward as it was. It prints the forward as "lazyfwd forward list" does.`,
		Example: `  # Listen on local port 8080 instead
  lazyfwd forward edit 3 --listen 8080

  # Connect to port 4967 and pick the local port again
  lazyfwd forward edit 3 --target 4967 --listen auto

  # Connect to a unix socket on the host instead
  lazyfwd forward edit 3 --target /run/app.sock --listen /tmp/app.sock`,
		Args: cobra.ExactArgs(1),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			isTargetSet, isListenSet := cmd.Flags().Changed("target"), cmd.Flags().Changed("listen")
			if !isTargetSet && !isListenSet {
				return &usageError{errors.New("set --target, --listen, or both")}
			}
			client, err := e.connect()
			if err != nil {
				return err
			}
			defer client.Close()
			response, err := client.Call(control.Request{Op: control.OpState})
			if err != nil {
				return explain(err, "")
			}
			current, ok := response.State.Forward(ids[0])
			if !ok {
				return &owner.ForwardNotFoundError{ID: ids[0]}
			}
			request := control.Request{Op: control.OpEditForward, IDs: ids, Target: current.Forward.Target, Bind: current.Forward.Bind}
			if isTargetSet {
				targets, err := workspace.ParseTargets(target)
				if err != nil {
					return &usageError{fmt.Errorf("--target: %w", err)}
				}
				if len(targets) != 1 {
					return &usageError{fmt.Errorf("--target: a forward has one target, got %d", len(targets))}
				}
				request.Target = targets[0]
			}
			if isListenSet {
				request.Bind = workspace.End{}
				if listen != listenAuto {
					if request.Bind, err = workspace.ParseBind(listen); err != nil || request.Bind == (workspace.End{}) {
						return &usageError{fmt.Errorf("--listen must be a port, a socket path, or %s, got %q", listenAuto, listen)}
					}
				}
			}
			response, err = client.Call(request)
			if err != nil {
				return explain(err, current.Forward.Host)
			}
			if response.Forwards[0].Status.State == tunnel.Starting && client.HasOwner() {
				return startForwards(cmd, client, control.OpStartForwards, ids, true)
			}
			return printForwards(cmd, response.Forwards)
		}),
	}
	edit.Flags().StringVar(&target, "target", "", "where the forward connects: the remote end of a local forward, the local end of a remote one")
	edit.Flags().StringVar(&listen, "listen", "", `where the forward listens: a port, a socket path, or "auto"`)
	return edit
}

// newForwardRunCommand makes start, stop, or toggle.
func newForwardRunCommand(e env, name string, op control.Op, short, long, example string) *cobra.Command {
	var isNotWaiting bool
	command := &cobra.Command{
		Use:     name + " ID...",
		Short:   short,
		Long:    long + " It checks every ID before it acts on any, and needs a running owner.",
		Example: example,
		Args:    cobra.MinimumNArgs(1),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			client, err := e.connect()
			if err != nil {
				return err
			}
			defer client.Close()
			return startForwards(cmd, client, op, ids, !isNotWaiting)
		}),
	}
	if op == control.OpStopForwards {
		command.Long += ` It prints the forwards as "lazyfwd forward list" does.`
		return command
	}
	command.Long += `

A forward that starts takes until ssh has connected and the master holds the
forward: the command waits up to ` + startWait.String() + ` for that, unless --no-wait is given. It
prints the forwards as "lazyfwd forward list" does, then exits with 1 if a
forward failed, 4 if a host needs an interactive login, and 5 if a forward was
still starting when the wait ended.`
	command.Flags().BoolVar(&isNotWaiting, "no-wait", false, "return while forwards are still starting")
	return command
}

// startForwards runs a start, stop, or toggle, prints the forwards, and
// returns an error for each forward that failed or is still starting.
func startForwards(cmd *cobra.Command, client *control.Client, op control.Op, ids []workspace.ForwardID, isWaiting bool) error {
	request := control.Request{Op: op, IDs: ids}
	if isWaiting {
		request.WaitMillis = startWait.Milliseconds()
	}
	response, err := client.Call(request)
	if err != nil {
		return explain(err, "")
	}
	if err := printForwards(cmd, response.Forwards); err != nil {
		return err
	}
	return startOutcome(response.Forwards, isWaiting)
}

// startOutcome is the error for forwards that failed or, after a wait, are
// still starting; a login comes first, since it is what unblocks a person.
func startOutcome(forwards []owner.ForwardState, isWaiting bool) error {
	var needsLogin, failed, starting []string
	for _, state := range forwards {
		forward, status := state.Forward, state.Status
		switch {
		case status.State == tunnel.Failed && status.NeedsLogin:
			needsLogin = append(needsLogin, fmt.Sprintf("forward %d: %s needs an interactive login: %s; ask a person to run `lazyfwd host login %s` in a terminal, which starts it again", forward.ID, forward.Host, status.Reason, forward.Host))
		case status.State == tunnel.Failed:
			failed = append(failed, fmt.Sprintf("forward %d failed: %s", forward.ID, status.Reason))
		case status.State == tunnel.Starting && isWaiting:
			starting = append(starting, fmt.Sprintf("forward %d is still starting after %s; it keeps starting, and `lazyfwd forward list` shows how it ends", forward.ID, startWait))
		}
	}
	switch {
	case len(needsLogin) > 0:
		return &exitError{code: exitNeedsLogin, err: errors.New(strings.Join(needsLogin, "\n"))}
	case len(failed) > 0:
		return &exitError{code: exitFailed, err: errors.New(strings.Join(failed, "\n"))}
	case len(starting) > 0:
		return &exitError{code: exitStillStarting, err: errors.New(strings.Join(starting, "\n"))}
	}
	return nil
}

func newForwardOpenCommand(e env) *cobra.Command {
	return &cobra.Command{
		Use:   "open ID",
		Short: "Open a forward's local end in the browser",
		Long: `Open a forward's local end in this machine's browser and print its URL: https
for a target on port 443 or 8443, http otherwise. It refuses a unix socket and a
known non-web port such as 5432. It does not start the forward.

Without a graphical session, such as over ssh, it fails and says to open the URL
yourself; the url field of "lazyfwd forward list --json" has it too.`,
		Example: `  lazyfwd forward open 3
  lazyfwd forward list --json | jq -r '.[] | select(.id == 3) | .url'`,
		Args: cobra.ExactArgs(1),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			response, err := e.call(control.Request{Op: control.OpState})
			if err != nil {
				return err
			}
			state, ok := response.State.Forward(ids[0])
			if !ok {
				return &owner.ForwardNotFoundError{ID: ids[0]}
			}
			forward := state.Forward
			url, isWeb := forward.LocalURL()
			switch {
			case forward.LocalEnd().IsSocket():
				return fmt.Errorf("%s is a unix socket, which a browser cannot open; connect with its client", forward.LocalEnd())
			case !isWeb:
				return fmt.Errorf("%s is not a web port; connect with its client", forward.LocalEnd())
			}
			if err := browser.Open(url); err != nil {
				return fmt.Errorf("open browser: %w", err)
			}
			if isJSON(cmd) {
				return writeJSON(cmd.OutOrStdout(), forwardsJSON([]owner.ForwardState{state}))
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), url)
			return err
		}),
	}
}

func printForwards(cmd *cobra.Command, forwards []owner.ForwardState) error {
	out := forwardsJSON(forwards)
	if isJSON(cmd) {
		return writeJSON(cmd.OutOrStdout(), out)
	}
	return writeForwards(cmd.OutOrStdout(), out)
}

func parseIDs(args []string) ([]workspace.ForwardID, error) {
	ids := make([]workspace.ForwardID, len(args))
	for i, arg := range args {
		id, err := strconv.Atoi(arg)
		if err != nil || id < 1 {
			return nil, &usageError{fmt.Errorf("a forward ID is a positive number from `lazyfwd forward list`, got %q", arg)}
		}
		ids[i] = workspace.ForwardID(id)
	}
	return ids, nil
}

func forwardIDs(forwards []owner.ForwardState) []workspace.ForwardID {
	ids := make([]workspace.ForwardID, len(forwards))
	for i, state := range forwards {
		ids[i] = state.Forward.ID
	}
	return ids
}

func forwardsOf(state owner.State, host string) []owner.ForwardState {
	var forwards []owner.ForwardState
	for _, forward := range state.Forwards {
		if forward.Forward.Host == host {
			forwards = append(forwards, forward)
		}
	}
	return forwards
}

func hasHost(state owner.State, name string) bool {
	for _, host := range state.Hosts {
		if host.Name == name {
			return true
		}
	}
	return false
}
