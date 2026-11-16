package tunnel

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/moritzwilksch/lazyfwd/internal/workspace"
)

// A host's connection is one OpenSSH ControlMaster on a lazyfwd-owned
// socket. Forwards live on the master; the listening-ports fetch runs as a
// client of it. Every ssh call ends in "--" and the host, so a host that
// starts with "-" cannot pass as an option.

// clearConfigForwards keeps the LocalForward, RemoteForward, and
// DynamicForward lines of ~/.ssh/config off lazyfwd's connections, where they
// would hold ports that no forward in lazyfwd shows. It clears command-line
// forwards too, so control commands read no config instead.
const clearConfigForwards = "ClearAllForwardings=yes"

// masterOptions set up a background master that logs to logPath instead of
// the terminal. The command line wins over ~/.ssh/config, so a user's own
// ControlMaster, ControlPath, and ControlPersist do not apply to it.
func masterOptions(socketPath, logPath string) []string {
	return []string{
		"-f", "-N",
		"-E", logPath,
		"-o", clearConfigForwards,
		"-o", "ControlMaster=yes",
		"-o", "ControlPath=" + socketPath,
		"-o", "ControlPersist=no",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
	}
}

// batchMasterArgs open the master without prompting; they fail when the host
// needs a password, a passphrase, a host key decision, or 2FA.
func batchMasterArgs(socketPath, logPath, host string) []string {
	args := append(masterOptions(socketPath, logPath), "-o", "BatchMode=yes")
	return append(args, "--", host)
}

// loginArgs open the master on the terminal, so the user can answer ssh's
// prompts; ssh moves to the background once authenticated.
func loginArgs(socketPath, logPath, host string) []string {
	return append(masterOptions(socketPath, logPath), "--", host)
}

// controlArgs send a control command ("check", "exit", "forward", "cancel")
// to the master on socketPath. They read no ssh config: a control command
// sends the config's forwards along with its own, and it needs nothing else
// from the config since it never connects to the host.
func controlArgs(socketPath, command, host string, extra ...string) []string {
	args := append([]string{"-F", "none", "-S", socketPath, "-O", command}, extra...)
	return append(args, "--", host)
}

// remoteCommandArgs run command on host through the master, or over a fresh
// connection without prompts when the master is gone.
func remoteCommandArgs(socketPath, host, command string) []string {
	return []string{
		"-S", socketPath,
		"-o", "ControlMaster=no",
		"-o", clearConfigForwards,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"--", host, command,
	}
}

// probeArgs connect stdin and stdout through the master to target on host:
// a unix socket path or host:port. ssh exits 255 at once when nothing
// accepts on target; a live listener takes the connection.
func probeArgs(socketPath, host, target string) []string {
	return []string{
		"-S", socketPath,
		"-o", "ControlMaster=no",
		"-o", "BatchMode=yes",
		"-W", target,
		"--", host,
	}
}

// removeSocketCommand removes path on a remote host if it is a socket. It
// runs under sh, whatever the login shell; socket paths hold no characters
// that need quoting.
func removeSocketCommand(path string) string {
	return "sh -c 'test -S " + path + " && rm -f " + path + "'"
}

func forwardArgs(socketPath string, forward workspace.Forward) []string {
	return controlArgs(socketPath, "forward", forward.Host, forward.SSHArgs()...)
}

func cancelArgs(socketPath string, forward workspace.Forward) []string {
	return controlArgs(socketPath, "cancel", forward.Host, forward.SSHArgs()...)
}

// loginMarkers are what ssh logs in batch mode when an interactive login
// would get further: an unknown or changed host key, or authentication that
// needs a password, passphrase, or 2FA answer. ssh logs the first through a
// ProxyJump hop too, ahead of the line that ends the connection.
var loginMarkers = []string{
	"Host key verification failed",
	"Permission denied",
}

// needsLogin reports whether a failed batch connect's log asks for an
// interactive login rather than reporting a network or config error.
func needsLogin(log string) bool {
	for _, marker := range loginMarkers {
		if strings.Contains(log, marker) {
			return true
		}
	}
	return false
}

var masterPIDPattern = regexp.MustCompile(`Master running \(pid=(\d+)\)`)

// masterPID reads the master's process ID from `ssh -O check` output.
func masterPID(checkOutput string) (int, bool) {
	match := masterPIDPattern.FindStringSubmatch(checkOutput)
	if match == nil {
		return 0, false
	}
	pid, err := strconv.Atoi(match[1])
	return pid, err == nil
}

// controlFailure is the reason in a failed control command's output, such as
// "Port forwarding failed" from "mux_client_forward: forwarding request
// failed: Port forwarding failed".
func controlFailure(output string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	if _, reason, ok := strings.Cut(first, "request failed: "); ok {
		return strings.TrimSpace(reason)
	}
	return strings.TrimSpace(first)
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
