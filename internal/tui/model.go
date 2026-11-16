// Package tui is lazyfwd's terminal interface: Live, Hosts, Forwards, and Listening panels.
package tui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/moritzwilksch/lazyfwd/internal/browser"
	"github.com/moritzwilksch/lazyfwd/internal/listeners"
	"github.com/moritzwilksch/lazyfwd/internal/tunnel"
	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

// Store persists the workspace after every change.
type Store interface {
	Save(workspace.Workspace) error
}

// panel is a panel in focus order; its value is its number key.
type panel int

const (
	panelLive panel = iota
	panelHosts
	panelForwards
	panelListeners
	panelCount
)

type exitedMsg tunnel.Exited

// listenerResult is one fetch of a host's listening ports.
type listenerResult struct {
	listeners []listeners.Listener
	err       error
	isLoading bool
}

type listenersMsg struct {
	host   string
	result listenerResult
}

// Model is the bubbletea model. Forward status derives from the supervisor
// (running) and failures (last ssh error since the forward last started).
type Model struct {
	store          Store
	supervisor     *tunnel.Supervisor
	sshConfigHosts []string
	workspace      workspace.Workspace
	failures       map[workspace.ForwardID]string
	// listenerResults caches each host's listening ports for the session.
	listenerResults map[string]listenerResult

	focus     panel
	hostIndex int
	// liveID is the selected live forward. liveIndex is its row, which the
	// cursor keeps when that forward leaves the list.
	liveID                workspace.ForwardID
	liveIndex             int
	forwardIndex          int
	listenerIndex         int
	isShowingAllListeners bool
	isShowingHelp         bool
	form                  *form
	message               string
	width                 int
	height                int
}

func New(store Store, supervisor *tunnel.Supervisor, ws workspace.Workspace, sshConfigHosts []string) Model {
	return Model{
		store:           store,
		supervisor:      supervisor,
		sshConfigHosts:  sshConfigHosts,
		workspace:       ws,
		failures:        map[workspace.ForwardID]string{},
		listenerResults: map[string]listenerResult{},
		focus:           panelHosts,
	}
}

func (m Model) Init() tea.Cmd {
	return m.waitForExit()
}

func (m Model) waitForExit() tea.Cmd {
	exits := m.supervisor.Exits()
	return func() tea.Msg { return exitedMsg(<-exits) }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	// Any message can start, stop, or fail a forward, so the live list may
	// have changed under the cursor.
	next.selectLive(next.liveCursor(next.liveForwards()))
	return next, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case exitedMsg:
		m.failures[msg.ID] = msg.Reason
		return m, m.waitForExit()
	case listenersMsg:
		m.listenerResults[msg.host] = msg.result
		m.listenerIndex = clamp(m.listenerIndex, len(m.visibleListeners()))
		return m, nil
	case tea.KeyMsg:
		if m.form != nil {
			return m.updateForm(msg)
		}
		if m.isShowingHelp {
			m.isShowingHelp = false // any key closes the help
			return m, nil
		}
		return m.updateKey(msg)
	}
	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	m.message = ""
	// Methods that change m run before the return reads it; Go leaves the
	// order within one return statement unspecified.
	var cmd tea.Cmd
	switch key := msg.String(); key {
	case "q", "ctrl+c":
		m.supervisor.StopAll()
		return m, tea.Quit
	case "tab", "l", "right":
		cmd = m.focusPanel((m.focus + 1) % panelCount)
	case "shift+tab", "h", "left":
		cmd = m.focusPanel((m.focus + panelCount - 1) % panelCount)
	case "0":
		cmd = m.focusPanel(panelLive)
	case "1":
		cmd = m.focusPanel(panelHosts)
	case "2":
		cmd = m.focusPanel(panelForwards)
	case "3":
		cmd = m.focusPanel(panelListeners)
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "g", "<", "home":
		m.move(-maxRows)
	case "G", ">", "end":
		m.move(maxRows)
	case "ctrl+d", ".", "pgdown":
		m.move(m.halfPage())
	case "ctrl+u", ",", "pgup":
		m.move(-m.halfPage())
	case "?":
		m.isShowingHelp = true
	case "A":
		if m.selectedHost() != "" {
			m.form = newForwardForm(workspace.Remote)
		}
	case "r":
		cmd = m.fetchListeners()
	default:
		switch m.focus {
		case panelLive:
			cmd = m.updateLiveKey(key)
		case panelHosts:
			cmd = m.updateHostKey(key)
		case panelForwards:
			m.updateForwardKey(key)
		case panelListeners:
			m.updateListenerKey(key)
		}
	}
	return m, cmd
}

