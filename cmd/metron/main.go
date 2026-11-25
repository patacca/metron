// Command metron governs engineering review rounds. See docs/architecture.md.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/patacca/metron/internal/cli"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "metron: cannot determine the working directory: %v\n", err)
		os.Exit(3)
	}
	os.Exit(cli.Run(os.Args[1:], cli.Env{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Dir:    dir,
		Getenv: os.Getenv,
		Now:    time.Now,
	}))
}
