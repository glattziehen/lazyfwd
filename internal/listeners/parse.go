// Package listeners lists the TCP ports listening on a remote host by asking
// the host itself over ssh (ss, netstat, sockstat, lsof, or /proc, whichever
// the platform has), never by probing ports.
package listeners

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Listener is one listening TCP port, merged across its bind addresses.
type Listener struct {
	Port int
	// Process is the owning process name. It is empty for other users'
	// processes, such as system services and Docker's docker-proxy, and for
	// every port when the host's tool cannot name processes.
	Process string
}

// IsOwned reports whether the ssh user owns the listening process.
func (l Listener) IsOwned() bool {
	return l.Process != ""
}

// Section markers that listCommand prints before each tool's output.
const (
	sectionSS           = "lazyfwd:ss"
	sectionNetstatLinux = "lazyfwd:netstat-linux"
	sectionProc         = "lazyfwd:proc"
	sectionUser         = "lazyfwd:user"
	sectionSockstat     = "lazyfwd:sockstat"
	sectionLsof         = "lazyfwd:lsof"
	sectionNetstatBSD   = "lazyfwd:netstat-bsd"
	sectionUnsupported  = "lazyfwd:unsupported"
)

var errNoScriptOutput = errors.New("the host did not run lazyfwd's sh script (is it a Windows host?)")

var processName = regexp.MustCompile(`users:\(\("([^"]*)"`)

// Parse reads the output of listCommand into listeners sorted by port. Rows
// for the same port (IPv4 and IPv6, several interfaces, lsof and netstat)
// merge into one, keeping a process name when any row has one. It fails when
// the output holds no section, or names an unsupported OS.
func Parse(output string) ([]Listener, error) {
	byPort := map[int]Listener{}
	add := func(port int, process string) {
		listener := byPort[port]
		listener.Port = port
		if listener.Process == "" {
			listener.Process = process
		}
		byPort[port] = listener
	}
	section, user, lsofCommand := "", "", ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case sectionSS, sectionNetstatLinux, sectionProc, sectionUser, sectionSockstat, sectionLsof, sectionNetstatBSD, sectionUnsupported:
			section = line
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		switch section {
		case sectionSS:
			// LISTEN 0 128 127.0.0.1:4966 0.0.0.0:* users:(("diffle",pid=507415,fd=9))
			if len(fields) < 5 || fields[0] != "LISTEN" {
				continue
			}
			port, ok := portAfter(fields[3], ":", 10)
			if !ok {
				continue
			}
			process := ""
			if match := processName.FindStringSubmatch(line); match != nil {
				process = match[1]
			}
			add(port, process)
		case sectionNetstatLinux:
			// tcp 0 0 127.0.0.1:4966 0.0.0.0:* LISTEN 507415/diffle
			// The program column is "-" for other users' processes.
			if len(fields) < 7 || !strings.HasPrefix(fields[0], "tcp") || fields[5] != "LISTEN" {
				continue
			}
			if port, ok := portAfter(fields[3], ":", 10); ok {
				_, process, _ := strings.Cut(strings.Join(fields[6:], " "), "/")
				add(port, process)
			}
		case sectionProc:
			// 0: 0100007F:1366 00000000:0000 0A ..., with the port in hex; state 0A is LISTEN.
			if len(fields) < 4 || fields[3] != "0A" {
				continue
			}
			if port, ok := portAfter(fields[1], ":", 16); ok {
				add(port, "")
			}
		case sectionUser:
			user = line
		case sectionSockstat:
			// moritz node 1234 20 tcp4 127.0.0.1:3000 *:*
			if len(fields) < 6 || !strings.HasPrefix(fields[4], "tcp") {
				continue
			}
			if port, ok := portAfter(fields[5], ":", 10); ok {
				process := ""
				if fields[0] == user {
					process = fields[1]
				}
				add(port, process)
			}
		case sectionLsof:
			// lsof -F prints one field per line: p<pid>, c<command>, f<fd>, n<address>.
			switch value := line[1:]; line[0] {
			case 'p':
				lsofCommand = ""
			case 'c':
				lsofCommand = value
			case 'n':
				if port, ok := portAfter(value, ":", 10); ok {
					add(port, lsofCommand)
				}
			}
		case sectionNetstatBSD:
			// tcp4 0 0 127.0.0.1.4966 *.* LISTEN
			if len(fields) < 6 || !strings.HasPrefix(fields[0], "tcp") || fields[5] != "LISTEN" {
				continue
			}
			if port, ok := portAfter(fields[3], ".", 10); ok {
				add(port, "")
			}
		case sectionUnsupported:
			return nil, fmt.Errorf("%s hosts are not supported", line)
		}
	}
	if section == "" {
		return nil, errNoScriptOutput
	}
	listeners := make([]Listener, 0, len(byPort))
	for _, listener := range byPort {
		listeners = append(listeners, listener)
	}
	slices.SortFunc(listeners, func(a, b Listener) int { return cmp.Compare(a.Port, b.Port) })
	return listeners, nil
}

// portAfter reads the port that follows the last separator in a local
// address, in the given base: ':' for most tools (127.0.0.1:4966), '.' for
// BSD netstat (127.0.0.1.4966), and base 16 for /proc (0100007F:1366).
func portAfter(address, separator string, base int) (int, bool) {
	port, err := strconv.ParseInt(address[strings.LastIndex(address, separator)+1:], base, 0)
	return int(port), err == nil
}

// Owned keeps the listeners of the ssh user's own processes.
func Owned(listeners []Listener) []Listener {
	return slices.DeleteFunc(slices.Clone(listeners), func(l Listener) bool { return !l.IsOwned() })
}
