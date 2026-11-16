package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"
)

// portProbeTimeout bounds a connect to a loopback port; loopback answers at
// once unless a listener's queue is full.
const portProbeTimeout = time.Second

// IsLocalPortFree reports whether a TCP port on the local loopback can be
// bound without sharing it with another listener, including one on the
// wildcard address. ssh binds "localhost" on IPv4 and IPv6, so both must be
// free; a machine without IPv6 only needs IPv4.
func IsLocalPortFree(port int) bool {
	config := net.ListenConfig{Control: exclusiveBind}
	portText := strconv.Itoa(port)
	listener, err := config.Listen(context.Background(), "tcp4", net.JoinHostPort("127.0.0.1", portText))
	if err != nil {
		return false
	}
	defer func() { _ = listener.Close() }()
	listener6, err := config.Listen(context.Background(), "tcp6", net.JoinHostPort("::1", portText))
	if err != nil {
		return !errors.Is(err, errAddrInUse)
	}
	_ = listener6.Close()
	return true
}

// loopbackHosts are the addresses ssh listens on for a forward bound to
// localhost. ssh reports success when it binds only one of them, so a forward
// on a port that another program holds on the other would answer on half of
// localhost and that program on the rest.
var loopbackHosts = []string{"127.0.0.1", "::1"}

// checkLocalPort reports an error naming the port when something listens on
// it on the local loopback. It connects rather than binds: a port whose
// connections sit in TIME_WAIT, as right after a forward stops, refuses the
// connection and counts as free on every system. Without IPv6 the ::1
// connect fails and counts as free.
func checkLocalPort(port int) error {
	for _, host := range loopbackHosts {
		connection, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), portProbeTimeout)
		if err == nil {
			_ = connection.Close()
			return portInUseError(port)
		}
	}
	return nil
}

func portInUseError(port int) error {
	return fmt.Errorf("localhost:%d is already in use", port)
}
