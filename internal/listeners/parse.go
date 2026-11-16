// Package listeners lists the TCP ports listening on a remote host by asking
// the host itself (ss on Linux, lsof and netstat on macOS, over ssh), never by
// probing ports.
package listeners

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Listener is one listening TCP port, merged across its bind addresses.
type Listener struct {
	Port int
	// Process is the owning process name. Without root, ss and lsof show it
	// only for the ssh user's own sockets, so it is empty for system services.
	Process string
}

// IsOwned reports whether the ssh user owns the listening process.
func (l Listener) IsOwned() bool {
	return l.Process != ""
}

// Section markers that listCommand prints before each tool's output.
const (
	sectionSS      = "lazyfwd:ss"
	sectionLsof    = "lazyfwd:lsof"
	sectionNetstat = "lazyfwd:netstat"
)

var processName = regexp.MustCompile(`users:\(\("([^"]*)"`)

// Parse reads the output of listCommand into listeners sorted by port. Rows
// for the same port (IPv4 and IPv6, several interfaces, lsof and netstat)
// merge into one, keeping a process name when any row has one.
func Parse(output string) []Listener {
	byPort := map[int]Listener{}
	add := func(port int, process string) {
		listener := byPort[port]
		listener.Port = port
		if listener.Process == "" {
			listener.Process = process
		}
		byPort[port] = listener
	}
	section, lsofCommand := "", ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case sectionSS, sectionLsof, sectionNetstat:
			section = line
			continue
		}
		switch section {
		case sectionSS:
			// LISTEN 0 128 127.0.0.1:4966 0.0.0.0:* users:(("diffle",pid=507415,fd=9))
			fields := strings.Fields(line)
			if len(fields) < 5 || fields[0] != "LISTEN" {
				continue
			}
			port, ok := portAfter(fields[3], ":")
			if !ok {
				continue
			}
			process := ""
			if match := processName.FindStringSubmatch(line); match != nil {
				process = match[1]
			}
			add(port, process)
		case sectionLsof:
			// lsof -F prints one field per line: p<pid>, c<command>, f<fd>, n<address>.
			if line == "" {
				continue
			}
			switch value := line[1:]; line[0] {
			case 'p':
				lsofCommand = ""
			case 'c':
				lsofCommand = value
			case 'n':
				if port, ok := portAfter(value, ":"); ok {
					add(port, lsofCommand)
				}
			}
		case sectionNetstat:
			// tcp4 0 0 127.0.0.1.4966 *.* LISTEN
			fields := strings.Fields(line)
			if len(fields) < 6 || !strings.HasPrefix(fields[0], "tcp") || fields[5] != "LISTEN" {
				continue
			}
			if port, ok := portAfter(fields[3], "."); ok {
				add(port, "")
			}
		}
	}
	listeners := make([]Listener, 0, len(byPort))
	for _, listener := range byPort {
		listeners = append(listeners, listener)
	}
	slices.SortFunc(listeners, func(a, b Listener) int { return cmp.Compare(a.Port, b.Port) })
	return listeners
}

// portAfter reads the port that follows the last separator in a local
// address: ':' for ss and lsof (127.0.0.1:4966), '.' for macOS netstat
// (127.0.0.1.4966).
func portAfter(address, separator string) (int, bool) {
	port, err := strconv.Atoi(address[strings.LastIndex(address, separator)+1:])
	return port, err == nil
}

// Owned keeps the listeners of the ssh user's own processes.
func Owned(listeners []Listener) []Listener {
	return slices.DeleteFunc(slices.Clone(listeners), func(l Listener) bool { return !l.IsOwned() })
}
