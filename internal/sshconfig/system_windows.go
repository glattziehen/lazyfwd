package sshconfig

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// SystemConfigPath is the system-wide client config of Microsoft's OpenSSH,
// the ssh that Windows ships: ssh_config in the ssh folder of ProgramData.
// It is "" when Windows does not name that folder.
func SystemConfigPath() string {
	programData, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return ""
	}
	return filepath.Join(programData, "ssh", "ssh_config")
}
