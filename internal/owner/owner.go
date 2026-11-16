// Package owner holds the workspace and runs its forwards for one lazyfwd
// process. Every change goes through an Owner, whether it comes from the TUI
// or from a CLI command over the control socket, so both apply the same
// rules and each sees the other's changes at once.
package owner

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/glattziehen/lazyfwd/internal/listeners"
	"github.com/glattziehen/lazyfwd/internal/tunnel"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

// subscriberBuffer bounds the events a subscriber may fall behind by; it
// misses later ones until it catches up.
const subscriberBuffer = 64

// ErrNoConnections reports an operation that needs live ssh connections on
// an Owner that holds none: one made by a CLI command while no lazyfwd runs.
var ErrNoConnections = errors.New("no lazyfwd holds the ssh connections")

// HostNotFoundError reports a host that is not in the host list.
type HostNotFoundError struct {
	Host string
}

func (e *HostNotFoundError) Error() string {
	return fmt.Sprintf("host %q is not in the host list", e.Host)
}

// ForwardNotFoundError reports an unknown forward ID.
type ForwardNotFoundError struct {
	ID workspace.ForwardID
}

func (e *ForwardNotFoundError) Error() string {
	return fmt.Sprintf("no forward has ID %d", e.ID)
}

// Store persists the workspace after every change.
type Store interface {
	Save(workspace.Workspace) error
}

// Tunnels runs forwards over ssh; *tunnel.Supervisor implements it.
type Tunnels interface {
	Start(workspace.Forward)
	Stop(workspace.ForwardID)
	Status(workspace.ForwardID) tunnel.Status
	Events() <-chan tunnel.Event
	Run(ctx context.Context, host, command string) (string, error)
	ControlPath(host string) string
	StopAll()
}

// Event says that something changed.
type Event struct {
	// Text says what happened, in one line for a log.
	Text string
	// LoginHost is set when a forward on that host failed because ssh needs
	// an interactive login there.
	LoginHost string
}

// Owner is the workspace of one lazyfwd process and the forwards it runs.
// Its methods are safe for concurrent use; each runs as one step.
type Owner struct {
	store          Store
	tunnels        Tunnels
	sshConfigHosts []string
	closeOnce      sync.Once
	done           chan struct{}

	mu          sync.Mutex
	workspace   workspace.Workspace
	subscribers map[chan Event]struct{}
}

// New owns ws, saving it to store after every change. A nil tunnels makes
// an Owner that runs nothing, for changing saved state while no lazyfwd
// runs: its forwards are all stopped, and operations that need connections
// fail with ErrNoConnections.
func New(store Store, tunnels Tunnels, ws workspace.Workspace, sshConfigHosts []string) *Owner {
	o := &Owner{
		store:          store,
		tunnels:        tunnels,
		sshConfigHosts: sshConfigHosts,
		done:           make(chan struct{}),
		workspace:      ws,
		subscribers:    map[chan Event]struct{}{},
	}
	if tunnels != nil {
		go o.watch(tunnels.Events())
	}
	return o
}

// Close stops every forward; the Owner starts nothing afterwards.
func (o *Owner) Close() {
	o.closeOnce.Do(func() {
		close(o.done)
		if o.tunnels != nil {
			o.tunnels.StopAll()
		}
	})
}

// Subscribe delivers an Event for every change until cancel is called.
func (o *Owner) Subscribe() (events <-chan Event, cancel func()) {
	channel := make(chan Event, subscriberBuffer)
	o.mu.Lock()
	o.subscribers[channel] = struct{}{}
	o.mu.Unlock()
	return channel, func() {
		o.mu.Lock()
		delete(o.subscribers, channel)
		o.mu.Unlock()
	}
}

// State is a copy of the workspace with each forward's status.
func (o *Owner) State() State {
	o.mu.Lock()
	defer o.mu.Unlock()
	state := State{Hosts: o.workspace.Hosts(o.sshConfigHosts)}
	for _, forward := range o.workspace.Forwards {
		state.Forwards = append(state.Forwards, ForwardState{Forward: forward, Status: o.status(forward.ID)})
	}
	return state
}

// AddHost adds a host by hand and pins it.
func (o *Owner) AddHost(name string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.workspace.AddHost(name); err != nil {
		return err
	}
	return o.saveAndPublish("host %s added", name)
}

