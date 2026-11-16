// lazyfwd manages ssh port forwards from a keyboard-first TUI or the command line.
package main

import (
	"os"
	"runtime/debug"

	"github.com/glattziehen/lazyfwd/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], version()))
}

// version is the main module version the go command stamps at build time:
// the tag for a release or `go install ...@vX`, a pseudo-version from git for
// a local build, and "(devel)" for a build outside a git checkout.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "(devel)"
	}
	return info.Main.Version
}
