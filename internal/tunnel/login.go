package tunnel

import (
	"context"
	"os/exec"
	"time"
)

// runControl runs a short ssh command, such as a control command to a
// master, and ends it after controlTimeout or when ctx ends.
func runControl(ctx context.Context, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, controlTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.WaitDelay = time.Second
	return cmd.CombinedOutput()
}