// focusPanel moves focus. Entering a host's forwards or listeners fetches its
// listeners once per session; r refetches.
func (m *Model) focusPanel(target panel) tea.Cmd {
	m.focus = target
	if target == panelLive || target == panelHosts {
		return nil
	}
	if _, ok := m.listenerResults[m.selectedHost()]; ok {
		return nil
	}
	return m.fetchListeners()
}

func (m *Model) fetchListeners() tea.Cmd {
	host := m.selectedHost()
	if host == "" {
		return nil
	}
	m.listenerResults[host] = listenerResult{isLoading: true}
	return func() tea.Msg {
		found, err := listeners.Fetch(host)
		return listenersMsg{host: host, result: listenerResult{listeners: found, err: err}}
	}
}

func (m *Model) updateHostKey(key string) tea.Cmd {
	if key == "a" {
		m.form = newHostForm()
		return nil
	}
	host, ok := m.selectedHostEntry()
	if !ok {
		return nil
	}
	switch key {
	case "enter":
		return m.focusPanel(panelForwards)
	case "p":
		m.workspace.TogglePin(host.Name)
		m.save()
		m.selectHost(host.Name)
	case "d":
		// Refuse rather than cascade, so no single key deletes more than one forward.
		if count := len(m.workspace.ForwardsOf(host.Name)); count > 0 {
			m.message = fmt.Sprintf("delete the forwards of %s first (%d left)", host.Name, count)
			return nil
		}
		if !host.IsManual {
			m.message = host.Name + " comes from your ssh config (~/.ssh/config or /etc/ssh/ssh_config); remove it there"
			return nil
		}
		m.workspace.RemoveHost(host.Name)
		m.save()
		m.clampHostIndex()
	}
	return nil
}

// updateLiveKey runs the Forwards panel's row keys on the selected live
// forward; enter shows that forward in the Forwards panel.
func (m *Model) updateLiveKey(key string) tea.Cmd {
	forward, ok := m.selectedLive()
	if !ok {
		return nil
	}
	if key == "enter" {
		m.selectHost(forward.Host)
		m.forwardIndex = slices.IndexFunc(m.forwards(), func(f workspace.Forward) bool { return f.ID == forward.ID })
		m.listenerIndex = 0
		return m.focusPanel(panelForwards)
	}
	m.actOnForward(key, forward)
	return nil
}

func (m *Model) updateForwardKey(key string) {
	if key == "a" && m.selectedHost() != "" {
		m.form = newForwardForm(workspace.Local)
		return
	}
	if forward, ok := m.selectedForward(); ok {
		m.actOnForward(key, forward)
	}
}

// actOnForward runs a forward's row key: start or stop, edit, delete, or open.
func (m *Model) actOnForward(key string, forward workspace.Forward) {
	switch key {
	case " ", "s":
		m.toggle(forward)
	case "e":
		m.form = newEditForm(forward)
	case "d":
		m.supervisor.Stop(forward.ID)
		delete(m.failures, forward.ID)
		m.workspace.DeleteForward(forward.ID)
		m.save()
		m.forwardIndex = clamp(m.forwardIndex, len(m.forwards()))
	case "o":
		url, isWeb := forward.LaptopURL()
		if !isWeb {
			m.message = fmt.Sprintf("%s is not a web port; connect with its client", forward.LaptopAddr())
			return
		}
		if err := browser.Open(url); err != nil {
			m.message = "open browser: " + err.Error()
		}
	}
}

