package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRunDirRemovesOnlyUnlockedLeftovers(t *testing.T) {
	base := filepath.Join(t.TempDir(), "run")
	crashed, held := filepath.Join(base, "1"), filepath.Join(base, "2")
	if err := ensurePrivateDir(base); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{crashed, held} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(crashed, lockName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := tryLock(filepath.Join(held, lockName))
	if err != nil || lock == nil {
		t.Fatalf("lock held dir: %v", err)
	}
	defer func() { _ = lock.Close() }()

	dir, err := openRunDir(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.remove()

	if _, err := os.Stat(crashed); !os.IsNotExist(err) {
		t.Errorf("crashed run directory survived: %v", err)
	}
	if _, err := os.Stat(held); err != nil {
		t.Errorf("held run directory was removed: %v", err)
	}
}

func TestDefaultBaseDirFitsSocketPaths(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/"+strings.Repeat("x", 120))
	if base := DefaultBaseDir(); !fitsSocketPath(base) {
		t.Errorf("%s leaves socket paths too long", base)
	}
}
