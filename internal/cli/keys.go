package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SarthakShrivastav-a/hoppr/internal/keys"
)

func newKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Inventory, fix, and adopt SSH private keys",
		Long: `Find every private key on this machine and say what it is used for.

Keys arrive from a cloud console as a .pem in Downloads and stay there. This
command finds them, shows which hosts reference each one, flags the ones
nothing uses, and reports whether their permissions are actually safe — which
on Windows means the NTFS ACL, not the mode bits that chmod pretends to set.`,
	}

	cmd.AddCommand(
		newKeysListCmd(),
		newKeysFixCmd(),
		newKeysAdoptCmd(),
	)
	return cmd
}

type keyJSON struct {
	Path        string   `json:"path"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Fingerprint string   `json:"fingerprint"`
	Encrypted   bool     `json:"encrypted"`
	Stray       bool     `json:"stray"`
	PermOK      bool     `json:"permissions_ok"`
	PermDetail  string   `json:"permissions_detail"`
	UsedBy      []string `json:"used_by"`
}

func newKeysListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list", "scan"},
		Short:   "List every private key and what uses it",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, hosts, err := load()
			if err != nil {
				return err
			}

			ks := keys.Scan(keys.DefaultRoots())
			usage := keys.Index(ks, hosts)

			if flagJSON {
				list := make([]keyJSON, 0, len(usage))
				for _, u := range usage {
					var used []string
					for _, h := range u.Hosts {
						used = append(used, h.Alias)
					}
					list = append(list, keyJSON{
						Path:        u.Key.Path,
						Name:        u.Key.Name(),
						Type:        u.Key.Type,
						Fingerprint: u.Key.Fingerprint,
						Encrypted:   u.Key.Encrypted,
						Stray:       u.Key.Stray(),
						PermOK:      u.Key.Perm.OK,
						PermDetail:  u.Key.Perm.Detail,
						UsedBy:      used,
					})
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(list)
			}

			if len(usage) == 0 {
				fmt.Fprintln(out, "no private keys found")
				return nil
			}

			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "KEY\tTYPE\tPERMS\tUSED BY\tPATH")

			for _, u := range usage {
				k := u.Key

				perm := "ok"
				if !k.Perm.OK {
					perm = "TOO OPEN"
				}

				used := "—"
				if !u.Orphaned() {
					var aliases []string
					for _, h := range u.Hosts {
						aliases = append(aliases, h.Alias)
					}
					used = strings.Join(aliases, ",")
				}

				name := k.Name()
				if k.Stray() {
					name += " *"
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, k.Describe(), perm, used, k.Path)
			}
			if err := w.Flush(); err != nil {
				return err
			}

			fmt.Fprintln(out, "\n* lives outside ~/.ssh — `hoppr keys adopt` moves it and repoints the hosts that use it")
			return nil
		},
	}

	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit JSON")
	return cmd
}

func newKeysFixCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "fix [key]",
		Short: "Lock a key's permissions down so ssh will accept it",
		Long: `Repair the permissions on a private key.

On Unix this sets mode 0600. On Windows it strips inherited ACL entries and
grants the file to your account alone, which is the only thing that actually
resolves "WARNING: UNPROTECTED PRIVATE KEY FILE" there — chmod does nothing
on NTFS.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ks := keys.Scan(keys.DefaultRoots())

			var targets []*keys.Key
			switch {
			case all:
				for _, k := range ks {
					if !k.Perm.OK {
						targets = append(targets, k)
					}
				}
			case len(args) == 1:
				k, err := findKey(ks, args[0])
				if err != nil {
					return err
				}
				targets = append(targets, k)
			default:
				return fmt.Errorf("name a key, or pass --all to fix every key with open permissions")
			}

			if len(targets) == 0 {
				fmt.Fprintln(out, "nothing to fix — every key is already locked down")
				return nil
			}

			for _, k := range targets {
				if err := keys.Fix(k.Path); err != nil {
					return fmt.Errorf("%s: %w", k.Name(), err)
				}
				fmt.Fprintf(out, "locked %s\n", k.Path)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "fix every key with open permissions")
	return cmd
}

func newKeysAdoptCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "adopt [key]",
		Short: "Move a stray key into ~/.ssh/keys and repoint the hosts that use it",
		Long: `Move a private key into ~/.ssh/keys, lock its permissions, and rewrite every
Host stanza that referenced its old location.

The rewrite is the point. Moving a key without it silently breaks the hosts
that used it, which is worse than leaving the key in Downloads.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := guard(); err != nil {
				return err
			}

			tree, hosts, err := load()
			if err != nil {
				return err
			}
			ks := keys.Scan(keys.DefaultRoots())

			var targets []*keys.Key
			switch {
			case all:
				for _, k := range ks {
					if k.Stray() {
						targets = append(targets, k)
					}
				}
			case len(args) == 1:
				k, err := findKey(ks, args[0])
				if err != nil {
					return err
				}
				targets = append(targets, k)
			default:
				return fmt.Errorf("name a key, or pass --all to adopt every stray key")
			}

			if len(targets) == 0 {
				fmt.Fprintln(out, "no stray keys — everything already lives in ~/.ssh")
				return nil
			}

			for _, k := range targets {
				res, err := keys.Adopt(k, hosts)
				if err != nil {
					return fmt.Errorf("%s: %w", k.Name(), err)
				}

				fmt.Fprintf(out, "moved %s\n     → %s\n", res.From, res.To)
				if len(res.Rewritten) > 0 {
					fmt.Fprintf(out, "     repointed: %s\n", strings.Join(res.Rewritten, ", "))
				}
			}

			return tree.Save()
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "adopt every key found outside ~/.ssh")
	return cmd
}

// findKey resolves a key by file name or by path, so both
// `hoppr keys fix prod.pem` and `hoppr keys fix ~/Downloads/prod.pem` work.
func findKey(ks []*keys.Key, name string) (*keys.Key, error) {
	var matches []*keys.Key
	for _, k := range ks {
		if strings.EqualFold(k.Name(), name) || strings.EqualFold(k.Path, name) {
			matches = append(matches, k)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no key named %q — run `hoppr keys ls` to see what was found", name)
	case 1:
		return matches[0], nil
	default:
		// Two keys with the same file name in different directories is exactly
		// the mess this tool exists to clean up, so say so rather than guessing.
		var paths []string
		for _, k := range matches {
			paths = append(paths, k.Path)
		}
		return nil, fmt.Errorf("%q is ambiguous — name a full path:\n  %s", name, strings.Join(paths, "\n  "))
	}
}
