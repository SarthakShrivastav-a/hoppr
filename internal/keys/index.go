package keys

import (
	"path/filepath"

	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/paths"
)

// Usage is the reverse index: which hosts reference a given key.
//
// This is the question a key inventory is actually for. "I have four .pem files
// — which of them still matters, and what breaks if I delete this one?" is
// unanswerable today without grepping ssh_config by hand, so people never
// delete any of them, and the pile grows.
type Usage struct {
	Key   *Key
	Hosts []*host.Host
}

// Orphaned reports whether no host references this key. An orphan is a
// candidate for deletion, though hoppr only ever says so — it does not act.
func (u Usage) Orphaned() bool { return len(u.Hosts) == 0 }

// Index maps every key to the hosts whose IdentityFile points at it.
//
// Paths are compared after expanding ~ and resolving to absolute form, because
// the same key is legitimately written as ~/.ssh/keys/prod.pem in one stanza
// and C:\Users\me\.ssh\keys\prod.pem in another.
func Index(ks []*Key, hosts []*host.Host) []Usage {
	byPath := make(map[string]*Usage, len(ks))
	out := make([]Usage, 0, len(ks))

	for _, k := range ks {
		out = append(out, Usage{Key: k})
	}
	for i := range out {
		byPath[normalize(out[i].Key.Path)] = &out[i]
	}

	for _, h := range hosts {
		for _, idf := range h.IdentityFiles {
			if u, ok := byPath[normalize(idf)]; ok {
				u.Hosts = append(u.Hosts, h)
			}
		}
	}
	return out
}

// Missing returns the identity files that hosts reference but that do not exist
// on disk.
//
// This is the other half of the same question, and the one that produces a
// baffling "Permission denied (publickey)" at the worst moment: the config
// points at a key that was renamed, moved, or never restored from the old
// laptop. Better to say so up front.
func Missing(hosts []*host.Host) map[string][]*host.Host {
	missing := map[string][]*host.Host{}
	for _, h := range hosts {
		for _, idf := range h.IdentityFiles {
			p := paths.Expand(idf)
			if fileExists(p) {
				continue
			}
			missing[idf] = append(missing[idf], h)
		}
	}
	return missing
}

// normalize resolves a config-style path to a comparable absolute form.
func normalize(p string) string {
	abs, err := filepath.Abs(paths.Expand(p))
	if err != nil {
		abs = paths.Expand(p)
	}
	if filepath.Separator == '\\' {
		return lower(filepath.Clean(abs))
	}
	return filepath.Clean(abs)
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
