package connect

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/SarthakShrivastav-a/hoppr/internal/paths"
	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

// Apply writes the Spec into f as a new Host stanza and returns it.
//
// It fails rather than overwriting when the alias already exists. Silently
// replacing a host the user already has is how a manager destroys a working
// config, and the caller is better placed to offer a rename.
func (s *Spec) Apply(f *sshconfig.File) (*sshconfig.Block, error) {
	if s.Alias == "" {
		return nil, fmt.Errorf("a host needs an alias")
	}
	if f.FindHost(s.Alias) != nil {
		return nil, fmt.Errorf("host %q already exists in %s", s.Alias, f.Path)
	}

	b := f.AddHost(s.Alias)
	b.Set("HostName", s.HostName)

	if s.User != "" {
		b.Set("User", s.User)
	}
	if s.Port != 0 && s.Port != 22 {
		b.Set("Port", strconv.Itoa(s.Port))
	}

	if len(s.IdentityFiles) > 0 {
		// Written in tilde form so the config survives being synced to another
		// machine or into WSL.
		tilde := make([]string, len(s.IdentityFiles))
		for i, p := range s.IdentityFiles {
			tilde[i] = paths.Tildify(paths.Expand(p))
		}
		b.SetAll("IdentityFile", tilde)

		// An explicit key means we should use only that key. Otherwise ssh
		// offers every key in the agent first and can burn through the server's
		// auth attempts before reaching this one.
		b.Set("IdentitiesOnly", "yes")
	}

	if s.ProxyJump != "" {
		b.Set("ProxyJump", s.ProxyJump)
	}

	for _, fwd := range s.Forwards {
		kind, arg, ok := strings.Cut(fwd, " ")
		if !ok {
			continue
		}
		switch kind {
		case "-L":
			bind, dest := splitForward(arg)
			b.SetArgs("LocalForward", bind, dest)
		case "-R":
			bind, dest := splitForward(arg)
			b.SetArgs("RemoteForward", bind, dest)
		case "-D":
			b.Set("DynamicForward", arg)
		}
	}

	// Deterministic order, so importing the same command twice produces the
	// same file rather than a diff that depends on map iteration.
	extras := make([]string, 0, len(s.Extra))
	for k := range s.Extra {
		extras = append(extras, k)
	}
	sort.Strings(extras)
	for _, k := range extras {
		b.Set(k, s.Extra[k])
	}

	return b, nil
}

// splitForward converts ssh's command-line forward syntax into the two
// arguments ssh_config wants: `-L 8080:localhost:80` on the command line is
// `LocalForward 8080 localhost:80` in the file.
//
// The split is from the right because the bind side is optional and may itself
// carry an address: `-L 127.0.0.1:8080:localhost:80` is four fields, and the
// last two are always the destination.
func splitForward(arg string) (bind, dest string) {
	parts := strings.Split(arg, ":")
	if len(parts) < 3 {
		return arg, ""
	}
	bind = strings.Join(parts[:len(parts)-2], ":")
	dest = strings.Join(parts[len(parts)-2:], ":")
	return bind, dest
}
