package sshconfig

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// String renders the file back to text.
//
// For a file that has not been edited this returns the exact bytes that were
// parsed. That property is what the round-trip tests pin down, and it is the
// whole reason the AST carries raw indentation and separators around.
func (f *File) String() string {
	var sb strings.Builder
	for _, n := range f.Nodes {
		n.render(&sb)
	}
	out := sb.String()
	if !f.TrailingNewline {
		out = strings.TrimSuffix(out, "\n")
	}
	return out
}

// WriteTo renders the file to w.
func (f *File) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, f.String())
	return int64(n), err
}

// Save writes the file back to its own path, atomically.
//
// The rename-into-place dance matters more here than in most places: this is
// the user's ~/.ssh/config. A half-written config after a crash or a full disk
// would lock them out of every host they own, so the new content is fully
// materialized in a sibling temp file and only then swapped in.
//
// The mode of the existing file is preserved. On a fresh file we choose 0600,
// because a config can name ProxyJump targets and internal hostnames, and there
// is no reason for it to be world-readable.
func (f *File) Save() error {
	if f.Path == "" {
		return fmt.Errorf("sshconfig: cannot save a file with no path")
	}

	content := f.String()

	mode := os.FileMode(0o600)
	if st, err := os.Stat(f.Path); err == nil {
		mode = st.Mode().Perm()
	}

	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("sshconfig: create %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".hoppr-config-*")
	if err != nil {
		return fmt.Errorf("sshconfig: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := io.WriteString(tmp, content); err != nil {
		tmp.Close()
		return fmt.Errorf("sshconfig: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sshconfig: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("sshconfig: close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("sshconfig: chmod temp file: %w", err)
	}

	// Windows will not rename over an existing file, so clear the way first.
	// This is the one moment the config is absent; the temp file is already
	// complete and fsynced, so the window is a single syscall wide.
	if err := os.Rename(tmpName, f.Path); err != nil {
		if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("sshconfig: replace %s: %w", f.Path, err)
		}
		if err := os.Rename(tmpName, f.Path); err != nil {
			return fmt.Errorf("sshconfig: rename into place: %w", err)
		}
	}

	// What is on disk is now what we rendered, so the file is clean again.
	f.orig = content
	return nil
}
