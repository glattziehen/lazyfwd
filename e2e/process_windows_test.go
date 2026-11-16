package e2e

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func shortTempRoot() string { return os.TempDir() }

// ownProcessGroup lets interrupt reach lazyfwd serve alone.
func ownProcessGroup() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// interrupt sends ctrl+break to the process's group; Go delivers it to
// lazyfwd as os.Interrupt, as it does ctrl+c.
func interrupt(process *os.Process) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(process.Pid))
}

// leftoverSSH lists every ssh.exe process; the CI machine runs no other.
func leftoverSSH(string) []string {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return []string{err.Error()}
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	var found []string
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if name := windows.UTF16ToString(entry.ExeFile[:]); strings.EqualFold(name, "ssh.exe") {
			found = append(found, name+" pid "+strconv.Itoa(int(entry.ProcessID)))
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		found = append(found, err.Error())
	}
	return found
}
