// Package listeners lists the TCP ports listening on a remote host by asking
// the host itself (ss -ltnp over ssh), never by probing ports.
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
	// Process is the owning process name. ss shows it only for the ssh user's
	// own sockets, so it is empty for system services.
	Process string
}

// IsOwned reports whether the ssh user owns the listening process.
func (l Listener) IsOwned() bool {
	return l.Process != ""
}

var processName = regexp.MustCompile(`users:\(\("([^"]*)"`)

// Parse reads `ss -ltnp` output into listeners sorted by port. Rows for the
// same port (IPv4 and IPv6, several interfaces) merge into one, keeping a
// process name when any row has one.
func Parse(output string) []Listener {
	byPort := map[int]Listener{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "LISTEN" {
			continue
		}
		local := fields[3]
		port, err := strconv.Atoi(local[strings.LastIndex(local, ":")+1:])
		if err != nil {
			continue
		}
		listener := byPort[port]
		listener.Port = port
		if match := processName.FindStringSubmatch(line); match != nil && listener.Process == "" {
			listener.Process = match[1]
		}
		byPort[port] = listener
	}
	listeners := make([]Listener, 0, len(byPort))
	for _, listener := range byPort {
		listeners = append(listeners, listener)
	}
	slices.SortFunc(listeners, func(a, b Listener) int { return cmp.Compare(a.Port, b.Port) })
	return listeners
}

// Owned keeps the listeners of the ssh user's own processes.
func Owned(listeners []Listener) []Listener {
	return slices.DeleteFunc(slices.Clone(listeners), func(l Listener) bool { return !l.IsOwned() })
}
