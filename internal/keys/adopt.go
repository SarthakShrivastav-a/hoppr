package keys

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/paths"
)

// AdoptResult describes what adopting a key did, so the caller can report it
// honestly rather than just claiming success.
type AdoptResult struct {
	From string
	To   string

	// Rewritten lists the host aliases whose IdentityFile was repointed at the
	// key's new home.
	Rewritten []string

	// Hardened reports whether the key's permissions had to be repaired.
	Hardened bool
}

// Adopt moves a key into the managed key directory, locks down its permissions,
// and repoints every host that referenced it at its new location.
//
// The move is the easy part. Repointing the config is what makes it safe to do:
// without it, adopting a key silently breaks every host that used it, which is
// worse than leaving the key in Downloads.
//
// The key is copied and then removed rather than renamed, because the source is
// very often on a different volume — a .pem in Downloads on C: and a key
// directory on a user profile redirected elsewhere is an ordinary corporate
// setup, and os.Rename fails across volumes.
func Adopt(k *Key, hosts []*host.Host) (*AdoptResult, error) {
	dir, err := paths.KeyDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}
	// The directory's own ACL is what new keys inherit, so it has to be locked
	// down too, or every key adopted into it starts out group-readable.
	if err := Harden(dir); err != nil {
		return nil, fmt.Errorf("harden key directory: %w", err)
	}

	dst := filepath.Join(dir, k.Name())
	if samePath(dst, k.Path) {
		// Already home. Still worth fixing permissions.
		res := &AdoptResult{From: k.Path, To: k.Path}
		if !k.Perm.OK {
			if err := Fix(k.Path); err != nil {
				return nil, err
			}
			res.Hardened = true
		}
		return res, nil
	}

	dst, err = uniquePath(dst)
	if err != nil {
		return nil, err
	}

	if err := copyFile(k.Path, dst); err != nil {
		return nil, err
	}
	if err := Harden(dst); err != nil {
		// The key is copied but unprotected. Do not leave it that way.
		os.Remove(dst)
		return nil, fmt.Errorf("harden %s: %w", dst, err)
	}

	// Bring the public half along if there is one; ssh-copy-id and agent
	// tooling both want it, and leaving it behind orphans it.
	if k.PublicPath != "" {
		_ = copyFile(k.PublicPath, dst+".pub")
	}

	res := &AdoptResult{From: k.Path, To: dst, Hardened: true}

	// Repoint the config before deleting the original, so that a failure here
	// leaves the user with a working config and a duplicated key rather than a
	// broken config and no key.
	res.Rewritten = repoint(hosts, k.Path, dst)

	if err := os.Remove(k.Path); err != nil {
		return nil, fmt.Errorf("copied to %s, but could not remove the original: %w", dst, err)
	}
	if k.PublicPath != "" {
		_ = os.Remove(k.PublicPath)
	}

	k.Path = dst
	k.Perm = Audit(dst)
	return res, nil
}

// repoint rewrites IdentityFile entries pointing at oldPath to newPath, and
// returns the aliases it touched.
//
// The new path is written in tilde form so the config stays portable between
// Windows, Linux, and WSL.
func repoint(hosts []*host.Host, oldPath, newPath string) []string {
	want := normalize(oldPath)
	tilde := paths.Tildify(newPath)

	var touched []string
	for _, h := range hosts {
		idfs := h.IdentityFiles
		changed := false

		for i, idf := range idfs {
			if normalize(idf) != want {
				continue
			}
			idfs[i] = tilde
			changed = true
		}
		if !changed {
			continue
		}

		h.Block().SetAll("IdentityFile", idfs)

		// A host with an explicit key should use only that key. Without
		// IdentitiesOnly, ssh offers every key the agent holds first, and a
		// server that allows six attempts will slam the door with "Too many
		// authentication failures" before reaching the right one.
		if _, ok := h.Block().Get("IdentitiesOnly"); !ok {
			h.Block().Set("IdentitiesOnly", "yes")
		}
		touched = append(touched, h.Alias)
	}
	return touched
}

// uniquePath avoids clobbering an existing key that happens to share a name.
// Two different projects both handing you a key called id_rsa.pem is not a
// hypothetical.
func uniquePath(dst string) (string, error) {
	if !fileExists(dst) {
		return dst, nil
	}

	ext := filepath.Ext(dst)
	base := strings.TrimSuffix(dst, ext)
	for i := 2; i < 100; i++ {
		cand := fmt.Sprintf("%s-%d%s", base, i, ext)
		if !fileExists(cand) {
			return cand, nil
		}
	}
	return "", fmt.Errorf("cannot find a free name near %s", dst)
}

// copyFile writes src to dst, creating dst with owner-only permissions from the
// start rather than creating it open and tightening it afterwards.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
