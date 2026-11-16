// lazyfwd manages ssh port forwards from a keyboard-first TUI or the command line.
package main

import (
	"os"

	"github.com/glattziehen/lazyfwd/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], version()))
}
