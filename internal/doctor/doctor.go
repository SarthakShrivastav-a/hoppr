// Package doctor inspects an SSH setup and reports what is wrong with it.
//
// Every check here corresponds to a failure that produces a genuinely unhelpful
// error message at the worst possible moment — "Permission denied (publickey)",
// "Too many authentication failures", "Bad configuration option", "UNPROTECTED
// PRIVATE KEY FILE" — and that has a mechanical, checkable cause.
package doctor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/keys"
	"github.com/SarthakShrivastav-a/hoppr/internal/paths"
)

// Severity ranks a finding.
type Severity int

const (
	// Info is worth saying but is not a problem.
	Info Severity = iota
	// Warn will bite eventually, or is biting silently now.
	Warn
	// Fail means something is broken right now.
	Fail
)

func (s Severity) String() string {
	switch s {
	case Fail:
		return "fail"
	case Warn:
		return "warn"
	default:
		return "ok"
	}
}

// Finding is one diagnosis.
type Finding struct {
	Severity Severity
	Title    string

	// Detail explains the consequence, not just the observation. "mode 0644"
	// is an observation; "ssh will refuse this key" is why anyone should care.
	Detail string

	// Fix, when non-nil, repairs the finding.
	Fix func() error

	// FixLabel describes what Fix would do, so the user can decline it.
	FixLabel string
}

// Fixable reports whether this finding can be repaired automatically.
func (f Finding) Fixable() bool { return f.Fix != nil }

// Report is the full diagnosis.
type Report struct {
	Findings []Finding
}

// Add appends a finding.
func (r *Report) Add(f Finding) { r.Findings = append(r.Findings, f) }

// Problems returns only the findings that are actually wrong.
func (r *Report) Problems() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Severity > Info {
			out = append(out, f)
		}
	}
	return out
}

// Fixable returns the problems that can be repaired automatically.
func (r *Report) Fixable() []Finding {
	var out []Finding
	for _, f := range r.Problems() {
		if f.Fixable() {
			out = append(out, f)
		}
	}
	return out
}

// Run performs every check against the given hosts and keys.
func Run(configPath string, hosts []*host.Host, ks []*keys.Key) *Report {
	r := &Report{}

	checkSSHBinary(r)
	checkSSHDir(r)
	checkConfigLineEndings(r, configPath)
	checkDuplicateAliases(r, hosts)
	checkKeyPermissions(r, ks)
	checkMissingKeys(r, hosts)
	checkStrayKeys(r, ks)
	checkIdentitiesOnly(r, hosts)

	return r
}

// checkSSHBinary confirms there is an ssh to shell out to at all. Everything
// else hoppr does is downstream of this.
func checkSSHBinary(r *Report) {
	path, err := exec.LookPath("ssh")
	if err != nil {
		r.Add(Finding{
			Severity: Fail,
			Title:    "no ssh binary on PATH",
			Detail:   "hoppr drives the system ssh client; without it nothing here can connect. On Windows, install the OpenSSH Client optional feature.",
		})
		return
	}
	r.Add(Finding{Severity: Info, Title: "ssh client", Detail: path})
}

// checkSSHDir catches the root cause of most Windows key-permission grief: keys
// inherit the ACL of ~/.ssh, so if that folder is open, every key created in it
// starts out open too.
func checkSSHDir(r *Report) {
	dir, err := paths.SSHDir()
	if err != nil {
		return
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}

	st := keys.Audit(dir)
	if st.OK {
		r.Add(Finding{Severity: Info, Title: "~/.ssh permissions", Detail: st.Detail})
		return
	}

	r.Add(Finding{
		Severity: Warn,
		Title:    "~/.ssh is too permissive",
		Detail:   st.Detail + ". New keys inherit this, so they start out exposed too.",
		FixLabel: "lock ~/.ssh to your account",
		Fix:      func() error { return keys.Harden(dir) },
	})
}

// checkConfigLineEndings catches a Windows-only trap: OpenSSH can reject a
// config with CRLF line endings, and the error it gives — "Bad configuration
// option" — points at the keyword rather than at the invisible carriage return
// that is actually the problem.
func checkConfigLineEndings(r *Report, configPath string) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		r.Add(Finding{
			Severity: Fail,
			Title:    "ssh_config starts with a byte-order mark",
			Detail:   "OpenSSH reads the BOM as part of the first keyword and rejects the line. A Windows editor added it.",
		})
	}
	if bytes.Contains(raw, []byte("\r\n")) {
		r.Add(Finding{
			Severity: Warn,
			Title:    "ssh_config has CRLF line endings",
			Detail:   "Some OpenSSH builds reject these with a misleading \"Bad configuration option\" that names the keyword rather than the stray carriage return.",
		})
	}
}

// checkDuplicateAliases catches the same alias declared in two stanzas, where
// OpenSSH silently applies the first and ignores the second — so the user edits
// the one they can see and nothing changes.
func checkDuplicateAliases(r *Report, hosts []*host.Host) {
	seen := map[string][]string{}
	for _, h := range hosts {
		for _, a := range h.Aliases {
			seen[a] = append(seen[a], h.File)
		}
	}

	var dupes []string
	for alias, files := range seen {
		if len(files) > 1 {
			dupes = append(dupes, fmt.Sprintf("%s (in %s)", alias, strings.Join(shorten(files), ", ")))
		}
	}
	if len(dupes) == 0 {
		return
	}
	sort.Strings(dupes)

	r.Add(Finding{
		Severity: Warn,
		Title:    fmt.Sprintf("%d alias declared more than once", len(dupes)),
		Detail:   strings.Join(dupes, "; ") + ". OpenSSH applies the first match and ignores the rest, so editing the later one appears to do nothing.",
	})
}

