// Command skills installs agent skills into every directory the agents on this
// machine read.
package main

import (
	"fmt"
	"os"

	"github.com/aniaan/skills/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		// Blank lines around it: an error listing candidate skills runs several
		// lines and gets lost against the command that produced it.
		fmt.Fprintf(os.Stderr, "\nerror: %v\n\n", err)
		os.Exit(1)
	}
}
