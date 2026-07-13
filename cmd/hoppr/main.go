// Command hoppr manages SSH hosts and keys from the terminal.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/SarthakShrivastav-a/hoppr/internal/cli"
	"github.com/SarthakShrivastav-a/hoppr/internal/version"
)

// Version is injected at release time via -ldflags.
var Version string

func main() {
	if Version != "" {
		version.Version = Version
	}

	if err := cli.Execute(); err != nil {
		// When we shelled out to ssh, ssh's exit code is the meaningful one and
		// it has already explained itself on stderr. Pass the code through
		// untouched and stay quiet, so `hoppr connect web && deploy` behaves
		// the way the shell expects.
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}

		fmt.Fprintln(os.Stderr, "hoppr: "+err.Error())
		os.Exit(1)
	}
}
