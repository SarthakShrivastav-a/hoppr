package sshconfig

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxIncludeDepth bounds Include recursion. OpenSSH caps nesting at 16; a
// config that exceeds that is either pathological or a cycle, and either way we
// stop rather than spin.
const maxIncludeDepth = 16

// Tree is a config file plus every file it pulls in via Include, in the order
// OpenSSH would read them.
//
// Splitting a config across ~/.ssh/conf.d/*.conf is common on any machine with
// more than a handful of hosts, and several SSH managers simply do not see
// those hosts: they read ~/.ssh/config, find no Host blocks, and report an
// empty list. Worse, when they add a host they append it to the root file even
// though the user clearly keeps hosts elsewhere.
//
// A Tree keeps every file addressable, so a host found in conf.d/work.conf is
// edited and saved back to conf.d/work.conf.
type Tree struct {
	Root  *File
	Files []*File // Root first, then includes in read order
}

// LoadTree parses path and follows its Include directives.
//
// An Include that matches nothing is not an error — OpenSSH tolerates it, and
// a glob like conf.d/*.conf on a machine with no conf.d is perfectly normal.
func LoadTree(path string) (*Tree, error) {
	t := &Tree{}
	seen := map[string]bool{}
	root, err := t.load(path, seen, 0)
	if err != nil {
		return nil, err
	}
	t.Root = root
	return t, nil
}

func (t *Tree) load(path string, seen map[string]bool, depth int) (*File, error) {
	if depth > maxIncludeDepth {
		return nil, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if seen[abs] {
		// A cycle, or the same file included twice. Either way, read it once.
		return nil, nil
	}
	seen[abs] = true

	f, err := Load(path)
	if err != nil {
		return nil, err
	}
	t.Files = append(t.Files, f)

	for _, inc := range includePaths(f) {
		for _, m := range expandInclude(inc, filepath.Dir(path)) {
			if _, err := t.load(m, seen, depth+1); err != nil {
				return nil, err
			}
		}
	}
	return f, nil
}

// includePaths collects the arguments of every top-level Include directive.
//
// Include inside a Host or Match block is legal but rare, and it is scoped to
// that block. hoppr reads only top-level includes; a block-scoped one still
// renders losslessly, we just do not follow it.
func includePaths(f *File) []string {
	var out []string
	for _, n := range f.Nodes {
		if d, ok := n.(*Directive); ok && d.Is("Include") {
			out = append(out, splitArgs(d.Value)...)
		}
	}
	return out
}

// expandInclude resolves one Include argument to concrete file paths.
//
// Per ssh_config(5), a relative Include is relative to ~/.ssh for a user
// config, not to the current working directory. base is the directory of the
// including file, which is ~/.ssh in the normal case and the right anchor for
// a nested include besides.
func expandInclude(pattern, base string) []string {
	p := ExpandTilde(pattern)
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}

	matches, err := filepath.Glob(p)
	if err != nil || len(matches) == 0 {
		return nil
	}
	// Glob order is already lexical, but pin it: OpenSSH reads includes in a
	// deterministic order and so must we, or a host could resolve differently
	// between runs.
	sort.Strings(matches)

	var files []string
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil || st.IsDir() {
			continue
		}
		files = append(files, m)
	}
	return files
}

// ExpandTilde replaces a leading ~ with the user's home directory.
//
// OpenSSH understands ~ in IdentityFile and Include on every platform,
// including Windows, so hoppr has to as well.
func ExpandTilde(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// HostBlocks returns every Host stanza across the whole tree, paired with the
// file that declares it so edits can be saved back to the right place.
func (t *Tree) HostBlocks() []TreeBlock {
	var out []TreeBlock
	for _, f := range t.Files {
		for _, b := range f.HostBlocks() {
			out = append(out, TreeBlock{File: f, Block: b})
		}
	}
	return out
}

// TreeBlock is a Host stanza together with the file it lives in.
type TreeBlock struct {
	File  *File
	Block *Block
}

// FindHost locates the stanza declaring alias anywhere in the tree.
func (t *Tree) FindHost(alias string) (TreeBlock, bool) {
	for _, tb := range t.HostBlocks() {
		for _, p := range tb.Block.Patterns() {
			if p == alias {
				return tb, true
			}
		}
	}
	return TreeBlock{}, false
}

// Save writes back only the files that actually changed.
//
// Editing one host in ~/.ssh/config should not bump the mtime of every file in
// conf.d, both because it is noisy in a dotfiles git repo and because those
// files may be symlinks into someone else's tree.
func (t *Tree) Save() error {
	for _, f := range t.Files {
		if !f.Dirty() {
			continue
		}
		if err := f.Save(); err != nil {
			return err
		}
	}
	return nil
}
