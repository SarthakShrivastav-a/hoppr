package keys

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SarthakShrivastav-a/hoppr/internal/paths"
)

// scanDepth bounds how deep below each root we descend.
//
// The roots are ~/.ssh and the handful of directories a browser writes to. We
// want prod.pem sitting loose in Downloads, and prod.pem one folder down in
// Downloads/aws-keys. We do not want to walk a node_modules tree that happens
// to live under Documents.
const scanDepth = 3

// skipDirs are never descended into. They are large, they never contain a key
// the user put there on purpose, and walking them makes a scan feel broken.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"vendor":       true,
	"AppData":      true,
	"Library":      true,
	".cache":       true,
	"__pycache__":  true,
	"venv":         true,
	".venv":        true,
}

// DefaultRoots returns the directories worth scanning for stray keys.
//
// Downloads is first for a reason: it is where a .pem from an AWS or Azure
// console lands, and where it then stays. Every one of these is a directory
// a browser can write to, which is precisely the argument for moving keys out
// of them.
func DefaultRoots() []string {
	home, err := paths.Home()
	if err != nil {
		return nil
	}
	var roots []string
	for _, d := range []string{".ssh", "Downloads", "Desktop", "Documents", "keys", ".keys"} {
		p := filepath.Join(home, d)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			roots = append(roots, p)
		}
	}
	return roots
}

// Scan walks roots and returns every private key it finds, sorted by path.
//
// Unreadable files and directories are skipped rather than reported. A scan
// that aborts because it met one permission-denied folder under Documents is
// useless; the user wants the keys we could find.
func Scan(roots []string) []*Key {
	seen := map[string]bool{}
	var found []*Key

	for _, root := range roots {
		rootDepth := strings.Count(filepath.Clean(root), string(filepath.Separator))

		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// A directory we cannot read is not a failure of the scan.
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}

			if d.IsDir() {
				if path != root && skipDirs[d.Name()] {
					return fs.SkipDir
				}
				if strings.Count(filepath.Clean(path), string(filepath.Separator))-rootDepth > scanDepth {
					return fs.SkipDir
				}
				return nil
			}

			// .pub files are the public half; they are found via their private
			// sibling, not on their own.
			if strings.HasSuffix(path, ".pub") || strings.HasSuffix(path, ".ppk") {
				return nil
			}

			abs, err := filepath.Abs(path)
			if err != nil {
				abs = path
			}
			if seen[abs] {
				return nil
			}

			k, err := Load(path)
			if err != nil {
				// ErrNotAKey is the overwhelmingly common case: it just means
				// this was an ordinary file.
				return nil
			}
			seen[abs] = true
			found = append(found, k)
			return nil
		})
	}

	sort.Slice(found, func(i, j int) bool { return found[i].Path < found[j].Path })
	return found
}

// Stray reports whether the key lives outside the managed key directory.
//
// A stray key is not broken, but it is the one that gets deleted by a Downloads
// cleanup, synced to a cloud drive, or left world-readable — so it is the one
// worth offering to adopt.
func (k *Key) Stray() bool {
	dir, err := paths.KeyDir()
	if err != nil {
		return false
	}
	sshDir, err := paths.SSHDir()
	if err != nil {
		return false
	}

	parent := filepath.Dir(mustAbs(k.Path))
	return !samePath(parent, dir) && !samePath(parent, sshDir)
}

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// samePath compares two paths for equality, case-insensitively on Windows,
// where C:\Users\Me and C:\users\me are the same directory.
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if filepath.Separator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}
