package sshconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemConfigPathIsInProgramData(t *testing.T) {
	want := filepath.Join(os.Getenv("ProgramData"), "ssh", "ssh_config")
	if got := SystemConfigPath(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
