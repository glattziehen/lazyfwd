package tunnel

import (
	"context"
	"errors"
	"net"
	"strconv"
	"syscall"
)

// IsLaptopPortFree reports whether a TCP port on the laptop's loopback can be
// bound without sharing it with another listener, including one on the
// wildcard address. ssh binds "localhost" on IPv4 and IPv6, so both must be
// free; a machine without IPv6 only needs IPv4.
func IsLaptopPortFree(port int) bool {
	config := net.ListenConfig{Control: exclusiveBind}
	portText := strconv.Itoa(port)
	listener, err := config.Listen(context.Background(), "tcp4", net.JoinHostPort("127.0.0.1", portText))
	if err != nil {
		return false
	}
	defer listener.Close()
	listener6, err := config.Listen(context.Background(), "tcp6", net.JoinHostPort("::1", portText))
	if err != nil {
		return !errors.Is(err, syscall.EADDRINUSE)
	}
	_ = listener6.Close()
	return true
}
