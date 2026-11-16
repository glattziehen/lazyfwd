package tunnel

import (
	"net"
	"testing"
)

// A server on the wildcard address also answers on localhost; a forward on
// its port would shadow it on BSD-family systems, where binding 127.0.0.1
// next to *:port can succeed.
func TestIsLaptopPortFreeSeesWildcardListener(t *testing.T) {
	for _, address := range []string{"0.0.0.0:0", "[::]:0"} {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			t.Logf("skip %s: %v", address, err)
			continue
		}
		port := listener.Addr().(*net.TCPAddr).Port
		if IsLaptopPortFree(port) {
			t.Errorf("port %d with a listener on %s reported free", port, address)
		}
		listener.Close()
	}
}

func TestIsLaptopPortFreeSeesLoopbackListener(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if port := listener.Addr().(*net.TCPAddr).Port; IsLaptopPortFree(port) {
		t.Errorf("port %d with a loopback listener reported free", port)
	}
}
