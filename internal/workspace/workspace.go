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
	if err := validateHostName(name); err != nil {
		return err
	}
	if !slices.Contains(w.ManualHosts, name) {
		w.ManualHosts = append(w.ManualHosts, name)
	}
	if !slices.Contains(w.PinnedHosts, name) {
		w.PinnedHosts = append(w.PinnedHosts, name)
	}
	return nil
}

func validateHostName(name string) error {
	if name == "" || strings.ContainsAny(name, " \t") {
		return fmt.Errorf("host must be an ssh destination without spaces, got %q", name)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("host must not start with -, which ssh reads as an option, got %q", name)
	}
	return nil
}

// validate reports the first manual host or forward that the forms could not
// have made. Values from workspace.json reach ssh and, for a remote socket
// path, a remote shell, so a hand-edited or crafted file must meet the same
// rules as typed input.
func (w Workspace) validate() error {
	for _, name := range w.ManualHosts {
		if err := validateHostName(name); err != nil {
			return fmt.Errorf("manual host: %w", err)
		}
	}
	for _, forward := range w.Forwards {
		if err := forward.validate(); err != nil {
			return fmt.Errorf("forward %d on host %q: %w", forward.ID, forward.Host, err)
		}
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

// AddForwards adds one forward per target through host. A zero bind picks
// a TCP port automatically: for Local, the target port if free on the local
// machine, else the nearest free port; for Remote, the target port. A socket
// target has no port to pick from, so it needs a bind. isFree reports whether
// a local port is free right now.
func (w *Workspace) AddForwards(host string, direction Direction, targets []End, bind End, isFree func(int) bool) ([]Forward, error) {
	if bind != (End{}) && len(targets) > 1 {
		return nil, fmt.Errorf("set the listening end only for a single target, got %d targets", len(targets))
	}
	binds, err := w.binds(host, direction, targets, bind, isFree, 0)
	if err != nil {
		return nil, err
	}
	added := make([]Forward, len(targets))
	for i, target := range targets {
		w.NextID++
		added[i] = Forward{ID: w.NextID, Host: host, Direction: direction, Bind: binds[i], Target: target}
	}
	w.Forwards = append(w.Forwards, added...)
	return added, nil
}

// UpdateForward changes a forward's target and bind, with the same defaults
// as AddForwards.
func (w *Workspace) UpdateForward(id ForwardID, target End, bind End, isFree func(int) bool) (Forward, error) {
	i := slices.IndexFunc(w.Forwards, func(f Forward) bool { return f.ID == id })
	if i < 0 {
		return Forward{}, ErrForwardNotFound
	}
	forward := w.Forwards[i]
	binds, err := w.binds(forward.Host, forward.Direction, []End{target}, bind, isFree, id)
	if err != nil {
		return Forward{}, err
	}
	forward.Target, forward.Bind = target, binds[0]
	w.Forwards[i] = forward
	return forward, nil
}

// repairIDs gives every forward its own positive ID and moves NextID above
// all of them. A hand-edited or merged file can lack next_id or repeat an ID,
// which would let a new forward share an ID and one delete remove both. IDs
// are internal and nothing else refers to them, so renumbering a repeat keeps
// every forward and loses nothing.
func (w *Workspace) repairIDs() {
	w.NextID = max(w.NextID, 0)
	for _, forward := range w.Forwards {
		w.NextID = max(w.NextID, forward.ID)
	}
	isTaken := map[ForwardID]bool{}
	for i, forward := range w.Forwards {
		if forward.ID <= 0 || isTaken[forward.ID] {
			w.NextID++
			w.Forwards[i].ID = w.NextID
		}
		isTaken[w.Forwards[i].ID] = true
	}
}

// DeleteForward removes a forward; deleting an unknown forward is a no-op.
func (w *Workspace) DeleteForward(id ForwardID) {
	w.Forwards = slices.DeleteFunc(w.Forwards, func(f Forward) bool { return f.ID == id })
}

// binds resolves the bind of each target and refuses one that another
// forward listens on.
func (w Workspace) binds(host string, direction Direction, targets []End, bind End, isFree func(int) bool, editedID ForwardID) ([]End, error) {
	binds, err := w.pickBinds(direction, targets, bind, isFree, editedID)
	if err != nil {
		return nil, err
	}
	for i, bind := range binds {
		if slices.Contains(binds[:i], bind) || w.isListeningOn(host, direction, bind, editedID) {
			return nil, fmt.Errorf("another forward already listens on the %s end %s", listeningSide(direction), bind)
		}
	}
	return binds, nil
}

// isListeningOn reports whether a forward other than editedID listens on
// bind: on the local machine for any host, or on host itself. The master
// keeps one listener for two identical forwards, so stopping either would
// close the other's.
func (w Workspace) isListeningOn(host string, direction Direction, bind End, editedID ForwardID) bool {
	return slices.ContainsFunc(w.Forwards, func(f Forward) bool {
		return f.ID != editedID && f.Direction == direction && f.Bind == bind && (direction == Local || f.Host == host)
	})
}

// pickBinds returns the given bind, or picks one per target. Local ports held
// by other forwards, running or not, count as taken, so starting them later
// cannot collide.
func (w Workspace) pickBinds(direction Direction, targets []End, bind End, isFree func(int) bool, editedID ForwardID) ([]End, error) {
	if bind != (End{}) {
		return []End{bind}, nil
	}
	wanted := make([]int, len(targets))
	for i, target := range targets {
		if target.IsSocket() {
			return nil, fmt.Errorf("target %s is a socket, so there is no port to copy: set the %s end", target, listeningSide(direction))
		}
		wanted[i] = target.Port
	}
	ports := wanted
	if direction == Local {
		isHeld := map[int]bool{}
		for _, forward := range w.Forwards {
			if forward.Direction == Local && !forward.Bind.IsSocket() && forward.ID != editedID {
				isHeld[forward.Bind.Port] = true
			}
		}
		var err error
		ports, err = AssignPorts(wanted, func(port int) bool { return !isHeld[port] && isFree(port) })
		if err != nil {
			return nil, err
		}
	}
	binds := make([]End, len(ports))
	for i, port := range ports {
		binds[i] = End{Host: BindHost, Port: port}
	}
	return binds, nil
}

// listeningSide names the end that listens, as the forms label it.
func listeningSide(direction Direction) string {
	if direction == Remote {
		return "remote"
	}
	return "local"
}
