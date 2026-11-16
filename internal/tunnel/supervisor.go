// Package tunnel runs forwards as ssh child processes.
package tunnel

import (
	"bytes"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

// Exited reports that a forward's ssh process ended without Stop being called.
type Exited struct {
	ID workspace.ForwardID
	// Reason is ssh's last stderr line, or the exit error when stderr was empty.
	Reason string
}

// Supervisor owns one ssh process per running forward.
type Supervisor struct {
	mu        sync.Mutex
	processes map[workspace.ForwardID]*exec.Cmd
	exits     chan Exited
}

func NewSupervisor() *Supervisor {
	return &Supervisor{processes: map[workspace.ForwardID]*exec.Cmd{}, exits: make(chan Exited, 64)}
}

// Exits delivers one Exited per ssh process that ends on its own.
func (s *Supervisor) Exits() <-chan Exited {
	return s.exits
}

// Start runs ssh for the forward; starting a running forward is a no-op.
func (s *Supervisor) Start(forward workspace.Forward) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.processes[forward.ID]; ok {
		return nil
	}
	cmd := exec.Command("ssh", forward.SSHArgs()...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	s.processes[forward.ID] = cmd
	go s.wait(forward.ID, cmd, &stderr)
	return nil
}

func (s *Supervisor) wait(id workspace.ForwardID, cmd *exec.Cmd, stderr *bytes.Buffer) {
	err := cmd.Wait()
	s.mu.Lock()
	isStopped := s.processes[id] != cmd
	if !isStopped {
		delete(s.processes, id)
	}
	s.mu.Unlock()
	if isStopped {
		return
	}
	reason := lastLine(stderr.String())
	if reason == "" && err != nil {
		reason = err.Error()
	}
	if reason == "" {
		reason = "ssh exited"
	}
	s.exits <- Exited{ID: id, Reason: reason}
}

// Stop ends the forward's ssh process; stopping a stopped forward is a no-op.
func (s *Supervisor) Stop(id workspace.ForwardID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cmd, ok := s.processes[id]; ok {
		delete(s.processes, id)
		_ = cmd.Process.Kill()
	}
}

// StopAll ends every ssh process.
func (s *Supervisor) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cmd := range s.processes {
		delete(s.processes, id)
		_ = cmd.Process.Kill()
	}
}

// IsRunning reports whether the forward's ssh process is alive.
func (s *Supervisor) IsRunning(id workspace.ForwardID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.processes[id]
	return ok
}

// IsLaptopPortFree reports whether a TCP port on the laptop's loopback can be
// bound. ssh binds "localhost" on IPv4 and IPv6, so both must be free; a
// machine without IPv6 only needs IPv4.
func IsLaptopPortFree(port int) bool {
	portText := strconv.Itoa(port)
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", portText))
	if err != nil {
		return false
	}
	defer listener.Close()
	listener6, err := net.Listen("tcp6", net.JoinHostPort("::1", portText))
	if err != nil {
		return !errors.Is(err, syscall.EADDRINUSE)
	}
	_ = listener6.Close()
	return true
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
