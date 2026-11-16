package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

const (
	// socketName is the control socket's name in the owner's run directory.
	socketName = "control"
	// requestTimeout bounds reading a request and writing its response,
	// beyond the request's own wait.
	requestTimeout = time.Minute
)

// Server answers CLI commands for one owner.
type Server struct {
	listener net.Listener
	lock     *workspace.Lock
	wg       sync.WaitGroup
}

// Serve answers on a socket in runDir, a directory that only the user can
// access, and records the socket's path in lock so that CLI commands find
// it. Close stops answering.
func Serve(o *owner.Owner, lock *workspace.Lock, runDir string) (*Server, error) {
	path := filepath.Join(runDir, socketName)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// The directory already keeps others out; the mode says so on the socket too.
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	if err := lock.SetHolderAddress(path); err != nil {
		_ = listener.Close()
		return nil, err
	}
	s := &Server{listener: listener, lock: lock}
	s.wg.Add(1)
	go s.accept(o)
	return s, nil
}

// Address is the socket's path.
func (s *Server) Address() string {
	return s.listener.Addr().String()
}

// Close stops answering and waits for requests in progress. It leaves the
// lock to its holder.
func (s *Server) Close() {
	_ = s.lock.SetHolderAddress("")
	_ = s.listener.Close()
	s.wg.Wait()
}

func (s *Server) accept(o *owner.Owner) {
	defer s.wg.Done()
	for {
		connection, err := s.listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			answer(o, connection)
		}()
	}
}

func answer(o *owner.Owner, connection net.Conn) {
	defer func() { _ = connection.Close() }()
	_ = connection.SetDeadline(time.Now().Add(requestTimeout))
	var request Request
	if err := json.NewDecoder(bufio.NewReader(connection)).Decode(&request); err != nil {
		return
	}
	_ = connection.SetDeadline(time.Now().Add(requestTimeout + maxWait))
	_ = json.NewEncoder(connection).Encode(handle(o, request))
}
