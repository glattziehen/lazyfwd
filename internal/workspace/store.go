package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(content, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}
