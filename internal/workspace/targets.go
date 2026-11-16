package workspace

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// DefaultTargetHost is the target host when a target names only a port.
	DefaultTargetHost = "localhost"
	// MaxTargets bounds one target list, so a typo like 4000-49000 fails fast.
	MaxTargets = 64
	// maxSocketPathLength is the longest unix socket path macOS can bind
	// (Linux allows 107 bytes); either end may be a Mac.
	maxSocketPathLength = 103
)

// socketPathPattern is an absolute path in characters that need no quoting in
// ssh's forward spec or in a remote shell. ssh reads a spec part as a socket
// only if it holds a "/", expands neither "~" nor relative paths, and
// expands % tokens such as %h, so "%" is not allowed.
var socketPathPattern = regexp.MustCompile(`^/[A-Za-z0-9/._+@=-]+$`)

// ParseTargets parses a comma-separated list of targets. Each item is
// [host:]port, [host:]first-last, or an absolute unix socket path, e.g.
// "4966-4968", "db:5432,6379", "/var/run/docker.sock".
func ParseTargets(text string) ([]End, error) {
	var targets []End
	for _, item := range strings.Split(text, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if isPathLike(item) {
			socket, err := parseSocketPath(item)
			if err != nil {
				return nil, err
			}
			targets = append(targets, socket)
			continue
		}
		host, ports := DefaultTargetHost, item
		if i := strings.LastIndex(item, ":"); i >= 0 {
			host, ports = strings.Trim(item[:i], "[]"), item[i+1:]
		}
		if host == "" {
			return nil, fmt.Errorf("target %q has an empty host", item)
		}
		first, last, err := parsePortRange(ports)
		if err != nil {
			return nil, fmt.Errorf("target %q: %w", item, err)
		}
		for port := first; port <= last; port++ {
			targets = append(targets, End{Host: host, Port: port})
		}
		if len(targets) > MaxTargets {
			return nil, fmt.Errorf("%q expands to more than %d targets", text, MaxTargets)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no target given, expected e.g. 4966, 4966-4968, db:5432, or /run/app.sock")
	}
	return targets, nil
}

// ParseBind parses where a forward listens: a TCP port or an absolute unix
// socket path. An empty text means "pick automatically" and returns the zero
// End.
func ParseBind(text string) (End, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return End{}, nil
	}
	if isPathLike(text) {
		return parseSocketPath(text)
	}
	port, err := parsePort(text)
	if err != nil {
		return End{}, err
	}
	return End{Host: BindHost, Port: port}, nil
}

// isPathLike reports whether text is meant as a socket path rather than a
// port or host:port.
func isPathLike(text string) bool {
	return strings.ContainsAny(text, "/~")
}

func parseSocketPath(text string) (End, error) {
	switch {
	case strings.HasPrefix(text, "~"):
		return End{}, fmt.Errorf("socket path %q: write it out from /, since ssh does not expand ~", text)
	case !strings.HasPrefix(text, "/"):
		return End{}, fmt.Errorf("socket path %q must be absolute, starting with /", text)
	case len(text) > maxSocketPathLength:
		return End{}, fmt.Errorf("socket path %q is longer than %d bytes", text, maxSocketPathLength)
	case !socketPathPattern.MatchString(text):
		return End{}, fmt.Errorf("socket path %q may hold only letters, digits, and / . _ + @ = -", text)
	}
	return End{Path: text}, nil
}

func parsePortRange(text string) (int, int, error) {
	firstText, lastText, isRange := strings.Cut(text, "-")
	first, err := parsePort(firstText)
	if err != nil {
		return 0, 0, err
	}
	if !isRange {
		return first, first, nil
	}
	last, err := parsePort(lastText)
	if err != nil {
		return 0, 0, err
	}
	if last < first {
		return 0, 0, fmt.Errorf("range %d-%d is reversed", first, last)
	}
	return first, last, nil
}

func parsePort(text string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("port must be 1-65535, got %q", text)
	}
	return port, nil
}
