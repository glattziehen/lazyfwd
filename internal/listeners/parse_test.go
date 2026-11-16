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
	if got, err := Parse(ssOutput); err != nil || !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestOwnedDropsSystemServices(t *testing.T) {
	want := []Listener{{3773, "node-MainThread"}, {4966, "diffle"}}
	found, _ := Parse(ssOutput)
	if got := Owned(found); !slices.Equal(got, want) {
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
lazyfwd:netstat-bsd
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
	if got, err := Parse(macOutput); err != nil || !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// The fixtures below are real listCommand output from a non-root user "dev"
// in containers, with root listening on 2222 and dev on 4966 (127.0.0.1) and
// 8080 (::). Alpine has busybox netstat; Debian slim has neither ss nor
// netstat, so the script reads /proc; Debian with net-tools has its netstat.
const busyboxOutput = `lazyfwd:netstat-linux
Active Internet connections (only servers)
Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
tcp        0      0 0.0.0.0:2222            0.0.0.0:*               LISTEN      -
tcp        0      0 127.0.0.1:4966          0.0.0.0:*               LISTEN      10/nc
tcp        0      0 :::8080                 :::*                    LISTEN      11/nc
`

const netToolsOutput = `lazyfwd:netstat-linux
Active Internet connections (only servers)
Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
tcp        0      0 0.0.0.0:2222            0.0.0.0:*               LISTEN      -
tcp        0      0 127.0.0.1:4966          0.0.0.0:*               LISTEN      134/perl
tcp6       0      0 :::8080                 :::*                    LISTEN      135/perl
`

const procOutput = `lazyfwd:proc
  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1366 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 1748238 1 0000000000000000 100 0 0 10 0
   1: 00000000:08AE 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1749198 1 0000000000000000 100 0 0 10 0
  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 1742779 1 0000000000000000 100 0 0 10 0
`

// sockstatOutput is FreeBSD output written from sockstat's documented format,
// not captured: sockstat names every user's processes, so only dev's count.
const sockstatOutput = `lazyfwd:user
dev
lazyfwd:sockstat
USER     COMMAND    PID   FD  PROTO  LOCAL ADDRESS         FOREIGN ADDRESS
dev      node       1234  20  tcp4   127.0.0.1:3000        *:*
root     sshd       812   3   tcp6   *:22                  *:*
root     sshd       812   4   tcp4   *:22                  *:*
dev      postgres   990   6   tcp6   ::1:5432              *:*
dev      postgres   990   7   tcp4   127.0.0.1:5432        *:*
root     syslogd    600   6   udp4   *:514                 *:*
`

func TestParseReadsEachLinuxAndBSDFormat(t *testing.T) {
	containerWant := []Listener{{2222, ""}, {4966, "perl"}, {8080, "perl"}}
	for _, test := range []struct {
		name   string
		output string
		want   []Listener
	}{
		{"busybox netstat", busyboxOutput, []Listener{{2222, ""}, {4966, "nc"}, {8080, "nc"}}},
		{"net-tools netstat", netToolsOutput, containerWant},
		{"/proc/net/tcp without names", procOutput, []Listener{{2222, ""}, {4966, ""}, {8080, ""}}},
		{"FreeBSD sockstat", sockstatOutput, []Listener{{22, ""}, {3000, "node"}, {5432, "postgres"}}},
	} {
		got, err := Parse(test.output)
		if err != nil || !slices.Equal(got, test.want) {
			t.Errorf("%s: got %v, %v, want %v", test.name, got, err, test.want)
		}
	}
}

func TestParseRejectsUnsupportedHost(t *testing.T) {
	_, err := Parse("lazyfwd:unsupported\nMINGW64_NT-10.0-19045\n")
	if err == nil || err.Error() != "MINGW64_NT-10.0-19045 hosts are not supported" {
		t.Fatalf("got %v", err)
	}
}

// A Windows host whose shell is cmd.exe or PowerShell never runs the script.
func TestParseRejectsOutputWithoutSections(t *testing.T) {
	if _, err := Parse(""); err == nil {
		t.Fatal("empty output parsed without error")
	}
}
