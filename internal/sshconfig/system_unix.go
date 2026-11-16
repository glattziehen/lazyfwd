//go:build unix

package sshconfig

// SystemConfigPath is OpenSSH's system-wide client config.
func SystemConfigPath() string {
	return "/etc/ssh/ssh_config"
}
