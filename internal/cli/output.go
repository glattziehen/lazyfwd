package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/glattziehen/lazyfwd/internal/listeners"
	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// The JSON shapes below are a public contract that scripts parse: add
// fields freely, but never rename or remove one. helpJSON documents them.

// hostJSON is one host in --json output.
type hostJSON struct {
	Name        string `json:"name"`
	Pinned      bool   `json:"pinned"`
	InSSHConfig bool   `json:"in_ssh_config"`
	Added       bool   `json:"added"`
	Forwards    int    `json:"forwards"`
}

// forwardJSON is one forward in --json output.
type forwardJSON struct {
	ID         workspace.ForwardID `json:"id"`
	Host       string              `json:"host"`
	Direction  workspace.Direction `json:"direction"`
	LocalEnd   string              `json:"local_end"`
	RemoteEnd  string              `json:"remote_end"`
	Status     string              `json:"status"`
	Reason     string              `json:"reason"`
	NeedsLogin bool                `json:"needs_login"`
	URL        string              `json:"url"`
}

// portsJSON is a host's listening ports in --json output.
type portsJSON struct {
	Host  string     `json:"host"`
	Ports []portJSON `json:"ports"`
	// HiddenPorts counts other users' ports that --all would add.
	HiddenPorts int `json:"hidden_ports"`
	// OwnOnly is set when the host shows only your own ports, so --all
	// cannot add any.
	OwnOnly bool `json:"own_only"`
}

type portJSON struct {
	Port    int    `json:"port"`
	Process string `json:"process"`
	// Owned is set when the ssh user's own process listens.
	Owned bool `json:"owned"`
	// ForwardID is the host's local forward to localhost:Port, or null.
	ForwardID *workspace.ForwardID `json:"forward_id"`
}

func hostsJSON(state owner.State, hosts []workspace.Host) []hostJSON {
	out := []hostJSON{}
	for _, host := range hosts {
		out = append(out, hostJSON{
			Name:        host.Name,
			Pinned:      host.IsPinned,
			InSSHConfig: host.IsInSSHConfig,
			Added:       host.IsManual,
			Forwards:    len(state.ForwardsOf(host.Name)),
		})
	}
	return out
}

func forwardsJSON(forwards []owner.ForwardState) []forwardJSON {
	out := []forwardJSON{}
	for _, state := range forwards {
		forward := state.Forward
		url, _ := forward.LocalURL()
		out = append(out, forwardJSON{
			ID:         forward.ID,
			Host:       forward.Host,
			Direction:  forward.Direction,
			LocalEnd:   forward.LocalEnd().String(),
			RemoteEnd:  forward.RemoteEnd().String(),
			Status:     string(state.Status.State),
			Reason:     state.Status.Reason,
			NeedsLogin: state.Status.NeedsLogin,
			URL:        url,
		})
	}
	return out
}

func listingJSON(host string, listing listeners.Listing, state owner.State, isShowingAll bool) portsJSON {
	shown := listing.Listeners
	if !isShowingAll {
		shown = listeners.Owned(shown)
	}
	out := portsJSON{Host: host, Ports: []portJSON{}, HiddenPorts: len(listing.Listeners) - len(shown), OwnOnly: listing.IsOwnOnly}
	for _, listener := range shown {
		port := portJSON{Port: listener.Port, Process: listener.Process, Owned: listener.IsOwned()}
		if forward, ok := state.ListenerForward(host, listener.Port); ok {
			port.ForwardID = &forward.ID
		}
		out.Ports = append(out.Ports, port)
	}
	return out
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// writeTable writes rows under a header, in columns two spaces apart.
func writeTable(w io.Writer, header []string, rows [][]string) error {
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, row := range append([][]string{header}, rows...) {
		_, _ = fmt.Fprintln(table, strings.TrimRight(strings.Join(row, "\t"), "\t"))
	}
	return table.Flush()
}

func writeHosts(w io.Writer, hosts []hostJSON) error {
	var rows [][]string
	for _, host := range hosts {
		var sources []string
		if host.InSSHConfig {
			sources = append(sources, "ssh config")
		}
		if host.Added {
			sources = append(sources, "added")
		}
		if len(sources) == 0 {
			sources = append(sources, "forwards")
		}
		rows = append(rows, []string{host.Name, yesNo(host.Pinned), strings.Join(sources, ", "), strconv.Itoa(host.Forwards)})
	}
	return writeTable(w, []string{"HOST", "PINNED", "SOURCE", "FORWARDS"}, rows)
}

func writeForwards(w io.Writer, forwards []forwardJSON) error {
	var rows [][]string
	for _, forward := range forwards {
		rows = append(rows, []string{
			strconv.Itoa(int(forward.ID)), forward.Host, string(forward.Direction),
			forward.LocalEnd, forward.RemoteEnd, forward.Status, forward.Reason,
		})
	}
	return writeTable(w, []string{"ID", "HOST", "DIRECTION", "LOCAL END", "REMOTE END", "STATUS", "REASON"}, rows)
}

func writePorts(w io.Writer, ports portsJSON) error {
	var rows [][]string
	for _, port := range ports.Ports {
		process, forward := port.Process, ""
		if process == "" {
			process = "-"
		}
		if port.ForwardID != nil {
			forward = strconv.Itoa(int(*port.ForwardID))
		}
		rows = append(rows, []string{strconv.Itoa(port.Port), process, forward})
	}
	if err := writeTable(w, []string{"PORT", "PROCESS", "FORWARD"}, rows); err != nil {
		return err
	}
	switch {
	case ports.OwnOnly:
		_, _ = fmt.Fprintf(w, "%s shows you only your own ports; other users' need root there\n", ports.Host)
	case ports.HiddenPorts > 0:
		_, _ = fmt.Fprintf(w, "%d more of other users, such as Docker: --all shows them\n", ports.HiddenPorts)
	}
	return nil
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
