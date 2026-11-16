// Package sshconfig lists the concrete host aliases of an OpenSSH client config.
package sshconfig

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// maxIncludeDepth matches OpenSSH's limit on nested Include directives.
const maxIncludeDepth = 16

// SystemConfigPath is OpenSSH's system-wide client config on Linux and macOS.
const SystemConfigPath = "/etc/ssh/ssh_config"

// Hosts lists the aliases named in Host lines of the configs at paths and the
// files they include. Patterns (*, ?, negations) are skipped since they name
// no single host, and so are aliases no host name can have, such as systemd's
// ".host" and "machine/.host" proxy aliases in /etc/ssh/ssh_config.d. Match blocks name no aliases, so they add none. Like ssh, it
// skips a config it cannot open, such as a missing one.
func Hosts(paths ...string) ([]string, error) {
	var hosts []string
	for _, path := range paths {
		if err := collect(path, filepath.Dir(path), 0, &hosts); err != nil {
			return nil, err
		}
	}
	slices.Sort(hosts)
	return slices.Compact(hosts), nil
}

func collect(path, sshDir string, depth int, hosts *[]string) error {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		keyword, args := splitDirective(scanner.Text())
		switch keyword {
		case "host":
			for _, alias := range args {
				if !strings.ContainsAny(alias, "*?!/") && !strings.HasPrefix(alias, ".") {
					*hosts = append(*hosts, alias)
				}
			}
		case "include":
			if depth >= maxIncludeDepth {
				continue
			}
			for _, pattern := range args {
				if !filepath.IsAbs(pattern) && !strings.HasPrefix(pattern, "~") {
					pattern = filepath.Join(sshDir, pattern)
				}
				pattern = expandHome(pattern)
				matches, _ := filepath.Glob(pattern)
				for _, match := range matches {
					if err := collect(match, sshDir, depth+1, hosts); err != nil {
						return err
					}
				}
			}
		}
	}
	return scanner.Err()
}

// splitDirective splits "Keyword arg1 arg2" or "Keyword=arg" into a lowercase
// keyword and its args. Splitting on "=" everywhere is safe for Host and
// Include, the only directives read here.
func splitDirective(line string) (string, []string) {
	line, _, _ = strings.Cut(line, "#")
	fields := strings.FieldsFunc(strings.ReplaceAll(line, "\"", ""), func(r rune) bool {
		return r == ' ' || r == '\t' || r == '='
	})
	if len(fields) == 0 {
		return "", nil
	}
	return strings.ToLower(fields[0]), fields[1:]
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
