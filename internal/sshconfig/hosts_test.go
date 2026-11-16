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
