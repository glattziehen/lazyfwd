package listeners

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const fetchTimeout = 20 * time.Second

// listCommand runs on the remote host; one ssh round trip per fetch. It runs
// under sh because the login shell may be fish, zsh, bash, or csh. The script
// is one line without single quotes, backslashes, or "!", which those shells
// would read differently inside single quotes. Each branch prints a section
// marker before its tool's output, and Parse reads the sections:
//   - Linux: ss; else netstat (net-tools or busybox); else /proc/net/tcp*,
//     which has no process names.
//   - FreeBSD, NetBSD, DragonFly: sockstat, which names every user's
//     processes, so the user name is printed too.
//   - macOS, OpenBSD: lsof names the ssh user's own listeners (without root it
//     sees no others), and netstat lists every listening port without names.
//     lsof exits 1 when it finds nothing, so netstat's status decides.
//   - Anything else: the OS name, which Parse reports as unsupported.
var listCommand = "sh -c '" + strings.Join([]string{
	"case $(uname -s) in",
	"Linux)",
	"if command -v ss >/dev/null 2>&1; then echo " + sectionSS + "; exec ss -ltnp; fi;",
	"if command -v netstat >/dev/null 2>&1; then echo " + sectionNetstatLinux + "; exec netstat -ltnp 2>/dev/null; fi;",
	"echo " + sectionProc + "; exec cat /proc/net/tcp /proc/net/tcp6 2>/dev/null;;",
	"Darwin|*BSD|DragonFly)",
	"if command -v sockstat >/dev/null 2>&1; then echo " + sectionUser + "; id -un; echo " + sectionSockstat + "; exec sockstat -46l; fi;",
	"echo " + sectionLsof + "; lsof +c 0 -nP -iTCP -sTCP:LISTEN -Fcn 2>/dev/null;",
	"echo " + sectionNetstatBSD + "; exec netstat -an -p tcp;;",
	"*) echo " + sectionUnsupported + "; uname -s;;",
	"esac",
}, " ") + "'"

// Runner runs command on host over ssh. It returns the command's stdout even
// when it fails, and an *exec.ExitError when the command ran and exited
// non-zero; any other error means ssh could not run it.
type Runner func(ctx context.Context, host, command string) (string, error)

// Fetch lists the ports listening on host with one remote command.
func Fetch(run Runner, host string) (Listing, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	stdout, runErr := run(ctx, host, listCommand)
	if ctx.Err() != nil {
		return Listing{}, fmt.Errorf("listing ports on %s timed out after %s", host, fetchTimeout)
	}
	// The script's output decides, whatever its exit status: a missing
	// /proc/net/tcp6, for one, fails cat after the ports are printed.
	found, parseErr := Parse(stdout)
	if parseErr == nil {
		return found, nil
	}
	// A command that ran but printed no script output means the host has no
	// sh to run it; any other failure is ssh's.
	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		return Listing{}, fmt.Errorf("listing ports on %s failed: %w", host, runErr)
	}
	return Listing{}, fmt.Errorf("cannot list ports on %s: %w", host, parseErr)
}
