package tunnel

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DefaultBaseDir is the parent of every lazyfwd run directory: lazyfwd in
// the user's temp dir.
func DefaultBaseDir() string {
	return filepath.Join(os.TempDir(), "lazyfwd")
}

// ensurePrivateDir creates dir, or accepts an existing one, only if it is a
// real directory that this user owns and that grants access to no one else;
// others could otherwise reach what lazyfwd keeps there. Windows ignores
// permission bits, so dir gets an access list instead: owned by this user,
// full control for this user, nothing inherited from its parent.
func ensurePrivateDir(dir string) error {
	token, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	user := token.User.Sid
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	if err := createPrivateDir(dir, user); err != nil && !errors.Is(err, fs.ErrExist) {
		return &fs.PathError{Op: "mkdir", Path: dir, Err: err}
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	isPrivate, err := isPrivateTo(dir, user)
	if err != nil {
		return err
	}
	if !info.IsDir() || !isPrivate {
		return fmt.Errorf("%s must be a directory that only you can access", dir)
	}
	return nil
}

func createPrivateDir(dir string, user *windows.SID) error {
	// O: owner, D:P protected access list, A;OICI;FA full control that files
	// and directories inside inherit.
	descriptor, err := windows.SecurityDescriptorFromString("O:" + user.String() + "D:P(A;OICI;FA;;;" + user.String() + ")")
	if err != nil {
		return err
	}
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	attributes := windows.SecurityAttributes{SecurityDescriptor: descriptor}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	return windows.CreateDirectory(path, &attributes)
}

// isPrivateTo reports whether user owns dir and every entry that allows
// access names user, SYSTEM, or Administrators. The last two can take any
// file anyway, as root can on Unix.
func isPrivateTo(dir string, user *windows.SID) (bool, error) {
	descriptor, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, err
	}
	owner, _, err := descriptor.Owner()
	if err != nil || !owner.Equals(user) {
		return false, err
	}
	acl, _, err := descriptor.DACL()
	// No access list grants everyone full access.
	if err != nil || acl == nil {
		return false, err
	}
	for i := range uint32(acl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return false, err
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if !sid.Equals(user) && !sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
				return false, nil
			}
		default:
			// Object and callback entries may grant access in ways this
			// check does not read.
			return false, nil
		}
	}
	return true, nil
}