// RemoveHost forgets a host added by hand. It refuses a host that still has
// forwards, so no single command deletes more than one forward, and a host
// from the ssh config, which lazyfwd never edits.
func (o *Owner) RemoveHost(name string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	host, err := o.host(name)
	if err != nil {
		return err
	}
	if count := len(o.workspace.ForwardsOf(name)); count > 0 {
		return fmt.Errorf("delete the forwards of %s first (%d left)", name, count)
	}
	if !host.IsManual {
		return fmt.Errorf("%s comes from your ssh config (~/.ssh/config or /etc/ssh/ssh_config); remove it there", name)
	}
	o.workspace.RemoveHost(name)
	return o.saveAndPublish("host %s removed", name)
}

// PinHost lists a host first.
func (o *Owner) PinHost(name string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, err := o.host(name); err != nil {
		return err
	}
	o.workspace.Pin(name)
	return o.saveAndPublish("host %s pinned", name)
}

// UnpinHost lists a host by name again.
func (o *Owner) UnpinHost(name string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, err := o.host(name); err != nil {
		return err
	}
	o.workspace.Unpin(name)
	return o.saveAndPublish("host %s unpinned", name)
}

// AddForwards adds one stopped forward per target through host, with the
// defaults of workspace.AddForwards.
func (o *Owner) AddForwards(host string, direction workspace.Direction, targets []workspace.End, bind workspace.End) ([]workspace.Forward, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	added, err := o.workspace.AddForwards(host, direction, targets, bind, tunnel.IsLocalPortFree)
	if err != nil {
		return nil, err
	}
	for _, forward := range added {
		o.publish(Event{Text: describe(forward) + " added"})
	}
	return added, o.save()
}

// EditForward changes a forward's target and bind, with the defaults of
// AddForwards, and restarts it if it was started. A refused edit leaves the
// forward running as it was.
func (o *Owner) EditForward(id workspace.ForwardID, target, bind workspace.End) (workspace.Forward, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	old, err := o.forward(id)
	if err != nil {
		return workspace.Forward{}, err
	}
	// The started forward's own local port counts as free, since the restart
	// releases it.
	wasActive := o.status(id).IsActive()
	isFree := tunnel.IsLocalPortFree
	if wasActive && old.Direction == workspace.Local && !old.Bind.IsSocket() {
		isFree = func(port int) bool { return port == old.Bind.Port || tunnel.IsLocalPortFree(port) }
	}
	forward, err := o.workspace.UpdateForward(id, target, bind, isFree)
	if err != nil {
		return workspace.Forward{}, err
	}
	o.stop(id)
	if wasActive {
		o.tunnels.Start(forward)
	}
	o.publish(Event{Text: describe(forward) + " edited"})
	return forward, o.save()
}

// DeleteForward stops and deletes a forward.
func (o *Owner) DeleteForward(id workspace.ForwardID) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, err := o.forward(id); err != nil {
		return err
	}
	o.stop(id)
	o.workspace.DeleteForward(id)
	return o.saveAndPublish("forward %d deleted", id)
}

// StartForward starts a stopped or failed forward and returns at once;
// starting a started forward is a no-op. AwaitStart waits for the outcome.
func (o *Owner) StartForward(id workspace.ForwardID) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	forward, err := o.forward(id)
	if err != nil {
		return err
	}
	if o.tunnels == nil {
		return ErrNoConnections
	}
	o.start(forward)
	return nil
}

// StopForward stops a forward and forgets its failure.
func (o *Owner) StopForward(id workspace.ForwardID) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, err := o.forward(id); err != nil {
		return err
	}
	if o.tunnels == nil {
		return ErrNoConnections
	}
	o.stop(id)
	o.publish(Event{Text: fmt.Sprintf("forward %d stopped", id)})
	return nil
}

// ToggleForward stops a started forward and starts a stopped or failed one.
func (o *Owner) ToggleForward(id workspace.ForwardID) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	forward, err := o.forward(id)
	if err != nil {
		return err
	}
	if o.tunnels == nil {
		return ErrNoConnections
	}
	if o.status(id).IsActive() {
		o.stop(id)
		o.publish(Event{Text: fmt.Sprintf("forward %d stopped", id)})
		return nil
	}
	o.start(forward)
	return nil
}

// AwaitStart waits while the forward is starting, until ctx ends, and
// returns its state then.
func (o *Owner) AwaitStart(ctx context.Context, id workspace.ForwardID) (ForwardState, error) {
	events, cancel := o.Subscribe()
	defer cancel()
	for {
		state, ok := o.State().Forward(id)
		if !ok {
			return ForwardState{}, &ForwardNotFoundError{ID: id}
		}
		if state.Status.State != tunnel.Starting {
			return state, nil
		}
		select {
		case <-events:
		case <-ctx.Done():
			return state, nil
		}
	}
}

