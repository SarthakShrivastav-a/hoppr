package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/SarthakShrivastav-a/hoppr/internal/connect"
	"github.com/SarthakShrivastav-a/hoppr/internal/forward"
	"github.com/SarthakShrivastav-a/hoppr/internal/host"
)

func newForwardCmd() *cobra.Command {
	var (
		local   string
		remote  string
		dynamic string
		save    bool
	)

	cmd := &cobra.Command{
		Use:   "forward <alias>",
		Short: "Open a port forward and keep it open",
		Long: `Run an SSH port forward, supervised.

The tunnel is restarted when it drops, with exponential backoff and jitter,
so a closed laptop lid or a flaky network does not silently leave you with a
dead port that still looks open.

  hoppr forward prod -L 5432:localhost:5432    reach the remote database locally
  hoppr forward prod -D 1080                   SOCKS proxy through prod
  hoppr forward prod -R 9000:localhost:3000    expose a local service remotely

Pass --save to store the forward on the host in ssh_config, so a plain
"ssh prod" brings it up too.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeHost,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			tree, hosts, err := load()
			if err != nil {
				return err
			}

			h, found := host.Find(hosts, args[0])
			if !found {
				return fmt.Errorf("no host named %q", args[0])
			}

			spec, err := specFromFlags(local, remote, dynamic)
			if err != nil {
				return err
			}

			if save {
				if err := guard(); err != nil {
					return err
				}
				keyword, kwArgs := spec.ConfigKeyword()
				h.Block().SetArgs(keyword, kwArgs...)
				if err := tree.Save(); err != nil {
					return err
				}
				fmt.Fprintf(out, "saved to %s: %s %s\n", h.File, keyword, spec.Arg())
			}

			t := forward.New(h, spec, connect.Options{ConfigPath: path})

			// Ctrl+C should take the tunnel down cleanly rather than orphaning
			// an ssh process holding a port open.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			fmt.Fprintf(out, "%s via %s — ctrl+c to stop\n", spec.String(), h.Alias)
			t.Start(ctx, out)

			fmt.Fprintln(out, "stopped")
			return nil
		},
	}

	cmd.Flags().StringVarP(&local, "local", "L", "", "local forward, bindPort:destHost:destPort")
	cmd.Flags().StringVarP(&remote, "remote", "R", "", "remote forward, bindPort:destHost:destPort")
	cmd.Flags().StringVarP(&dynamic, "dynamic", "D", "", "dynamic SOCKS forward, port")
	cmd.Flags().BoolVar(&save, "save", false, "also store the forward on the host in ssh_config")

	return cmd
}

// specFromFlags turns the three mutually exclusive flags into one spec.
func specFromFlags(local, remote, dynamic string) (forward.Spec, error) {
	set := 0
	for _, s := range []string{local, remote, dynamic} {
		if s != "" {
			set++
		}
	}

	switch {
	case set == 0:
		return forward.Spec{}, fmt.Errorf("pass one of -L, -R, or -D")
	case set > 1:
		return forward.Spec{}, fmt.Errorf("pass only one of -L, -R, or -D")
	}

	switch {
	case local != "":
		return forward.ParseSpec(forward.Local, local)
	case remote != "":
		return forward.ParseSpec(forward.Remote, remote)
	default:
		return forward.ParseSpec(forward.Dynamic, dynamic)
	}
}
