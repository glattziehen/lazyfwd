package cli

import (
	"github.com/spf13/cobra"

	"github.com/glattziehen/lazyfwd/internal/control"
)

func newPortCommand(e env) *cobra.Command {
	port := &cobra.Command{
		Use:   "port",
		Short: "List the TCP ports listening on a host",
		Args:  cobra.NoArgs,
	}
	var isShowingAll bool
	list := &cobra.Command{
		Use:   "list HOST",
		Short: "List the TCP ports listening on a host, with process names",
		Long: `List the TCP ports listening on HOST, with the name of the process that
listens where the host tells it. lazyfwd asks the host over the running owner's
connection (ss, netstat, sockstat, lsof, or /proc, whichever it has), so every
run is fresh. It needs a running owner, and takes up to 20 seconds.

By default it lists your own processes' ports; --all adds other users' ports,
such as system services and Docker's published ports. The FORWARD column is
the ID of the forward that already reaches the port from this machine (a local
forward to localhost:PORT); "lazyfwd forward add HOST PORT --start" adds one.

With --json, it prints:
  {"host": "box", "ports": [{"port": 4966, "process": "diffle", "owned": true,
   "forward_id": 3}], "hidden_ports": 2, "own_only": false}
  ports         sorted by port
  process       the listening process, or "" when the host does not name it
                (other users' processes, or a host without process names)
  owned         true when your ssh user's process listens
  forward_id    the forward to localhost:PORT, or null
  hidden_ports  other users' ports that --all adds
  own_only      true when the host shows only your own ports, so --all adds none`,
		Example: `  lazyfwd port list box
  lazyfwd port list box --all
  lazyfwd port list box --json | jq '.ports[] | select(.forward_id == null) | .port'`,
		Args: cobra.ExactArgs(1),
		RunE: action(func(cmd *cobra.Command, args []string) error {
			response, err := e.call(control.Request{Op: control.OpListeners, Host: args[0]})
			if err != nil {
				return err
			}
			out := listingJSON(args[0], *response.Listing, *response.State, isShowingAll)
			if isJSON(cmd) {
				return writeJSON(cmd.OutOrStdout(), out)
			}
			return writePorts(cmd.OutOrStdout(), out)
		}),
	}
	list.Flags().BoolVar(&isShowingAll, "all", false, "also list other users' ports")
	port.AddCommand(list)
	return port
}
