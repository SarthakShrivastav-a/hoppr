package keys

import (
	"os"
	"path/filepath"

	"github.com/SarthakShrivastav-a/hoppr/internal/paths"
)

// Locate resolves a key path that may be relative, bare, or already absolute.
//
// This exists because of the single most common shape of an ad-hoc ssh command:
//
//	cd ~/Downloads && ssh -i dev.pem ec2-user@host
//
// The `-i dev.pem` is relative to whatever directory the user happened to be
// standing in, which is why the `cd` is there at all. Copying that path into
// ssh_config verbatim produces an entry that works from exactly one directory
// and fails everywhere else — with "Permission denied (publickey)", which does
// not mention the working directory and sends people hunting in the wrong place.
//
// So: if the path resolves as given, take it. Otherwise go and find the key by
// name in the places keys actually live. Report false if it is nowhere, and let
// the caller decide whether to warn or to store the path unresolved.
func Locate(p string) (string, bool) {
	if p == "" {
		return "", false
	}

	expanded := paths.Expand(p)

	// An absolute path, or a relative one that happens to resolve from here.
	if isFile(expanded) {
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return expanded, true
		}
		return abs, true
	}

	// It did not resolve. If it carries directory separators the user meant a
	// specific location and got it wrong; guessing at a different file with the
	// same name would be worse than saying so.
	if filepath.Base(p) != p {
		return "", false
	}

	// A bare file name. Search the directories keys actually live in, in
	// priority order: the managed directory first, then ~/.ssh, then the
	// download dirs a .pem lands in.
	name := filepath.Base(p)
	for _, root := range searchRoots() {
		cand := filepath.Join(root, name)
		if isFile(cand) {
			return cand, true
		}
	}
	return "", false
}

// searchRoots is where Locate looks, in the order it looks.
func searchRoots() []string {
	var roots []string
	if dir, err := paths.KeyDir(); err == nil {
		roots = append(roots, dir)
	}
	if dir, err := paths.SSHDir(); err == nil {
		roots = append(roots, dir)
	}

	// DefaultRoots covers Downloads, Desktop, and Documents, which is where the
	// key in a pasted command almost always is.
	for _, r := range DefaultRoots() {
		roots = append(roots, r)
	}
	return roots
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
