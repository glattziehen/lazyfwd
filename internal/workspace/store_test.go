package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLockRefusesSecondHolderUntilReleased(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "workspace.json")}
	release, err := store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lock(); err == nil {
		t.Fatal("second Lock succeeded while the first was held")
	}
	release()
	releaseAgain, err := store.Lock()
	if err != nil {
		t.Fatalf("Lock after release: %v", err)
	}
	releaseAgain()
}

// Files written before socket support must load unchanged.
func TestLoadReadsWorkspaceFromBeforeSockets(t *testing.T) {
	store := FileStore{Path: filepath.Join("testdata", "workspace_before_sockets.json")}

	ws, err := store.Load()

	if err != nil {
		t.Fatal(err)
	}
	want := []Forward{
		{ID: 1, Host: "box", Direction: Local, Bind: End{Host: "localhost", Port: 4966}, Target: End{Host: "localhost", Port: 4966}},
		{ID: 2, Host: "box", Direction: Remote, Bind: End{Host: "localhost", Port: 3000}, Target: End{Host: "localhost", Port: 3000}},
	}
	if !reflect.DeepEqual(ws.Forwards, want) || ws.NextID != 2 {
		t.Fatalf("forwards = %+v, next id %d", ws.Forwards, ws.NextID)
	}
}

// A TCP forward keeps the stored shape older lazyfwd versions read.
func TestSaveWritesTCPForwardInTheShapeBeforeSockets(t *testing.T) {
	before, err := os.ReadFile(filepath.Join("testdata", "workspace_before_sockets.json"))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := FileStore{Path: filepath.Join("testdata", "workspace_before_sockets.json")}.Load()
	if err != nil {
		t.Fatal(err)
	}
	store := FileStore{Path: filepath.Join(t.TempDir(), "workspace.json")}

	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	saved, _ := os.ReadFile(store.Path)
	if string(saved) != string(before) {
		t.Fatalf("saved:\n%s\nwant:\n%s", saved, before)
	}
}

func TestFileStoreRoundTripsSocketEnds(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "workspace.json")}
	var ws Workspace
	_, _ = ws.AddForwards("box", Local, []End{{Path: "/var/run/docker.sock"}}, End{Host: BindHost, Port: 2375}, allFree)
	_, _ = ws.AddForwards("box", Remote, []End{{Host: "localhost", Port: 8080}}, End{Path: "/tmp/app.sock"}, allFree)
	if err := store.Save(ws); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load()

	if err != nil || !reflect.DeepEqual(loaded, ws) {
		t.Fatalf("loaded %+v, %v; want %+v", loaded, err, ws)
	}
}
