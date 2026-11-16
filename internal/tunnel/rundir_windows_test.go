package tunnel

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestOpenRunDirRefusesSharedBaseDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "run")
	// Everyone (WD) may read and list, as a shared directory might allow.
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;OW)(A;OICI;FR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(base, 0o700); err != nil {
		t.Fatal(err)
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(base, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := openRunDir(base); err == nil {
		t.Fatal("accepted a base directory everyone can read")
	}
}
