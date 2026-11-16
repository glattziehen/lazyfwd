package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"github.com/moritzwilksch/lazyfwd/internal/tunnel"
)

// loginCommand frames an interactive ssh login on the released terminal: a
// line saying why ssh asks, and on failure ssh's log, which otherwise goes to
// a file, with a pause so the user can read it.
type loginCommand struct {
	*tunnel.Login
	stdin  io.Reader
	stdout io.Writer
}

func (c *loginCommand) SetStdin(r io.Reader) {
	c.stdin = r
	c.Login.SetStdin(r)
}

func (c *loginCommand) SetStdout(w io.Writer) {
	c.stdout = w
	c.Login.SetStdout(w)
}

func (c *loginCommand) Run() error {
	_, _ = fmt.Fprintf(c.stdout, "lazyfwd: %s needs a login. lazyfwd returns once ssh connects; ctrl+c cancels.\n\n", c.Host)
	err := c.Login.Run()
	var connectErr *tunnel.ConnectError
	if !errors.As(err, &connectErr) {
		return err
	}
	_, _ = fmt.Fprintf(c.stdout, "\n%s\nlazyfwd: login to %s failed. Press enter to return.", connectErr.Log, c.Host)
	_, _ = bufio.NewReader(c.stdin).ReadString('\n')
	return err
}
