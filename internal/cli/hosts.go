package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SarthakShrivastav-a/hoppr/internal/connect"
	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

// hostJSON is the machine-readable shape of a host. It is a separate type from
// host.Host on purpose: this is a published contract, and it should not change
// just because an internal field was renamed.
type hostJSON struct {
	Alias         string   `json:"alias"`
	Aliases       []string `json:"aliases,omitempty"`
	HostName      string   `json:"hostname"`
	User          string   `json:"user,omitempty"`
	Port          int      `json:"port"`
	IdentityFiles []string `json:"identity_files,omitempty"`
	ProxyJump     string   `json:"proxy_jump,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Note          string   `json:"note,omitempty"`
	ConfigFile    string   `json:"config_file"`
}

func toJSON(h *host.Host) hostJSON {
	return hostJSON{
		Alias:         h.Alias,
		Aliases:       h.Aliases,
		HostName:      h.HostName,
		User:          h.User,
		Port:          h.EffectivePort(),
		IdentityFiles: h.IdentityFiles,
		ProxyJump:     h.ProxyJump,
		Tags:          h.Tags(),
		Note:          h.Meta.Note,
		ConfigFile:    h.File,
	}
}

func newListCmd() *cobra.Command {
	var tag string

	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List hosts",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, hosts, err := load()
			if err != nil {
				return err
			}

			query := ""
			if len(args) == 1 {
				query = args[0]
			}
			var tags []string
			if tag != "" {
				tags = strings.Split(tag, ",")
			}

			matches := host.Filter(hosts, query, tags)

			if flagJSON {
				list := make([]hostJSON, 0, len(matches))
				for _, m := range matches {
					list = append(list, toJSON(m.Host))
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(list)
			}

			if len(matches) == 0 {
				fmt.Fprintln(out, "no hosts")
				return nil
			}

			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ALIAS\tTARGET\tPORT\tTAGS")
			for _, m := range matches {
				h := m.Host
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
					h.Alias, h.Target(), h.EffectivePort(), strings.Join(h.Tags(), ","))
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVarP(&tag, "tag", "t", "", "only hosts with these tags (comma separated)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit JSON")
	return cmd
}

func newAddCmd() *cobra.Command {
	var (
		hostname string
		user     string
		port     int
		identity string
		jump     string
		tags     string
		note     string
	)

	cmd := &cobra.Command{
		Use:   "add <alias>",
		Short: "Add a host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := guard(); err != nil {
				return err
			}
			alias := args[0]

			if host.IsPattern(alias) {
				return fmt.Errorf("%q contains a glob or negation, which makes it a pattern rather than a host", alias)
			}
			if host.IsUnsafeAlias(alias) {
				return fmt.Errorf("%q starts with '-', which ssh would read as a flag rather than a host", alias)
			}
			if hostname == "" {
				return fmt.Errorf("--host is required")
			}

			tree, _, err := load()
			if err != nil {
				return err
			}
			if _, found := tree.FindHost(alias); found {
				return fmt.Errorf("host %q already exists", alias)
			}

			f := tree.Root
			b := f.AddHost(alias)
			b.Set("HostName", hostname)

			if user != "" {
				b.Set("User", user)
			}
			if port != 0 && port != 22 {
				b.Set("Port", strconv.Itoa(port))
			}
			if jump != "" {
				b.Set("ProxyJump", jump)
			}
			if identity != "" {
				b.Set("IdentityFile", connect.ResolveKeyPath(identity))
				b.Set("IdentitiesOnly", "yes")
			}

			b.SetMeta(sshconfig.Meta{Tags: splitList(tags), Note: note})

			if err := f.Save(); err != nil {
				return err
			}
			fmt.Fprintf(out, "added %s → %s\n", alias, hostname)
			return nil
		},
	}

	cmd.Flags().StringVar(&hostname, "host", "", "hostname or IP (required)")
	cmd.Flags().StringVarP(&user, "user", "u", "", "login user")
	cmd.Flags().IntVarP(&port, "port", "p", 0, "port (default 22)")
	cmd.Flags().StringVarP(&identity, "key", "i", "", "path to a private key")
	cmd.Flags().StringVarP(&jump, "jump", "J", "", "bastion to jump through")
	cmd.Flags().StringVarP(&tags, "tag", "t", "", "tags (comma separated)")
	cmd.Flags().StringVar(&note, "note", "", "free-text note")

	return cmd
}

func newImportCmd() *cobra.Command {
	var alias string

	cmd := &cobra.Command{
		Use:   `import "<ssh command>"`,
		Short: "Turn an ssh command you already use into a host entry",
		Long: `Parse an ad-hoc ssh command and store it as a Host stanza.

This is the fastest way to stop copy-pasting connection commands. Paste the
line exactly as you have been using it, including any leading "cd" — that
prefix is ignored:

  hoppr import "cd ~/Downloads && ssh -i dev.pem ec2-user@dev-api.example.com"

Flags such as -i, -p, -J, -L, -R, -D, and -o are all carried across.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := guard(); err != nil {
				return err
			}

			spec, err := connect.ParseCommand(args[0])
			if err != nil {
				return err
			}
			if alias != "" {
				spec.Alias = alias
			}

			tree, _, err := load()
			if err != nil {
				return err
			}

			if _, err := spec.Apply(tree.Root); err != nil {
				return err
			}
			if err := tree.Root.Save(); err != nil {
				return err
			}

			fmt.Fprintf(out, "added %s → %s\n", spec.Alias, spec.HostName)
			fmt.Fprintf(out, "connect with: ssh %s\n", spec.Alias)
			return nil
		},
	}

	cmd.Flags().StringVar(&alias, "as", "", "alias to store it under (default: inferred from the hostname)")
	return cmd
}

func newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "rm <alias>",
		Aliases:           []string{"remove"},
		Short:             "Remove a host",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeHost,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := guard(); err != nil {
				return err
			}
			alias := args[0]

			tree, _, err := load()
			if err != nil {
				return err
			}

			tb, found := tree.FindHost(alias)
			if !found {
				return fmt.Errorf("no host named %q", alias)
			}

			// Remove from the file that declares it, which is not necessarily
			// the root config.
			if !tb.File.RemoveHost(alias) {
				return fmt.Errorf("no host named %q", alias)
			}
			if err := tb.File.Save(); err != nil {
				return err
			}

			fmt.Fprintf(out, "removed %s from %s\n", alias, tb.File.Path)
			return nil
		},
	}
}

func newConnectCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:               "connect <alias>",
		Short:             "Connect to a host",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeHost,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			_, hosts, err := load()
			if err != nil {
				return err
			}

			h, found := host.Find(hosts, args[0])
			if !found {
				return fmt.Errorf("no host named %q", args[0])
			}

			opts := connect.Options{ConfigPath: path}

			// Check before previewing too: printing a command we would refuse to
			// run, as though it were fine, is its own small lie.
			if err := connect.Check(h); err != nil {
				return err
			}
			if dryRun {
				fmt.Fprintln(out, connect.Preview(h, opts))
				return nil
			}

			// Replace this process's stdio with ssh's and wait. The exit code
			// is ssh's, so `hoppr connect x && something` behaves the way a
			// script expects.
			sshCmd, err := connect.Command(h, opts)
			if err != nil {
				return err
			}
			return sshCmd.Run()
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the ssh command instead of running it")
	return cmd
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
