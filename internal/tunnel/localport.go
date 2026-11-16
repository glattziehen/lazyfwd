package tunnel

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

// portProbeTimeout bounds a connect to a loopback port; loopback answers at
// once unless a listener's queue is full.
const portProbeTimeout = time.Second

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
