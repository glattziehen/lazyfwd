package listeners

import (
	"slices"
	"testing"
)

// ssOutput is real `ss -ltnp` output from a non-root user, trimmed.
const ssOutput = `State  Recv-Q Send-Q                          Local Address:Port  Peer Address:PortProcess
LISTEN 0      4096                100.126.194.88%tailscale0:3773       0.0.0.0:*
LISTEN 0      4096                                  0.0.0.0:22         0.0.0.0:*
LISTEN 0      4096                            127.0.0.53%lo:53         0.0.0.0:*
LISTEN 0      511                                 127.0.0.1:3773       0.0.0.0:*    users:(("node-MainThread",pid=2298,fd=24))
LISTEN 0      128                                 127.0.0.1:4966       0.0.0.0:*    users:(("diffle",pid=507415,fd=9))
LISTEN 0      4096                                     [::]:22            [::]:*
LISTEN 0      128                                     [::1]:4966          [::]:*    users:(("diffle",pid=507415,fd=8))
`

func TestParseMergesRowsPerPort(t *testing.T) {
	want := []Listener{{22, ""}, {53, ""}, {3773, "node-MainThread"}, {4966, "diffle"}}
	if got := Parse(ssOutput); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestOwnedDropsSystemServices(t *testing.T) {
	want := []Listener{{3773, "node-MainThread"}, {4966, "diffle"}}
	if got := Owned(Parse(ssOutput)); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