// ControlPath is where a login to host opens the connection that this
// Owner's forwards and fetches on host then share.
func (o *Owner) ControlPath(host string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, err := o.host(host); err != nil {
		return "", err
	}
	if o.tunnels == nil {
		return "", ErrNoConnections
	}
	return o.tunnels.ControlPath(host), nil
}

// LoggedIn restarts host's failed forwards after an interactive login to
// host opened its connection.
func (o *Owner) LoggedIn(host string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.tunnels == nil {
		return ErrNoConnections
	}
	o.publish(Event{Text: "logged in to " + host})
	for _, forward := range o.workspace.ForwardsOf(host) {
		if o.status(forward.ID).State == tunnel.Failed {
			o.start(forward)
		}
	}
	return nil
}

// Listeners asks host which TCP ports listen there. It takes up to the
// fetch's own timeout; the error is a *tunnel.ConnectError when the
// connection did not open.
func (o *Owner) Listeners(host string) (listeners.Listing, error) {
	if o.tunnels == nil {
		return listeners.Listing{}, ErrNoConnections
	}
	return listeners.Fetch(o.tunnels.Run, host)
}

// watch turns the tunnels' events into Events until Close.
func (o *Owner) watch(events <-chan tunnel.Event) {
	for {
		select {
		case event := <-events:
			o.noticeTunnel(event)
		case <-o.done:
			return
		}
	}
}

func (o *Owner) noticeTunnel(event tunnel.Event) {
	o.mu.Lock()
	defer o.mu.Unlock()
	text := fmt.Sprintf("forward %d %s", event.ID, event.Status.State)
	if event.Status.Reason != "" {
		text += ": " + event.Status.Reason
	}
	ownerEvent := Event{Text: text}
	if forward, err := o.forward(event.ID); err == nil && event.Status.NeedsLogin {
		ownerEvent.LoginHost = forward.Host
	}
	o.publish(ownerEvent)
}

// The methods below need o.mu held.

func (o *Owner) host(name string) (workspace.Host, error) {
	hosts := o.workspace.Hosts(o.sshConfigHosts)
	if i := slices.IndexFunc(hosts, func(h workspace.Host) bool { return h.Name == name }); i >= 0 {
		return hosts[i], nil
	}
	return workspace.Host{}, &HostNotFoundError{Host: name}
}

func (o *Owner) forward(id workspace.ForwardID) (workspace.Forward, error) {
	if i := slices.IndexFunc(o.workspace.Forwards, func(f workspace.Forward) bool { return f.ID == id }); i >= 0 {
		return o.workspace.Forwards[i], nil
	}
	return workspace.Forward{}, &ForwardNotFoundError{ID: id}
}

func (o *Owner) status(id workspace.ForwardID) tunnel.Status {
	if o.tunnels == nil {
		return tunnel.Status{State: tunnel.Stopped}
	}
	return o.tunnels.Status(id)
}

func (o *Owner) start(forward workspace.Forward) {
	o.tunnels.Start(forward)
	o.publish(Event{Text: describe(forward) + " starting"})
}

// stop also forgets a failure.
func (o *Owner) stop(id workspace.ForwardID) {
	if o.tunnels != nil {
		o.tunnels.Stop(id)
	}
}

// save keeps a change in memory even when saving fails, as the TUI shows it.
func (o *Owner) save() error {
	if err := o.store.Save(o.workspace); err != nil {
		return fmt.Errorf("save workspace: %w", err)
	}
	return nil
}

func (o *Owner) saveAndPublish(format string, args ...any) error {
	o.publish(Event{Text: fmt.Sprintf(format, args...)})
	return o.save()
}

func (o *Owner) publish(event Event) {
	for subscriber := range o.subscribers {
		select {
		case subscriber <- event:
		default: // the subscriber is subscriberBuffer events behind
		}
	}
}

// describe names a forward for a log line, from its listening end to its
// target: "forward 3 on box (local localhost:4966 -> remote localhost:4966)".
func describe(forward workspace.Forward) string {
	from, to := "local "+forward.LocalEnd().String(), "remote "+forward.RemoteEnd().String()
	if forward.Direction == workspace.Remote {
		from, to = to, from
	}
	return fmt.Sprintf("forward %d on %s (%s -> %s)", forward.ID, forward.Host, from, to)
}
