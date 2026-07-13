package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/SarthakShrivastav-a/hoppr/internal/doctor"
	"github.com/SarthakShrivastav-a/hoppr/internal/keys"
)

func newDoctorCmd() *cobra.Command {
	var fix bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose what is about to break",
		Long: `Check the SSH setup for the problems that produce useless error messages.

  Permission denied (publickey)     a host names a key that is not there
  Too many authentication failures  a host names a key but not IdentitiesOnly,
                                    so ssh offers every agent key first and the
                                    server hangs up before reaching the right one
  Bad configuration option          a CRLF or a byte-order mark in ssh_config
  UNPROTECTED PRIVATE KEY FILE      a key readable by more than just you

Pass --fix to repair everything that can be repaired mechanically.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			_, hosts, err := load()
			if err != nil {
				return err
			}

			ks := keys.Scan(keys.DefaultRoots())
			report := doctor.Run(path, hosts, ks)

			problems := report.Problems()
			if len(problems) == 0 {
				fmt.Fprintln(out, "✓ nothing wrong")
				return nil
			}

			for _, f := range problems {
				badge := "warn"
				if f.Severity == doctor.Fail {
					badge = "FAIL"
				}
				fmt.Fprintf(out, "%s  %s\n", badge, f.Title)
				fmt.Fprintf(out, "      %s\n", f.Detail)
				if f.Fixable() && !fix {
					fmt.Fprintf(out, "      fixable: %s\n", f.FixLabel)
				}
				fmt.Fprintln(out)
			}

			if !fix {
				if n := len(report.Fixable()); n > 0 {
					fmt.Fprintf(out, "%d of these can be fixed automatically: hoppr doctor --fix\n", n)
				}
				// A non-zero exit so this is usable as a check in CI or a
				// pre-flight script.
				return fmt.Errorf("%d problem(s) found", len(problems))
			}

			if err := guard(); err != nil {
				return err
			}

			fixed := 0
			for _, f := range report.Fixable() {
				if err := f.Fix(); err != nil {
					fmt.Fprintf(out, "could not fix %q: %v\n", f.Title, err)
					continue
				}
				fmt.Fprintf(out, "fixed: %s\n", f.Title)
				fixed++
			}

			remaining := len(problems) - fixed
			if remaining > 0 {
				fmt.Fprintf(out, "\n%d problem(s) need a human.\n", remaining)
				return fmt.Errorf("%d problem(s) remain", remaining)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&fix, "fix", false, "repair everything that can be repaired automatically")
	return cmd
}
