// Package workspace holds the user's hosts and forwards and the rules for changing them.
package workspace

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Workspace is everything lazyfwd persists: manually added hosts, pins, and forwards.
type Workspace struct {
	ManualHosts []string  `json:"manual_hosts"`
	PinnedHosts []string  `json:"pinned_hosts"`
	Forwards    []Forward `json:"forwards"`
	NextID      ForwardID `json:"next_id"`
}

// Host is one entry of the host list.
type Host struct {
	Name          string
	IsPinned      bool
	IsManual      bool
	IsInSSHConfig bool
}

// ErrForwardNotFound reports an unknown forward ID.
var ErrForwardNotFound = errors.New("forward not found")

// Hosts merges ssh config hosts with manual hosts: pinned first, then by name.
func (w Workspace) Hosts(sshConfigHosts []string) []Host {
	byName := map[string]*Host{}
	add := func(name string) *Host {
		if host, ok := byName[name]; ok {
			return host
		}
		host := &Host{Name: name, IsPinned: slices.Contains(w.PinnedHosts, name)}
		byName[name] = host
		return host
	}
	for _, name := range sshConfigHosts {
		add(name).IsInSSHConfig = true
	}
	for _, name := range w.ManualHosts {
		add(name).IsManual = true
	}
	for _, forward := range w.Forwards {
		add(forward.Host)
	}
	hosts := make([]Host, 0, len(byName))
	for _, host := range byName {
		hosts = append(hosts, *host)
	}
	slices.SortFunc(hosts, func(a, b Host) int {
		if a.IsPinned != b.IsPinned {
			if a.IsPinned {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return hosts
}

// AddHost adds a manual host and pins it, since a host added by hand is one the user wants at hand.
func (w *Workspace) AddHost(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t") {
		return fmt.Errorf("host must be an ssh destination without spaces, got %q", name)
	}
	if !slices.Contains(w.ManualHosts, name) {
		w.ManualHosts = append(w.ManualHosts, name)
	}
	if !slices.Contains(w.PinnedHosts, name) {
		w.PinnedHosts = append(w.PinnedHosts, name)
	}
	return nil
}

// TogglePin pins or unpins a host.
func (w *Workspace) TogglePin(name string) {
	if i := slices.Index(w.PinnedHosts, name); i >= 0 {
		w.PinnedHosts = slices.Delete(w.PinnedHosts, i, i+1)
		return
	}
	w.PinnedHosts = append(w.PinnedHosts, name)
}

// RemoveHost forgets a host's manual entry and pin. The host stays listed
// while it has forwards or appears in the ssh config, which lazyfwd never edits.
func (w *Workspace) RemoveHost(name string) {
	w.ManualHosts = slices.DeleteFunc(w.ManualHosts, func(h string) bool { return h == name })
	w.PinnedHosts = slices.DeleteFunc(w.PinnedHosts, func(h string) bool { return h == name })
}

// ForwardsOf lists the forwards through one host in creation order.
func (w Workspace) ForwardsOf(host string) []Forward {
	var forwards []Forward
	for _, forward := range w.Forwards {
		if forward.Host == host {
			forwards = append(forwards, forward)
		}
	}
	return forwards
}

// AddForwards adds one forward per target through host. A bindPort of 0 picks
// one automatically: for Local, the target port if free on the laptop, else
// the nearest free port; for Remote, the target port. isFree reports whether
// a laptop port is free right now.
func (w *Workspace) AddForwards(host string, direction Direction, targets []Addr, bindPort int, isFree func(int) bool) ([]Forward, error) {
	if bindPort != 0 && len(targets) > 1 {
		return nil, fmt.Errorf("set a bind port only for a single target, got %d targets", len(targets))
	}
	bindPorts, err := w.bindPorts(direction, targets, bindPort, isFree, 0)
	if err != nil {
		return nil, err
	}
	added := make([]Forward, len(targets))
	for i, target := range targets {
		w.NextID++
		added[i] = Forward{ID: w.NextID, Host: host, Direction: direction, BindPort: bindPorts[i], Target: target}
	}
	w.Forwards = append(w.Forwards, added...)
	return added, nil
}

// UpdateForward changes a forward's target and bind port, with the same
// defaults as AddForwards.
func (w *Workspace) UpdateForward(id ForwardID, target Addr, bindPort int, isFree func(int) bool) (Forward, error) {
	i := slices.IndexFunc(w.Forwards, func(f Forward) bool { return f.ID == id })
	if i < 0 {
		return Forward{}, ErrForwardNotFound
	}
	forward := w.Forwards[i]
	bindPorts, err := w.bindPorts(forward.Direction, []Addr{target}, bindPort, isFree, id)
	if err != nil {
		return Forward{}, err
	}
	forward.Target, forward.BindPort = target, bindPorts[0]
	w.Forwards[i] = forward
	return forward, nil
}

// DeleteForward removes a forward; deleting an unknown forward is a no-op.
func (w *Workspace) DeleteForward(id ForwardID) {
	w.Forwards = slices.DeleteFunc(w.Forwards, func(f Forward) bool { return f.ID == id })
}

// bindPorts resolves the bind port of each target. Laptop ports held by other
// forwards, running or not, count as taken, so starting them later cannot collide.
func (w Workspace) bindPorts(direction Direction, targets []Addr, bindPort int, isFree func(int) bool, editedID ForwardID) ([]int, error) {
	if bindPort != 0 {
		return []int{bindPort}, nil
	}
	wanted := make([]int, len(targets))
	for i, target := range targets {
		wanted[i] = target.Port
	}
	if direction == Remote {
		return wanted, nil
	}
	isHeld := map[int]bool{}
	for _, forward := range w.Forwards {
		if forward.Direction == Local && forward.ID != editedID {
			isHeld[forward.BindPort] = true
		}
	}
	return AssignPorts(wanted, func(port int) bool { return !isHeld[port] && isFree(port) })
}
