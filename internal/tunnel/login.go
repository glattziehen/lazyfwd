package tunnel

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// checkMaster asks the master on socketPath for its process ID.
func checkMaster(socketPath, host string) (int, bool) {
	if _, err := os.Stat(socketPath); err != nil {
		return 0, false
	}
	output, _ := runControl(context.Background(), controlArgs(socketPath, "check", host))
	return masterPID(string(output))
}

// runControl runs a short ssh command, such as a control command to a
// master, and ends it after controlTimeout or when ctx ends.
func runControl(ctx context.Context, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, controlTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.WaitDelay = time.Second
	return cmd.CombinedOutput()
}

// connectError explains a failed master from its log.
func connectError(host, logPath string, err error) *ConnectError {
	log, _ := os.ReadFile(logPath)
	reason := lastLine(string(log))
	if reason == "" {
		reason = err.Error()
	}
	return &ConnectError{Host: host, Reason: reason, Log: string(log), NeedsLogin: needsLogin(string(log))}
}
