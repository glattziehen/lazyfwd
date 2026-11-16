//go:build unix

package tunnel

import "testing"

func TestOpenRunDirRefusesSharedBaseDir(t *testing.T) {
	base := t.TempDir() // 0755
	if _, err := openRunDir(base); err == nil {
		t.Fatal("accepted a base directory others can read")
	}
}
