package tunnel

import "github.com/glattziehen/lazyfwd/internal/workspace"

// On Windows every ssh call is its own connection: the ssh that Windows
// ships has no ControlMaster and no -f. These options apply to all of them.
// A ControlMaster or ControlPath from ~/.ssh/config would make that ssh fail
// with "getsockname failed: Not a socket", so the command line turns them off.
var windowsOptions = []string{
	"-o", "ControlMaster=no",
	"-o", "ControlPath=none",
	"-o", "ConnectTimeout=10",
}

// forwardProcessArgs run one forward in the foreground without prompts. -v
// makes ssh log the server's confirmation of a remote forward, which tells
// lazyfwd that the forward runs. They keep the config's forwards: a forward's
// own -L or -R would go with ClearAllForwardings, so lazyfwd refuses hosts
// whose config forwards ports instead.
func forwardProcessArgs(forward workspace.Forward) []string {
	args := append([]string{"-N", "-v"}, windowsOptions...)
	args = append(args,
		"-o", "BatchMode=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
	)
	args = append(args, forward.SSHArgs()...)
	return append(args, "--", forward.Host)
}

// commandArgs run command on host without prompts.
func commandArgs(host, command string) []string {
	args := append([]string{"-T"}, windowsOptions...)
	args = append(args, "-o", clearConfigForwards, "-o", "BatchMode=yes")
	return append(args, "--", host, command)
}

// windowsProbeArgs connect stdin and stdout to target on host, as probeArgs
// do through a master; -v logs when ssh has authenticated.
func windowsProbeArgs(host, target string) []string {
	args := append([]string{"-v"}, windowsOptions...)
	args = append(args, "-o", clearConfigForwards, "-o", "BatchMode=yes", "-W", target)
	return append(args, "--", host)
}

// configArgs print host's ssh config as ssh resolves it.
func configArgs(host string) []string {
	return []string{"-G", "--", host}
}

// hostKeyLoginArgs connect on the terminal, so the user can accept a host
// key, and exit once authenticated. They refuse passwords and 2FA: each
// forward is a new connection without prompts, so a password typed here
// would not let one start.
func hostKeyLoginArgs(host string) []string {
	args := append([]string{"-T"}, windowsOptions...)
	args = append(args,
		"-o", clearConfigForwards,
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
	)
	return append(args, "--", host, "exit")
}
