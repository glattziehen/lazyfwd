package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

// A hand-edited or merged file may lack next_id or repeat an id; one delete
// must still remove exactly one forward, and no forward may be dropped.
func TestLoadGivesEveryForwardItsOwnID(t *testing.T) {
	ws, err := FileStore{Path: filepath.Join("testdata", "workspace_without_next_id.json")}.Load()
	if err != nil {
		t.Fatal(err)
	}
	added, err := ws.AddForwards("box", Local, []End{{Host: "localhost", Port: 8080}}, End{}, allFree)
	if err != nil {
		t.Fatal(err)
	}

	ws.DeleteForward(added[0].ID)

	var ports []int
	for _, forward := range ws.Forwards {
		ports = append(ports, forward.Bind.Port)
	}
	if !slices.Equal(ports, []int{4966, 4967, 4968}) {
		t.Fatalf("forwards left after deleting the added one: %v", ports)
	}
}

// Dotfiles managers keep workspace.json as a symlink into their repository;
// saving must update the target and keep the link and the file's mode.
func TestSaveWritesThroughSymlinkAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "workspace.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config", "workspace.json")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	var ws Workspace
	_ = ws.AddHost("box")

	if err := (FileStore{Path: link}).Save(ws); err != nil {
		t.Fatal(err)
	}

	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link replaced: %v, %v", info.Mode(), err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("target mode %v, %v; want 0600", info.Mode().Perm(), err)
	}
	if loaded, err := (FileStore{Path: target}).Load(); err != nil || !slices.Equal(loaded.ManualHosts, []string{"box"}) {
		t.Fatalf("target holds %+v, %v", loaded, err)
	}
}
