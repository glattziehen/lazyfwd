package sshconfig

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestHostsFollowsIncludesAndSkipsPatterns(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("config", "Include conf.d/*\nHost box gpu-* !bad\n  HostName 10.0.0.1\nhost=db\nHost *\n")
	write("conf.d/work", "Host\tworkbox # comment\n")
	got, err := Hosts(filepath.Join(dir, "config"))
	if err != nil || !slices.Equal(got, []string{"box", "db", "workbox"}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestHostsOfMissingConfigIsEmpty(t *testing.T) {
	got, err := Hosts(filepath.Join(t.TempDir(), "config"))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

// The system config resolves relative Includes against its own directory,
// /etc/ssh, not ~/.ssh.
func TestHostsMergesUserAndSystemConfigs(t *testing.T) {
	userDir, systemDir := t.TempDir(), t.TempDir()
	files := map[string]string{
		filepath.Join(userDir, "config"):                      "Host box\n",
		filepath.Join(systemDir, "ssh_config"):                "Include ssh_config.d/*.conf\nHost *\n",
		filepath.Join(systemDir, "ssh_config.d", "jump.conf"): "Match host x\nHost jump box\n",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Hosts(filepath.Join(userDir, "config"), filepath.Join(systemDir, "ssh_config"))
	if err != nil || !slices.Equal(got, []string{"box", "jump"}) {
		t.Fatalf("got %v, %v", got, err)
	}
}
