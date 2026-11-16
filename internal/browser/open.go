// Package browser opens URLs in the user's default browser.
package browser

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// Open hands url to the platform's opener and returns once it has started.
// It fails with a message for the user when this machine has no browser to
// open, such as a server reached over ssh.
func Open(url string) error {
	command, err := opener()
	if err != nil {
		return fmt.Errorf("%w; open %s yourself", err, url)
	}
	cmd := exec.Command(command[0], append(command[1:], url)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// opener is the command that opens a URL appended to it.
func opener() ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		return []string{"open"}, nil
	case "windows":
		// url.dll hands the URL to the default browser. cmd's "start",
		// the usual alternative, would read & in a URL as a command separator.
		return []string{"rundll32", "url.dll,FileProtocolHandler"}, nil
	}
	// wslview, from wslu, opens the Windows browser from WSL, where xdg-open
	// is usually missing.
	if path, err := exec.LookPath("wslview"); err == nil {
		return []string{path}, nil
	}
	// Without a graphical session xdg-open falls back to a text browser such
	// as lynx, which would take over lazyfwd's terminal.
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return nil, errors.New("no graphical session here (DISPLAY and WAYLAND_DISPLAY are unset)")
	}
	path, err := exec.LookPath("xdg-open")
	if err != nil {
		return nil, errors.New("xdg-open not found (install xdg-utils)")
	}
	return []string{path}, nil
}
