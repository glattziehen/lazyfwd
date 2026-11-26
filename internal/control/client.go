package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/glattziehen/lazyfwd/internal/owner"
	"github.com/glattziehen/lazyfwd/internal/workspace"
)

const (
	// findTimeout bounds waiting for a lock holder that answers on no
	// socket: a command without an owner holds the lock only for a moment,
	// and a starting owner records its socket once the socket listens.
	findTimeout  = 3 * time.Second
	findInterval = 50 * time.Millisecond
	dialTimeout  = time.Second
	// callTimeout bounds a call beyond its wait; the slowest operation is a
	// listening-ports fetch, which gives up after 20 seconds.
	callTimeout = 40 * time.Second
)

// Claim locks the workspace for a new owner. It fails when an owner runs.
func Claim(store workspace.FileStore) (*workspace.Lock, error) {
	holder, lock, err := find(store)
	if err != nil {
		return nil, err
	}
	if lock == nil {
		return nil, fmt.Errorf("another lazyfwd (PID %d) is already running with %s; use it, or quit it first", holder.PID, store.Path)
	}
	return lock, nil
}

// Client calls the workspace's owner: a running lazyfwd over its socket or,
// while none runs, the command's own Owner without connections.
type Client struct {
	address string
	local   *owner.Owner
	lock    *workspace.Lock
}

// Connect finds the owner of the workspace in store. While none runs, it
// locks the workspace until Close, so that no owner starts meanwhile, and
// loads it with the hosts that sshConfigHosts lists.
func Connect(store workspace.FileStore, sshConfigHosts func() ([]string, error)) (*Client, error) {
	holder, lock, err := find(store)
	if err != nil {
		return nil, err
	}
	if lock == nil {
		return &Client{address: holder.Address}, nil
	}
	ws, err := store.Load()
	if err != nil {
		lock.Release()
		return nil, err
	}
	hosts, err := sshConfigHosts()
	if err != nil {
		lock.Release()
		return nil, err
	}
	return &Client{local: owner.New(store, nil, ws, hosts), lock: lock}, nil
}

// HasOwner reports whether a lazyfwd runs to hold the connections.
func (c *Client) HasOwner() bool {
	return c.local == nil
}

// Call runs one operation. Its error is an *Error when the owner refused
// or failed the operation; the Response may still hold results.
func (c *Client) Call(request Request) (Response, error) {
	request.Version = protocolVersion
	var response Response
	if c.local != nil {
		response = handle(c.local, request)
	} else {
		var err error
		if response, err = call(c.address, request); err != nil {
			return Response{}, fmt.Errorf("ask the running lazyfwd: %w", err)
		}
	}
	if response.Error != nil {
		return response, response.Error
	}
	return response, nil
}

// Close releases the workspace when the command held it.
func (c *Client) Close() {
	if c.lock != nil {
		c.local.Close()
		c.lock.Release()
	}
}

func call(address string, request Request) (Response, error) {
	connection, err := net.DialTimeout("unix", address, dialTimeout)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = connection.Close() }()
	wait := min(time.Duration(request.WaitMillis)*time.Millisecond, maxWait)
	_ = connection.SetDeadline(time.Now().Add(callTimeout + wait))
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return Response{}, err
	}
	var response Response
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return Response{}, err
	}
	return response, nil
}

// find returns the running owner or, when none runs, the workspace's lock.
// The lock file's holder counts only while it answers at its address: a
// crashed owner leaves its record behind, but not its lock.
func find(store workspace.FileStore) (workspace.Holder, *workspace.Lock, error) {
	deadline := time.Now().Add(findTimeout)
	for {
		lock, err := store.Lock()
		if err == nil {
			return workspace.Holder{}, lock, nil
		}
		if !errors.Is(err, workspace.ErrLocked) {
			return workspace.Holder{}, nil, err
		}
		holder, err := store.Holder()
		if err != nil {
			return workspace.Holder{}, nil, err
		}
		if holder.Address != "" && answers(holder.Address) {
			return holder, nil, nil
		}
		if time.Now().After(deadline) {
			return workspace.Holder{}, nil, fmt.Errorf("another process holds %s.lock but answers on no control socket; if it is a lazyfwd from before this version, quit it and start it again", store.Path)
		}
		time.Sleep(findInterval)
	}
}

func answers(address string) bool {
	connection, err := net.DialTimeout("unix", address, dialTimeout)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}
