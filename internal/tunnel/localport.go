package tunnel

import "fmt"

// loopbackHosts are the addresses ssh listens on for a forward bound to
// localhost. ssh reports success when it binds only one of them, so a forward
// on a port that another program holds on the other would answer on half of
// localhost and that program on the rest.
var loopbackHosts = []string{"127.0.0.1", "::1"}

func portInUseError(port int) error {
	return fmt.Errorf("localhost:%d is already in use", port)
}
