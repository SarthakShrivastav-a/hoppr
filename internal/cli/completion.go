package cli

import (
	"github.com/spf13/cobra"

	"github.com/SarthakShrivastav-a/hoppr/internal/host"
)

// completeHost completes an argument that names a host.
//
// This is the payoff of keeping ssh_config as the source of truth: the shell can
// complete host aliases because there is one obvious place to read them from,
// and it stays correct even when the user edits the config by hand.
func completeHost(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Already have the one argument we take.
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	_, hosts, err := load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Offer the target as the completion description, so `ssh pr<tab>` shows
	// what each candidate actually connects to rather than a bare list of names.
	var out []string
	for _, m := range host.Filter(hosts, toComplete, nil) {
		h := m.Host
		out = append(out, h.Alias+"\t"+h.Target())
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
