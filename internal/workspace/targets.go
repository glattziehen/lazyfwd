package workspace

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// DefaultTargetHost is the target host when a target names only a port.
	DefaultTargetHost = "localhost"
	// MaxTargets bounds one target list, so a typo like 4000-49000 fails fast.
	MaxTargets = 64
)

// ParseTargets parses a comma-separated list of targets. Each item is
// [host:]port or [host:]first-last, e.g. "4966-4968", "db:5432,6379".
func ParseTargets(text string) ([]End, error) {
	var targets []End
	for _, item := range strings.Split(text, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
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
		return nil, fmt.Errorf("no target given, expected e.g. 4966, 4966-4968, or db:5432")
	}
	return targets, nil
}

// ParsePort parses a TCP port. An empty text means "pick automatically" and returns 0.
func ParsePort(text string) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	return parsePort(text)
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
