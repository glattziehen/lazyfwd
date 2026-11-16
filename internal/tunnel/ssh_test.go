package tunnel

import (
	"slices"
	"testing"

	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

func TestEverySSHCallEndsOptionsBeforeTheHost(t *testing.T) {
	host := "-oProxyCommand=evil"
	forward := workspace.Forward{Host: host, Direction: workspace.Local, Bind: workspace.End{Host: workspace.BindHost, Port: 9000}, Target: workspace.End{Host: "localhost", Port: 8000}}
	calls := map[string][]string{
		"batch master": batchMasterArgs("/s", "/s.log", host),
		"login":        loginArgs("/s", "/s.log", host),
		"control":      controlArgs("/s", "exit", host),
		"forward":      forwardArgs("/s", forward),
		"cancel":       cancelArgs("/s", forward),
		"probe":        probeArgs("/s", host, "/tmp/app.sock"),
	}
	for name, args := range calls {
		if len(args) < 2 || !slices.Equal(args[len(args)-2:], []string{"--", host}) {
			t.Errorf("%s args end %v, want -- %s", name, args, host)
		}
	}
	remote := remoteCommandArgs("/s", host, "ss -ltn")
	if !slices.Equal(remote[len(remote)-3:], []string{"--", host, "ss -ltn"}) {
		t.Errorf("remote command args end %v", remote)
	}
}

func TestConnectionsSkipForwardsFromSSHConfig(t *testing.T) {
	calls := map[string][]string{
		"batch master":   batchMasterArgs("/s", "/s.log", "box"),
		"login":          loginArgs("/s", "/s.log", "box"),
		"remote command": remoteCommandArgs("/s", "box", "true"),
	}
	for name, args := range calls {
		if !slices.Contains(args, "ClearAllForwardings=yes") {
			t.Errorf("%s would open the config's forwards: %v", name, args)
		}
	}
	forward := workspace.Forward{Host: "box", Direction: workspace.Local, Bind: workspace.End{Host: workspace.BindHost, Port: 9000}, Target: workspace.End{Host: "localhost", Port: 8000}}
	if args := forwardArgs("/s", forward); !slices.Equal(args[:2], []string{"-F", "none"}) {
		t.Errorf("-O forward would send the config's forwards: %v", args)
	}
}

func TestOnlyTheBatchMasterRefusesPrompts(t *testing.T) {
	if !slices.Contains(batchMasterArgs("/s", "/s.log", "box"), "BatchMode=yes") {
		t.Error("batch master may prompt")
	}
	if slices.Contains(loginArgs("/s", "/s.log", "box"), "BatchMode=yes") {
		t.Error("login cannot prompt")
	}
}

func TestNeedsLogin(t *testing.T) {
	cases := []struct {
		log  string
		want bool
	}{
		{"Host key verification failed.", true},
		{"user@10.0.0.1: Permission denied (publickey,password).", true},
		{"user@10.0.0.1: Permission denied (keyboard-interactive).", true},
		// A ProxyJump hop that needs a login ends with the outer connection closing.
		{"jump@bastion: Permission denied (publickey).\nConnection closed by UNKNOWN port 65535", true},
		{"ssh: connect to host 127.0.0.1 port 2299: Connection refused", false},
		{"ssh: Could not resolve hostname nope.invalid: Name or service not known", false},
		{"ssh: connect to host 10.0.0.1 port 22: Connection timed out", false},
	}
	for _, c := range cases {
		if got := needsLogin(c.log); got != c.want {
			t.Errorf("needsLogin(%q) = %v, want %v", c.log, got, c.want)
		}
	}
}

func TestMasterPID(t *testing.T) {
	if pid, ok := masterPID("Master running (pid=617517)\n"); !ok || pid != 617517 {
		t.Errorf("got %d, %v", pid, ok)
	}
	if _, ok := masterPID("Control socket connect(/s): No such file or directory"); ok {
		t.Error("read a pid from a failed check")
	}
}

func TestControlFailureKeepsSSHsReason(t *testing.T) {
	output := "mux_client_forward: forwarding request failed: remote port forwarding failed for listen port 8813\nmuxclient: master forward request failed\n"
	if got := controlFailure(output); got != "remote port forwarding failed for listen port 8813" {
		t.Errorf("got %q", got)
	}
}

// The remote shell command must not depend on callers having checked the
// path: it refuses one the forms would refuse.
func TestRemoveSocketCommandRefusesUnsafePath(t *testing.T) {
	if command, err := removeSocketCommand("/x/a.sock;touch /tmp/INJECTED;true"); err == nil {
		t.Fatalf("built %q", command)
	}
	if _, err := removeSocketCommand("/tmp/app.sock"); err != nil {
		t.Fatalf("refused a safe path: %v", err)
	}
}
