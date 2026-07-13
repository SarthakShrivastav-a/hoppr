package connect

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Spec is a connection described by an ad-hoc ssh command, ready to be written
// into ssh_config as a Host stanza.
type Spec struct {
	Alias         string
	HostName      string
	User          string
	Port          int
	IdentityFiles []string
	ProxyJump     string
	Forwards      []string // raw -L/-R/-D arguments, in the order given

	// Extra holds any other -o Key=Value options, so they survive the import
	// instead of being silently dropped.
	Extra map[string]string
}

// ParseCommand turns a pasted ssh command line into a Spec.
//
// This is the on-ramp. Everyone managing servers by hand has a note, a wiki
// page, or a clipboard manager holding lines like
//
//	cd .\Downloads\ ssh -i dev.pem ec2-user@dev-api.example.com
//
// and retyping that into a form field, flag by flag, is exactly the tedium a
// manager is supposed to remove. Paste the line, get a host.
//
// The leading `cd ...` is tolerated deliberately: that prefix exists precisely
// because the key is somewhere inconvenient, which is the population we most
// want to import. Anything before the `ssh` token is discarded.
func ParseCommand(cmd string) (*Spec, error) {
	args, err := tokenize(cmd)
	if err != nil {
		return nil, err
	}

	args, sawSSH := trimToSSH(args)
	if len(args) == 0 {
		return nil, fmt.Errorf("no ssh command found in %q", strings.TrimSpace(cmd))
	}

	s := &Spec{Extra: map[string]string{}}

	for i := 0; i < len(args); i++ {
		a := args[i]

		// value returns the argument attached to a flag, supporting both the
		// separated form (-p 22) and the joined form (-p22) that ssh accepts.
		value := func() (string, bool) {
			if len(a) > 2 {
				return a[2:], true
			}
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}

		switch {
		case strings.HasPrefix(a, "-i"):
			if v, ok := value(); ok {
				s.IdentityFiles = append(s.IdentityFiles, normalizeKeyPath(v))
			}
		case strings.HasPrefix(a, "-p"):
			if v, ok := value(); ok {
				n, err := strconv.Atoi(v)
				if err != nil {
					return nil, fmt.Errorf("port %q is not a number", v)
				}
				s.Port = n
			}
		case strings.HasPrefix(a, "-J"):
			if v, ok := value(); ok {
				s.ProxyJump = v
			}
		case strings.HasPrefix(a, "-L"), strings.HasPrefix(a, "-R"), strings.HasPrefix(a, "-D"):
			kind := a[:2]
			if v, ok := value(); ok {
				s.Forwards = append(s.Forwards, kind+" "+v)
			}
		case strings.HasPrefix(a, "-o"):
			if v, ok := value(); ok {
				if k, val, found := strings.Cut(v, "="); found {
					s.Extra[strings.TrimSpace(k)] = strings.TrimSpace(val)
				}
			}
		case a == "-l":
			// The old-style user flag: ssh -l ec2-user host
			if i+1 < len(args) {
				i++
				s.User = args[i]
			}
		case strings.HasPrefix(a, "-"):
			// A flag we do not model (-v, -A, -N, -T...). Ignoring it is right:
			// these are per-invocation switches, not host configuration.
			continue

		default:
			// The first bare word is the destination. Anything after it is a
			// remote command, which is not host configuration either.
			if s.HostName != "" {
				continue
			}
			user, hostname, port, err := parseDestination(a)
			if err != nil {
				return nil, err
			}
			s.HostName = hostname
			if user != "" {
				s.User = user
			}
			if port != 0 && s.Port == 0 {
				s.Port = port
			}
		}
	}

	if s.HostName == "" {
		return nil, fmt.Errorf("no destination host found in %q", strings.TrimSpace(cmd))
	}

	// Without an explicit `ssh` token we are guessing, so demand that the
	// destination at least look like one. Otherwise `cd ~/Downloads` parses
	// happily into a host named "cd".
	if !sawSSH && !looksLikeDestination(s.HostName) {
		return nil, fmt.Errorf("%q does not look like an ssh command", strings.TrimSpace(cmd))
	}

	s.Alias = SuggestAlias(s.HostName)
	return s, nil
}

// trimToSSH drops everything up to and including the `ssh` token, so that a
// pasted line beginning with `cd ~/Downloads &&` or a Windows `cd .\Downloads\`
// still parses. It reports whether such a token was there at all.
func trimToSSH(args []string) ([]string, bool) {
	for i, a := range args {
		base := a
		if j := strings.LastIndexAny(base, `/\`); j >= 0 {
			base = base[j+1:]
		}
		base = strings.TrimSuffix(strings.ToLower(base), ".exe")
		if base == "ssh" {
			return args[i+1:], true
		}
	}
	// No `ssh` token. Keep the arguments anyway so that a bare
	// "ec2-user@host -i key.pem" still imports, but tell the caller we are
	// guessing.
	return args, false
}

// looksLikeDestination reports whether s is plausibly a host rather than a
// stray shell word. A user@ prefix, a dot, or a colon is enough; a bare word
// is not.
func looksLikeDestination(s string) bool {
	return strings.ContainsAny(s, "@.:")
}

// parseDestination splits ssh's destination argument, which may be a bare host,
// user@host, or the ssh:// URI form.
func parseDestination(d string) (user, hostname string, port int, err error) {
	if rest, ok := strings.CutPrefix(d, "ssh://"); ok {
		d = rest
		if i := strings.Index(d, "/"); i >= 0 {
			d = d[:i]
		}
		// Only the URI form carries a port in the destination; `host:port` on a
		// plain ssh destination is not a thing, and treating it as one would
		// mangle an IPv6 address.
		if h, p, found := strings.Cut(d, ":"); found && !strings.Contains(h, ":") {
			d = h
			if n, e := strconv.Atoi(p); e == nil {
				port = n
			}
		}
	}

	if u, h, found := strings.Cut(d, "@"); found {
		user, hostname = u, h
	} else {
		hostname = d
	}
	if hostname == "" {
		return "", "", 0, fmt.Errorf("destination %q has no hostname", d)
	}
	return user, hostname, port, nil
}

// normalizeKeyPath tidies a key path from a pasted command.
//
// A path lifted from a Windows shell often arrives as `.\dev.pem` or
// `.\Downloads\dev.pem`, and a leading `.\` is noise that would end up written
// into ssh_config verbatim.
func normalizeKeyPath(p string) string {
	p = strings.Trim(p, `"'`)
	p = strings.TrimPrefix(p, `.\`)
	p = strings.TrimPrefix(p, "./")
	return p
}

// SuggestAlias proposes a short, typeable alias for a hostname.
//
// dev-api.example.com becomes dev-api; an IP address is kept whole, since every
// part of it is load-bearing.
func SuggestAlias(hostname string) string {
	if isIPish(hostname) {
		return hostname
	}
	if i := strings.Index(hostname, "."); i > 0 {
		return hostname[:i]
	}
	return hostname
}

// isIPish reports whether s looks like an IPv4 or IPv6 literal rather than a
// DNS name. It does not need to be a strict parse — it only decides whether
// truncating at the first dot would be destructive.
func isIPish(s string) bool {
	if strings.Contains(s, ":") {
		return true // IPv6
	}
	for _, r := range s {
		if !unicode.IsDigit(r) && r != '.' {
			return false
		}
	}
	return strings.Contains(s, ".")
}
