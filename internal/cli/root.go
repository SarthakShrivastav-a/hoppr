// Package cli is hoppr's command-line surface.
//
// Every action the TUI offers is also a command, and every command that produces
// a list can produce JSON. That is not padding: people drive SSH config from
// Ansible and from shell scripts, and a manager that only works when a human is
// watching cannot be part of that.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/paths"
	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
	"github.com/SarthakShrivastav-a/hoppr/internal/tui"
	"github.com/SarthakShrivastav-a/hoppr/internal/version"
)

var (
	flagConfig   string
	flagReadOnly bool
	flagJSON     bool
)

// Execute runs the root command.
func Execute() error {
	return newRoot().Execute()
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "hoppr",
		Short: "An SSH session manager for the terminal",
		Long: `hoppr manages the hosts in your ssh_config and the keys they use.

It edits your real ~/.ssh/config in place, preserving comments, formatting,
and directives it does not understand. It never keeps a second copy of your
hosts, and it never invents its own connection format: connecting shells out
to your system ssh, so your agent, jump hosts, and Match blocks work exactly
as they already do.

Run with no arguments to open the interactive list.`,
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,

		// Bare `hoppr` opens the TUI. Anything else is a subcommand.
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			return tui.Run(path, flagReadOnly)
		},
	}

	root.PersistentFlags().StringVarP(&flagConfig, "config", "c", "",
		"path to ssh_config (default ~/.ssh/config, or $HOPPR_CONFIG)")
	root.PersistentFlags().BoolVar(&flagReadOnly, "read-only", false,
		"never modify the config, whatever else is asked")

	root.AddCommand(
		newListCmd(),
		newAddCmd(),
		newImportCmd(),
		newRemoveCmd(),
		newConnectCmd(),
		newKeysCmd(),
		newDoctorCmd(),
	)
	return root
}

// configPath resolves which ssh_config to operate on.
func configPath() (string, error) {
	if flagConfig != "" {
		return paths.Expand(flagConfig), nil
	}
	return paths.Config()
}

// load reads the config tree and the hosts in it.
func load() (*sshconfig.Tree, []*host.Host, error) {
	path, err := configPath()
	if err != nil {
		return nil, nil, err
	}
	tree, err := sshconfig.LoadTree(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	return tree, host.Load(tree), nil
}

// errReadOnly is returned by every mutating command when --read-only is set.
var errReadOnly = fmt.Errorf("--read-only is set: refusing to modify the config")

// guard blocks a mutation in read-only mode.
func guard() error {
	if flagReadOnly {
		return errReadOnly
	}
	return nil
}

// out is where normal output goes. Tests swap it.
var out = os.Stdout
