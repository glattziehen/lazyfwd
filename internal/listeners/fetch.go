package listeners

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	fetchTimeout = 20 * time.Second
	// listCommand runs on the remote host; one ssh round trip per fetch.
	listCommand = "ss -ltnp"
)

// Fetch lists the ports listening on host with one ssh command.
func Fetch(host string) ([]Listener, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", host, listCommand)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = err.Error()
		}
		return nil, fmt.Errorf("%q on %s failed: %s", listCommand, host, reason)
	}
	return Parse(stdout.String()), nil
}
