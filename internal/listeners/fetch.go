package listeners

import (
	"context"
	"fmt"
	"time"
)

const (
	fetchTimeout = 20 * time.Second
	// listCommand runs on the remote host; one ssh round trip per fetch. It
	// runs under sh because the login shell may be fish, zsh, or bash; the
	// script holds no single quotes or backslashes, which fish would read
	// differently inside single quotes. Linux has ss. macOS has no ss: there,
	// lsof lists the ssh user's own listeners with process names (without root
	// it sees no other users' processes), and netstat lists every listening
	// port without names. lsof exits 1 when it finds nothing, so netstat's
	// status decides the exit code.
	listCommand = "sh -c '" +
		"if command -v ss >/dev/null 2>&1; then echo " + sectionSS + "; exec ss -ltnp; fi; " +
		"echo " + sectionLsof + "; lsof +c 0 -nP -iTCP -sTCP:LISTEN -Fcn; " +
		"echo " + sectionNetstat + "; exec netstat -anp tcp'"
)

// Runner runs command on host over ssh and returns its stdout.
type Runner func(ctx context.Context, host, command string) (string, error)

// Fetch lists the ports listening on host with one remote command.
func Fetch(run Runner, host string) ([]Listener, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	stdout, err := run(ctx, host, listCommand)
	if err != nil {
		return nil, fmt.Errorf("listing ports on %s failed: %w", host, err)
	}
	return Parse(stdout), nil
}
