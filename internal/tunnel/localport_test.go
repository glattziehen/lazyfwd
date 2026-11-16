package tunnel

import (
	"fmt"
	"net"
	"testing"
)

// A server on the wildcard address also answers on localhost; a forward on
// its port would shadow it on BSD-family systems, where binding 127.0.0.1
// next to *:port can succeed.
func TestIsLocalPortFreeSeesWildcardListener(t *testing.T) {
	for _, address := range []string{"0.0.0.0:0", "[::]:0"} {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			t.Logf("skip %s: %v", address, err)
			continue
		}
		port := listener.Addr().(*net.TCPAddr).Port
		if IsLocalPortFree(port) {
			t.Errorf("port %d with a listener on %s reported free", port, address)
		}
		listener.Close()
	}
}

func TestIsLocalPortFreeSeesLoopbackListener(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if port := listener.Addr().(*net.TCPAddr).Port; IsLocalPortFree(port) {
		t.Errorf("port %d with a loopback listener reported free", port)
	}
}

// ssh reports success when it binds localhost on only one family, so a
// listener on either loopback address must make the forward fail.
func TestCheckLocalPortSeesListenerOnEitherLoopback(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "[::1]:0"} {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			t.Logf("skip %s: %v", address, err)
			continue
		}
		port := listener.Addr().(*net.TCPAddr).Port
		want := fmt.Sprintf("localhost:%d is already in use", port)
		if err := checkLocalPort(port); err == nil || err.Error() != want {
			t.Errorf("listener on %s: got %v, want %q", address, err, want)
		}
		listener.Close()
	}
}

// A forward that just stopped leaves its port's connections in TIME_WAIT;
// starting it again at once must work, also on macOS, where a bind without
// SO_REUSEADDR fails on such a port.
func TestCheckLocalPortAcceptsPortInTimeWait(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	client, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	// The side that closes first holds TIME_WAIT on the listener's port.
	server.Close()
	client.Close()
	listener.Close()
	if err := checkLocalPort(port); err != nil {
		t.Errorf("port %d in TIME_WAIT: %v", port, err)
	}
}
