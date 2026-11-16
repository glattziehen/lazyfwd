package workspace

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func allFree(int) bool { return true }

func TestAddForwardsAvoidsPortsHeldByOtherForwards(t *testing.T) {
	var ws Workspace
	if _, err := ws.AddForwards("box-a", Local, []Addr{{"localhost", 4966}}, 0, allFree); err != nil {
		t.Fatal(err)
	}
	added, err := ws.AddForwards("box-b", Local, []Addr{{"localhost", 4966}, {"localhost", 4967}}, 0, allFree)
	if err != nil {
		t.Fatal(err)
	}
	if got := []int{added[0].BindPort, added[1].BindPort}; !slices.Equal(got, []int{4965, 4967}) {
		t.Fatalf("bind ports = %v", got)
	}
}

func TestAddForwardsUsesManualBindPortEvenIfTaken(t *testing.T) {
	var ws Workspace
	added, err := ws.AddForwards("box", Local, []Addr{{"localhost", 4966}}, 9000, func(int) bool { return false })
	if err != nil || added[0].BindPort != 9000 {
		t.Fatalf("got %v, %v", added, err)
	}
}

func TestAddForwardsBindsRemoteForwardOnTargetPort(t *testing.T) {
	var ws Workspace
	added, err := ws.AddForwards("box", Remote, []Addr{{"localhost", 3000}}, 0, func(int) bool { return false })
	if err != nil || added[0].BindPort != 3000 {
		t.Fatalf("got %v, %v", added, err)
	}
	if got := added[0].SSHArgs(); !slices.Contains(got, "-R") || !slices.Contains(got, "3000:localhost:3000") {
		t.Fatalf("ssh args = %v", got)
	}
}

func TestUpdateForwardKeepsOwnPort(t *testing.T) {
	var ws Workspace
	added, _ := ws.AddForwards("box", Local, []Addr{{"localhost", 4966}}, 0, allFree)
	updated, err := ws.UpdateForward(added[0].ID, Addr{"localhost", 4966}, 0, allFree)
	if err != nil || updated.BindPort != 4966 {
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
	_, _ = ws.AddForwards("box", Local, []Addr{{"localhost", 4966}}, 0, allFree)
	ws.RemoveHost("box")
	if hosts := ws.Hosts(nil); len(hosts) != 1 || hosts[0].IsManual || len(ws.ForwardsOf("box")) != 1 {
		t.Fatalf("hosts = %v, forwards = %v", hosts, ws.Forwards)
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "lazyfwd", "workspace.json")}
	var ws Workspace
	_ = ws.AddHost("box")
	_, _ = ws.AddForwards("box", Remote, []Addr{{"localhost", 3000}}, 0, allFree)
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