func (m *Model) updateListenerKey(key string) {
	switch key {
	case "a":
		if m.selectedHost() != "" {
			m.form = newForwardForm(workspace.Local)
		}
	case "t":
		m.isShowingAllListeners = !m.isShowingAllListeners
		m.listenerIndex = 0
	case "enter", " ", "f":
		listener, ok := m.selectedListener()
		if !ok {
			return
		}
		if forward, ok := m.forwardTo(listener.Port); ok {
			m.toggle(forward)
			return
		}
		target := workspace.Addr{Host: workspace.DefaultTargetHost, Port: listener.Port}
		added, err := m.workspace.AddForwards(m.selectedHost(), workspace.Local, []workspace.Addr{target}, 0, tunnel.IsLaptopPortFree)
		if err != nil {
			m.message = err.Error()
			return
		}
		m.save()
		m.start(added[0])
	}
}

// forwardTo finds the selected host's local forward to localhost:port.
func (m Model) forwardTo(port int) (workspace.Forward, bool) {
	for _, forward := range m.forwards() {
		if forward.Direction == workspace.Local && forward.Target == (workspace.Addr{Host: workspace.DefaultTargetHost, Port: port}) {
			return forward, true
		}
	}
	return workspace.Forward{}, false
}

// visibleListeners are the selected host's fetched listeners, by default only
// those owned by the ssh user.
func (m Model) visibleListeners() []listeners.Listener {
	found := m.listenerResults[m.selectedHost()].listeners
	if m.isShowingAllListeners {
		return found
	}
	return listeners.Owned(found)
}

func (m Model) selectedListener() (listeners.Listener, bool) {
	visible := m.visibleListeners()
	if m.listenerIndex >= len(visible) {
		return listeners.Listener{}, false
	}
	return visible[m.listenerIndex], true
}

func (m Model) updateForm(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.form = nil
		return m, nil
	case "enter":
		if err := m.submit(); err != nil {
			m.message = err.Error()
			return m, nil
		}
		m.form, m.message = nil, ""
		return m, nil
	}
	return m, m.form.update(msg)
}

func (m *Model) submit() error {
	f := m.form
	switch f.kind {
	case formAddHost:
		if err := m.workspace.AddHost(f.value(0)); err != nil {
			return err
		}
		m.save()
		m.selectHost(f.value(0))
		return nil
	case formAddForward:
		targets, err := workspace.ParseTargets(f.value(0))
		if err != nil {
			return err
		}
		bindPort, err := workspace.ParsePort(f.value(1))
		if err != nil {
			return err
		}
		added, err := m.workspace.AddForwards(m.selectedHost(), f.direction, targets, bindPort, tunnel.IsLaptopPortFree)
		if err != nil {
			return err
		}
		m.save()
		for _, forward := range added {
			m.start(forward)
		}
		m.focus = panelForwards
		m.forwardIndex = len(m.workspace.ForwardsOf(m.selectedHost())) - 1
		return nil
	case formEditForward:
		targets, err := workspace.ParseTargets(f.value(0))
		if err != nil {
			return err
		}
		if len(targets) != 1 {
			return fmt.Errorf("a forward has one target, got %d", len(targets))
		}
		bindPort, err := workspace.ParsePort(f.value(1))
		if err != nil {
			return err
		}
		// Stop first so the forward's own laptop port counts as free again.
		wasRunning := m.supervisor.IsRunning(f.forwardID)
		m.supervisor.Stop(f.forwardID)
		delete(m.failures, f.forwardID)
		forward, err := m.workspace.UpdateForward(f.forwardID, targets[0], bindPort, tunnel.IsLaptopPortFree)
		if err != nil {
			return err
		}
		m.save()
		if wasRunning {
			m.start(forward)
		}
		return nil
	}
	return nil
}

// toggle stops a running forward and starts a stopped or failed one.
func (m *Model) toggle(forward workspace.Forward) {
	if m.supervisor.IsRunning(forward.ID) {
		m.supervisor.Stop(forward.ID)
		delete(m.failures, forward.ID)
		return
	}
	m.start(forward)
}