// checkKeyPermissions is the headline Windows check.
func checkKeyPermissions(r *Report, ks []*keys.Key) {
	for _, k := range ks {
		if k.Perm.OK {
			continue
		}

		f := Finding{
			Severity: Fail,
			Title:    "key permissions too open: " + k.Name(),
			Detail:   k.Perm.Detail,
		}
		if k.Perm.Fixable {
			path := k.Path
			f.FixLabel = "lock " + k.Name() + " to your account"
			f.Fix = func() error { return keys.Fix(path) }
		}
		r.Add(f)
	}
}

// checkMissingKeys catches a config pointing at a key that is not there, which
// surfaces at connect time as a bare "Permission denied (publickey)".
func checkMissingKeys(r *Report, hosts []*host.Host) {
	missing := keys.Missing(hosts)
	if len(missing) == 0 {
		return
	}

	paths_ := make([]string, 0, len(missing))
	for p := range missing {
		paths_ = append(paths_, p)
	}
	sort.Strings(paths_)

	for _, p := range paths_ {
		var aliases []string
		for _, h := range missing[p] {
			aliases = append(aliases, h.Alias)
		}
		sort.Strings(aliases)

		r.Add(Finding{
			Severity: Fail,
			Title:    "identity file not found: " + p,
			Detail: fmt.Sprintf("referenced by %s. Connecting will fail with \"Permission denied (publickey)\", which does not mention the missing file.",
				strings.Join(aliases, ", ")),
		})
	}
}

// checkStrayKeys flags private keys living outside the managed directory.
func checkStrayKeys(r *Report, ks []*keys.Key) {
	var stray []*keys.Key
	for _, k := range ks {
		if k.Stray() {
			stray = append(stray, k)
		}
	}
	if len(stray) == 0 {
		return
	}

	var names []string
	for _, k := range stray {
		names = append(names, filepath.Base(filepath.Dir(k.Path))+"/"+k.Name())
	}

	r.Add(Finding{
		Severity: Warn,
		Title:    fmt.Sprintf("%d private key outside ~/.ssh", len(stray)),
		Detail: strings.Join(names, ", ") +
			". These are in directories a browser writes to and a cleanup tool empties. Run `hoppr keys adopt` to move them and repoint the hosts that use them.",
	})
}

// checkIdentitiesOnly catches the cause of "Too many authentication failures".
//
// A host with an explicit IdentityFile but no IdentitiesOnly still offers every
// key the agent holds, in agent order, before the one the config names. A server
// that permits six attempts disconnects first. The failure is confusing because
// the config is visibly correct.
func checkIdentitiesOnly(r *Report, hosts []*host.Host) {
	var affected []string
	for _, h := range hosts {
		if len(h.IdentityFiles) == 0 {
			continue
		}
		if v, ok := h.Block().Get("IdentitiesOnly"); ok && strings.EqualFold(v, "yes") {
			continue
		}
		affected = append(affected, h.Alias)
	}
	if len(affected) == 0 {
		return
	}
	sort.Strings(affected)

	hs := affected
	r.Add(Finding{
		Severity: Warn,
		Title:    fmt.Sprintf("%d host names a key but does not set IdentitiesOnly", len(affected)),
		Detail: strings.Join(affected, ", ") +
			". ssh will still offer every key in your agent first; if the agent holds more keys than the server allows attempts, the connection dies with \"Too many authentication failures\" before the right key is tried.",
		FixLabel: "set IdentitiesOnly yes on these hosts",
		Fix: func() error {
			for _, h := range hostsByAlias(hosts, hs) {
				h.Block().Set("IdentitiesOnly", "yes")
			}
			return saveAll(hosts, hs)
		},
	})
}

func hostsByAlias(hosts []*host.Host, aliases []string) []*host.Host {
	want := map[string]bool{}
	for _, a := range aliases {
		want[a] = true
	}
	var out []*host.Host
	for _, h := range hosts {
		if want[h.Alias] {
			out = append(out, h)
		}
	}
	return out
}

// saveAll writes back every distinct config file touched by the given aliases.
func saveAll(hosts []*host.Host, aliases []string) error {
	seen := map[string]bool{}
	for _, h := range hostsByAlias(hosts, aliases) {
		f := h.ConfigFile()
		if f == nil || seen[f.Path] {
			continue
		}
		seen[f.Path] = true
		if err := f.Save(); err != nil {
			return err
		}
	}
	return nil
}

// shorten trims config paths to their file name for display.
func shorten(ps []string) []string {
	out := make([]string, 0, len(ps))
	seen := map[string]bool{}
	for _, p := range ps {
		b := filepath.Base(p)
		if seen[b] {
			continue
		}
		seen[b] = true
		out = append(out, b)
	}
	return out
}
