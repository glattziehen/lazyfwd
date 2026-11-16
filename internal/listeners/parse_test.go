package listeners

import (
	"slices"
	"testing"
)

// ssOutput is real `ss -ltnp` output from a non-root user on Linux, trimmed.
const ssOutput = `lazyfwd:ss
State  Recv-Q Send-Q                          Local Address:Port  Peer Address:PortProcess
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

// macOutput is listCommand output on macOS from a non-root user: lsof sees
// only the user's own processes, netstat sees every listening port.
const macOutput = `lazyfwd:lsof
p612
cControlCenter
f9
n*:7000
f10
n[::]:7000
p48211
cnode
f23
n127.0.0.1:3000
p50102
cpostgres
f7
n[::1]:5432
f8
n127.0.0.1:5432
lazyfwd:netstat
Active Internet connections (including servers)
Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)
tcp4       0      0  127.0.0.1.3000         *.*                    LISTEN
tcp6       0      0  ::1.5432               *.*                    LISTEN
tcp4       0      0  127.0.0.1.5432         *.*                    LISTEN
tcp4       0      0  192.168.1.20.22        192.168.1.10.53412     ESTABLISHED
tcp46      0      0  *.7000                 *.*                    LISTEN
tcp6       0      0  fe80::1%lo0.631        *.*                    LISTEN
tcp46      0      0  *.22                   *.*                    LISTEN
`

func TestParseMergesLsofNamesIntoNetstatPorts(t *testing.T) {
	want := []Listener{{22, ""}, {631, ""}, {3000, "node"}, {5432, "postgres"}, {7000, "ControlCenter"}}
	if got := Parse(macOutput); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
