// Package host turns ssh_config Host stanzas into the record hoppr shows,
// searches, and connects to.
package host

import (
	"strconv"
	"strings"

	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

// Host is one connectable entry from the user's ssh config.
type Host struct {
	// Alias is the name you type after `ssh`. It is the first non-pattern
	// entry on the Host line.
	Alias string

	// Aliases holds every name the stanza declares, including Alias itself.
	Aliases []string

	HostName      string
	User          string
	Port          int
	IdentityFiles []string
	ProxyJump     string

	// Meta is hoppr's own annotation: tags, note, color.
	Meta sshconfig.Meta

	// File is the config file that declares this host. Edits go back here,
	// which is what keeps a host defined in conf.d/work.conf from being
	// rewritten into ~/.ssh/config.
	File string

	block *sshconfig.Block
	file  *sshconfig.File
}

// Block exposes the underlying stanza so callers can edit it.
func (h *Host) Block() *sshconfig.Block { return h.block }

// ConfigFile exposes the file the host lives in, so a caller that edits the
// block knows which file to save.
func (h *Host) ConfigFile() *sshconfig.File { return h.file }

// Port default when the config does not say otherwise.
const DefaultPort = 22

// EffectivePort returns the port to display, substituting OpenSSH's default.
func (h *Host) EffectivePort() int {
	if h.Port == 0 {
		return DefaultPort
	}
	return h.Port
}

// Target renders the user@hostname the connection will actually go to. It is
// what the list shows under the alias.
func (h *Host) Target() string {
	name := h.HostName
	if name == "" {
		// A stanza with no HostName connects to the alias itself.
		name = h.Alias
	}
	if h.User != "" {
		return h.User + "@" + name
	}
	return name
}

// Addr returns host:port for dialing, which the reachability probe wants.
func (h *Host) Addr() string {
	name := h.HostName
	if name == "" {
		name = h.Alias
	}
	return name + ":" + strconv.Itoa(h.EffectivePort())
}

// Tags is a convenience passthrough to the annotation.
func (h *Host) Tags() []string { return h.Meta.Tags }

// IsPattern reports whether s is a glob or negation pattern rather than a
// literal host name.
//
// A stanza like `Host web-* !web-99` declares defaults for a family of hosts.
// Neither `web-*` nor `!web-99` is something you can connect to, and listing
// them as if they were is a bug other managers have shipped: users end up
// staring at a host called "!web-99" in the picker.
func IsPattern(s string) bool {
	return strings.ContainsAny(s, "*?") || strings.HasPrefix(s, "!")
}

// IsUnsafeAlias reports whether s would be read by ssh as an option rather than
// as a destination.
//
// Connecting runs `ssh <alias>`, so an alias of "-oProxyCommand=calc" is not a
// host at all — it is a flag, and ssh will execute it. Typing that yourself is
// your own business, but hoppr renders the config's hosts as a list you pick
// from, which turns a hostile stanza in someone else's Include'd conf.d file
// into a one-keypress command execution that looks like a normal server.
//
// Plain ssh has the same behaviour; the difference is that hoppr presents the
// thing as safe to select. So refuse it: never list it, never connect to it,
// never let one be created.
func IsUnsafeAlias(s string) bool {
	return strings.HasPrefix(s, "-")
}

// Connectable reports whether s is an alias hoppr will actually offer.
func Connectable(s string) bool {
	return s != "" && !IsPattern(s) && !IsUnsafeAlias(s)
}

// FromBlock builds a Host from a stanza, or reports false if the stanza
// declares no connectable alias at all.
func FromBlock(f *sshconfig.File, b *sshconfig.Block) (*Host, bool) {
	patterns := b.Patterns()

	var literals []string
	for _, p := range patterns {
		// Connectable drops both glob/negation patterns (which configure other
		// hosts rather than being one) and option-shaped aliases (which ssh
		// would execute as flags).
		if Connectable(p) {
			literals = append(literals, p)
		}
	}
	if len(literals) == 0 {
		// A pure pattern stanza — `Host *` and friends. It configures other
		// hosts; it is not a host.
		return nil, false
	}

	h := &Host{
		Alias:         literals[0],
		Aliases:       literals,
		HostName:      get(b, "HostName"),
		User:          get(b, "User"),
		ProxyJump:     get(b, "ProxyJump"),
		IdentityFiles: b.GetAll("IdentityFile"),
		Meta:          b.Meta(),
		File:          f.Path,
		block:         b,
		file:          f,
	}
	if p := get(b, "Port"); p != "" {
		// A non-numeric Port is a broken config, not a crash. Fall back to the
		// default and let `hoppr doctor` be the one to complain about it.
		if n, err := strconv.Atoi(p); err == nil {
			h.Port = n
		}
	}
	return h, true
}

func get(b *sshconfig.Block, key string) string {
	v, _ := b.Get(key)
	return v
}

// Load reads every connectable host from a config tree, in file order.
func Load(t *sshconfig.Tree) []*Host {
	var out []*Host
	for _, tb := range t.HostBlocks() {
		if h, ok := FromBlock(tb.File, tb.Block); ok {
			out = append(out, h)
		}
	}
	return out
}

// Find returns the host declaring alias, matching any of its aliases.
func Find(hosts []*Host, alias string) (*Host, bool) {
	for _, h := range hosts {
		for _, a := range h.Aliases {
			if strings.EqualFold(a, alias) {
				return h, true
			}
		}
	}
	return nil, false
}

// AllTags returns every distinct tag across the given hosts, sorted, which is
// what the tag filter bar is built from.
func AllTags(hosts []*Host) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range hosts {
		for _, t := range h.Tags() {
			k := strings.ToLower(t)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, t)
		}
	}
	sortStrings(out)
	return out
}
