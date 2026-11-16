package workspace

import (
	"path/filepath"
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
