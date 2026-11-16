package workspace

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func allFree(int) bool { return true }

func TestAddForwardsAvoidsPortsHeldByOtherForwards(t *testing.T) {
	var ws Workspace
	if _, err := ws.AddForwards("box-a", Local, []End{{Host: "localhost", Port: 4966}}, End{}, allFree); err != nil {
		t.Fatal(err)
	}
	added, err := ws.AddForwards("box-b", Local, []End{{Host: "localhost", Port: 4966}, {Host: "localhost", Port: 4967}}, End{}, allFree)
	if err != nil {
		t.Fatal(err)
	}
	if got := []int{added[0].Bind.Port, added[1].Bind.Port}; !slices.Equal(got, []int{4965, 4967}) {
		t.Fatalf("bind ports = %v", got)
	}
}

func TestAddForwardsUsesManualBindPortEvenIfTaken(t *testing.T) {
	var ws Workspace
	added, err := ws.AddForwards("box", Local, []End{{Host: "localhost", Port: 4966}}, End{Host: BindHost, Port: 9000}, func(int) bool { return false })
	if err != nil || added[0].Bind.Port != 9000 {
		t.Fatalf("got %v, %v", added, err)
	}
}

func TestAddForwardsBindsRemoteForwardOnTargetPort(t *testing.T) {
	var ws Workspace
	added, err := ws.AddForwards("box", Remote, []End{{Host: "localhost", Port: 3000}}, End{}, func(int) bool { return false })
	if err != nil || added[0].Bind.Port != 3000 {
		t.Fatalf("got %v, %v", added, err)
	}
	if got := added[0].SSHArgs(); !slices.Contains(got, "-R") || !slices.Contains(got, "3000:localhost:3000") {
		t.Fatalf("ssh args = %v", got)
	}
}

func TestAddForwardsNeedsListeningEndForSocketTarget(t *testing.T) {
	for direction, side := range map[Direction]string{Local: "local", Remote: "remote"} {
		var ws Workspace
		_, err := ws.AddForwards("box", direction, []End{{Path: "/var/run/docker.sock"}}, End{}, allFree)
		if err == nil || !strings.Contains(err.Error(), "set the "+side+" end") {
			t.Errorf("%s: got %v", direction, err)
		}
	}
}

func TestSocketBindsHoldNoLocalPort(t *testing.T) {
	var ws Workspace
	_, _ = ws.AddForwards("box", Local, []End{{Host: "localhost", Port: 8080}}, End{Path: "/tmp/app.sock"}, allFree)

	added, err := ws.AddForwards("box", Local, []End{{Host: "localhost", Port: 8080}}, End{}, allFree)

	if err != nil || added[0].Bind != (End{Host: BindHost, Port: 8080}) {
		t.Fatalf("got %v, %v", added, err)
	}
}

func TestUpdateForwardKeepsOwnPort(t *testing.T) {
	var ws Workspace
	added, _ := ws.AddForwards("box", Local, []End{{Host: "localhost", Port: 4966}}, End{}, allFree)
	updated, err := ws.UpdateForward(added[0].ID, End{Host: "localhost", Port: 4966}, End{}, allFree)
	if err != nil || updated.Bind.Port != 4966 {
		t.Fatalf("got %v, %v", updated, err)
	}
}

func TestHostsListsPinnedFirstThenByName(t *testing.T) {
	var ws Workspace
	_ = ws.AddHost("zeta")
	ws.TogglePin("beta")
	var names []string
	for _, host := range ws.Hosts([]string{"alpha", "beta"}) {
		names = append(names, host.Name)
	}
	if !slices.Equal(names, []string{"beta", "zeta", "alpha"}) {
		t.Fatalf("hosts = %v", names)
	}
}

func TestRemoveHostKeepsItsForwards(t *testing.T) {
	var ws Workspace
	_ = ws.AddHost("box")
	_, _ = ws.AddForwards("box", Local, []End{{Host: "localhost", Port: 4966}}, End{}, allFree)
	ws.RemoveHost("box")
	if hosts := ws.Hosts(nil); len(hosts) != 1 || hosts[0].IsManual || len(ws.ForwardsOf("box")) != 1 {
		t.Fatalf("hosts = %v, forwards = %v", hosts, ws.Forwards)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "lazyfwd", "workspace.json")}
	var ws Workspace
	_ = ws.AddHost("box")
	_, _ = ws.AddForwards("box", Remote, []End{{Host: "localhost", Port: 3000}}, End{}, allFree)
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || !reflect.DeepEqual(loaded, ws) {
		t.Fatalf("loaded %+v, %v; want %+v", loaded, err, ws)
	}
}

func TestAddHostRejectsNameThatSSHReadsAsOption(t *testing.T) {
	var ws Workspace
	if err := ws.AddHost("-oProxyCommand=touch /tmp/pwned"); err == nil {
		t.Fatal("AddHost accepted a name starting with -")
	}
}

// Two forwards cannot listen on the same end: the master keeps one listener,
// so stopping either would close the other's.
func TestListeningEndsMustNotRepeat(t *testing.T) {
	port := End{Host: BindHost, Port: 47890}
	socket := End{Path: "/tmp/app.sock"}
	target := []End{{Host: "localhost", Port: 8811}}
	cases := []struct {
		name            string
		firstHost, host string
		firstDir, dir   Direction
		bind            End
		wantRefused     bool
	}{
		{"local port, other host", "a", "b", Local, Local, port, true},
		{"local socket, other host", "a", "b", Local, Local, socket, true},
		{"remote port, same host", "a", "a", Remote, Remote, port, true},
		{"remote socket, same host", "a", "a", Remote, Remote, socket, true},
		{"remote port, other host", "a", "b", Remote, Remote, port, false},
		{"local and remote port", "a", "a", Local, Remote, port, false},
	}
	for _, c := range cases {
		var ws Workspace
		if _, err := ws.AddForwards(c.firstHost, c.firstDir, target, c.bind, allFree); err != nil {
			t.Fatal(err)
		}
		_, err := ws.AddForwards(c.host, c.dir, target, c.bind, allFree)
		if isRefused := err != nil; isRefused != c.wantRefused {
			t.Errorf("%s: refused %v (%v), want %v", c.name, isRefused, err, c.wantRefused)
		}
	}
}

func TestEditCannotTakeAnotherForwardsListeningEnd(t *testing.T) {
	var ws Workspace
	_, _ = ws.AddForwards("a", Local, []End{{Host: "localhost", Port: 8811}}, End{Host: BindHost, Port: 47890}, allFree)
	added, _ := ws.AddForwards("a", Local, []End{{Host: "localhost", Port: 8812}}, End{Host: BindHost, Port: 47891}, allFree)

	_, err := ws.UpdateForward(added[0].ID, End{Host: "localhost", Port: 8812}, End{Host: BindHost, Port: 47890}, allFree)

	if err == nil {
		t.Fatal("edit took another forward's local port")
	}
	if _, err := ws.UpdateForward(added[0].ID, End{Host: "localhost", Port: 8813}, End{Host: BindHost, Port: 47891}, allFree); err != nil {
		t.Fatalf("edit refused the forward's own local port: %v", err)
	}
}
