package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestLockRefusesSecondHolderUntilReleased(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "workspace.json")}
	lock, err := store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lock(); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Lock while the first was held: %v, want ErrLocked", err)
	}
	lock.Release()
	lockAgain, err := store.Lock()
	if err != nil {
		t.Fatalf("Lock after release: %v", err)
	}
	lockAgain.Release()
}

// A client finds the running owner through the address in the lock file; a
// crashed owner's address must not outlive the next Lock.
func TestLockForgetsTheLastHoldersAddress(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "workspace.json")}
	lock, err := store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.SetHolderAddress("/run/old/control"); err != nil {
		t.Fatal(err)
	}
	lock.Release()

	lock, err = store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	if address, err := store.HolderAddress(); address != "" || err != nil {
		t.Fatalf("address = %q, %v; want empty", address, err)
	}
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
	// Windows keeps only a read-only flag, so 0600 reads back as 0666 there.
	before, err := os.Stat(target)
	if err != nil {
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
	if err != nil || info.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("target mode %v, %v; want %v", info.Mode().Perm(), err, before.Mode().Perm())
	}
	if loaded, err := (FileStore{Path: target}).Load(); err != nil || !slices.Equal(loaded.ManualHosts, []string{"box"}) {
		t.Fatalf("target holds %+v, %v", loaded, err)
	}
}

// A socket path reaches a remote shell, so a file holding one the forms would
// refuse must not load: lazyfwd would rewrite the file without the forward,
// so it refuses the whole file and names the entry.
func TestLoadRefusesSocketPathTheFormsWouldRefuse(t *testing.T) {
	path := filepath.Join("testdata", "workspace_injecting_socket_path.json")

	_, err := FileStore{Path: path}.Load()

	if err == nil {
		t.Fatal("loaded a socket path that holds shell syntax")
	}
	for _, part := range []string{path, "forward 2", "box", "may hold only"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error %q does not name %q", err, part)
		}
	}
}

func TestLoadRefusesForwardsTheFormsCannotMake(t *testing.T) {
	cases := map[string]string{
		"unknown direction":    `{"id": 1, "host": "box", "direction": "sideways", "bind_port": 9000, "target": {"host": "localhost", "port": 80}}`,
		"empty host":           `{"id": 1, "host": "", "direction": "local", "bind_port": 9000, "target": {"host": "localhost", "port": 80}}`,
		"host with a space":    `{"id": 1, "host": "a b", "direction": "local", "bind_port": 9000, "target": {"host": "localhost", "port": 80}}`,
		"port out of range":    `{"id": 1, "host": "box", "direction": "local", "bind_port": 70000, "target": {"host": "localhost", "port": 80}}`,
		"no bind":              `{"id": 1, "host": "box", "direction": "local", "target": {"host": "localhost", "port": 80}}`,
		"empty target host":    `{"id": 1, "host": "box", "direction": "local", "bind_port": 9000, "target": {"port": 80}}`,
		"relative target path": `{"id": 1, "host": "box", "direction": "local", "bind_port": 9000, "target": {"path": "run/app.sock"}}`,
	}
	for name, forward := range cases {
		path := filepath.Join(t.TempDir(), "workspace.json")
		if err := os.WriteFile(path, []byte(`{"forwards": [`+forward+`]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := (FileStore{Path: path}).Load(); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

func TestLoadKeepsNewIDsPositive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(path, []byte(`{"next_id": -5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ws, err := FileStore{Path: path}.Load()
	if err != nil {
		t.Fatal(err)
	}

	added, _ := ws.AddForwards("box", Local, []End{{Host: "localhost", Port: 80}}, End{Host: BindHost, Port: 9000}, allFree)

	if added[0].ID <= 0 {
		t.Fatalf("new forward got id %d", added[0].ID)
	}
}
