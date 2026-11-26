package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/glattziehen/lazyfwd/internal/lockfile"
)

// FileStore keeps the workspace in one JSON file.
type FileStore struct {
	Path string
}

// DefaultStorePath is lazyfwd/workspace.json in the user's config directory.
func DefaultStorePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazyfwd", "workspace.json"), nil
}

// Load reads the workspace; a missing file is an empty workspace.
func (s FileStore) Load() (Workspace, error) {
	var workspace Workspace
	content, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return workspace, nil
	}
	if err != nil {
		return workspace, err
	}
	if err := json.Unmarshal(content, &workspace); err != nil {
		return workspace, fmt.Errorf("parse %s: %w", s.Path, err)
	}
	// Refuse the whole file rather than skip the entry: the next save would
	// drop a skipped forward from the file without a word.
	if err := workspace.validate(); err != nil {
		return Workspace{}, fmt.Errorf("%s: %w; fix or remove that entry", s.Path, err)
	}
	workspace.repairIDs()
	return workspace, nil
}

// Save writes the workspace atomically, so a crash never leaves a half-written file.
func (s FileStore) Save(workspace Workspace) error {
	content, err := json.MarshalIndent(workspace, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	// Write to a symlink's target, as dotfiles managers link the file, and
	// keep the file's mode.
	path, mode := s.Path, fs.FileMode(0o644)
	if resolved, err := filepath.EvalSymlinks(s.Path); err == nil {
		path = resolved
	}
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(content, '\n'), mode); err != nil {
		return err
	}
	// WriteFile's mode passes through the umask.
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ErrLocked reports that another process holds the workspace's lock.
var ErrLocked = errors.New("another lazyfwd holds the workspace")

// Lock is this process's claim on the workspace. The lock file also holds
// the Holder, so other lazyfwd processes can name it and reach it.
type Lock struct {
	file *os.File
}

// Holder is the process that holds the workspace's lock.
type Holder struct {
	PID int `json:"pid"`
	// Address is where the holder answers, or "" while it answers nowhere.
	Address string `json:"address,omitempty"`
}

// Lock claims the workspace for this process until Release is called or the
// process ends, so two lazyfwd instances never overwrite each other's file.
// The lock is on a file next to the workspace; the system drops it when the
// process exits, crashes included, so no stale lock survives. It
// fails with ErrLocked while another process holds the lock.
func (s FileStore) Lock() (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	err = lockfile.TryLock(file)
	if errors.Is(err, lockfile.ErrLocked) {
		_ = file.Close()
		return nil, fmt.Errorf("%w %s", ErrLocked, s.Path)
	}
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock %s: %w", s.Path, err)
	}
	lock := &Lock{file: file}
	// This replaces a crashed holder's record.
	if err := lock.SetHolderAddress(""); err != nil {
		_ = file.Close()
		return nil, err
	}
	return lock, nil
}

// SetHolderAddress records where this process answers while it holds the lock.
func (l *Lock) SetHolderAddress(address string) error {
	content, err := json.Marshal(Holder{PID: os.Getpid(), Address: address})
	if err != nil {
		return err
	}
	if err := l.file.Truncate(0); err != nil {
		return err
	}
	_, err = l.file.WriteAt(content, 0)
	return err
}

// Release gives up the lock.
func (l *Lock) Release() {
	_ = l.file.Close()
}

// Holder is the process that recorded itself in the lock file, or the zero
// Holder when none did. It is meaningful only while Lock fails with
// ErrLocked: a crashed holder's record stays in the file until the next Lock.
// A record that does not parse, such as the bare address of a lazyfwd from
// before PIDs, counts as none.
func (s FileStore) Holder() (Holder, error) {
	content, err := os.ReadFile(s.lockPath())
	if errors.Is(err, fs.ErrNotExist) {
		return Holder{}, nil
	}
	if err != nil {
		return Holder{}, err
	}
	var holder Holder
	if decodeErr := json.Unmarshal(content, &holder); decodeErr != nil {
		holder = Holder{}
	}
	return holder, nil
}

func (s FileStore) lockPath() string {
	return s.Path + ".lock"
}

// forwardJSON is a Forward as workspace.json stores it. Files from before
// socket support have only bind_port; a socket bind goes in bind_path, so
// those files load unchanged.
type forwardJSON struct {
	ID        ForwardID `json:"id"`
	Host      string    `json:"host"`
	Direction Direction `json:"direction"`
	BindPort  int       `json:"bind_port,omitempty"`
	BindPath  string    `json:"bind_path,omitempty"`
	Target    End       `json:"target"`
}

func (f Forward) MarshalJSON() ([]byte, error) {
	return json.Marshal(forwardJSON{
		ID: f.ID, Host: f.Host, Direction: f.Direction,
		BindPort: f.Bind.Port, BindPath: f.Bind.Path,
		Target: f.Target,
	})
}

func (f *Forward) UnmarshalJSON(data []byte) error {
	var stored forwardJSON
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	bind := End{Host: BindHost, Port: stored.BindPort}
	if stored.BindPath != "" {
		bind = End{Path: stored.BindPath}
	}
	*f = Forward{ID: stored.ID, Host: stored.Host, Direction: stored.Direction, Bind: bind, Target: stored.Target}
	return nil
}