func (m *Model) start(forward workspace.Forward) {
	delete(m.failures, forward.ID)
	if err := m.supervisor.Start(forward); err != nil {
		m.failures[forward.ID] = err.Error()
	}
}

func (m *Model) save() {
	if err := m.store.Save(m.workspace); err != nil {
		m.message = "save workspace: " + err.Error()
	}
}

// maxRows is larger than any list, so moving by it lands on the first or last row.
const maxRows = 1 << 30

// halfPage is half the screen height in rows, like vim's ctrl+d.
func (m Model) halfPage() int {
	return max(1, m.height/2)
}

func (m *Model) move(delta int) {
	switch m.focus {
	case panelLive:
		m.selectLive(m.liveCursor(m.liveForwards()) + delta)
	case panelHosts:
		m.hostIndex = clamp(m.hostIndex+delta, len(m.hosts()))
		m.forwardIndex, m.listenerIndex = 0, 0
	case panelForwards:
		m.forwardIndex = clamp(m.forwardIndex+delta, len(m.forwards()))
	case panelListeners:
		m.listenerIndex = clamp(m.listenerIndex+delta, len(m.visibleListeners()))
	}
}

func (m Model) hosts() []workspace.Host {
	return m.workspace.Hosts(m.sshConfigHosts)
}

func (m Model) forwards() []workspace.Forward {
	return m.workspace.ForwardsOf(m.selectedHost())
}

func (m Model) selectedHostEntry() (workspace.Host, bool) {
	hosts := m.hosts()
	if m.hostIndex >= len(hosts) {
		return workspace.Host{}, false
	}
	return hosts[m.hostIndex], true
}

func (m Model) selectedHost() string {
	host, _ := m.selectedHostEntry()
	return host.Name
}

func (m Model) selectedForward() (workspace.Forward, bool) {
	forwards := m.forwards()
	if m.forwardIndex >= len(forwards) {
		return workspace.Forward{}, false
	}
	return forwards[m.forwardIndex], true
}

// liveForwards are the running and failed forwards of all hosts, in host-list order.
func (m Model) liveForwards() []workspace.Forward {
	var live []workspace.Forward
	for _, host := range m.hosts() {
		for _, forward := range m.workspace.ForwardsOf(host.Name) {
			if m.status(forward) != statusStopped {
				live = append(live, forward)
			}
		}
	}
	return live
}

// liveCursor is the row of the selected live forward, or the row it left.
func (m Model) liveCursor(live []workspace.Forward) int {
	if i := slices.IndexFunc(live, func(f workspace.Forward) bool { return f.ID == m.liveID }); i >= 0 {
		return i
	}
	return clamp(m.liveIndex, len(live))
}

func (m *Model) selectLive(index int) {
	live := m.liveForwards()
	m.liveIndex = clamp(index, len(live))
	m.liveID = 0
	if m.liveIndex < len(live) {
		m.liveID = live[m.liveIndex].ID
	}
}

func (m Model) selectedLive() (workspace.Forward, bool) {
	live := m.liveForwards()
	i := m.liveCursor(live)
	if i >= len(live) {
		return workspace.Forward{}, false
	}
	return live[i], true
}

// focusedForward is the forward under the cursor of a focused forward list.
func (m Model) focusedForward() (workspace.Forward, bool) {
	switch m.focus {
	case panelLive:
		return m.selectedLive()
	case panelForwards:
		return m.selectedForward()
	}
	return workspace.Forward{}, false
}

func (m *Model) selectHost(name string) {
	for i, host := range m.hosts() {
		if host.Name == name {
			m.hostIndex = i
			return
		}
	}
}

func (m *Model) clampHostIndex() {
	m.hostIndex = clamp(m.hostIndex, len(m.hosts()))
	m.forwardIndex, m.listenerIndex = 0, 0
}

// clamp keeps i within [0, n-1], or 0 when n is 0.
func clamp(i, n int) int {
	return max(0, min(i, n-1))
}
