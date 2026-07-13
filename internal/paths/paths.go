// Package paths resolves the handful of filesystem locations hoppr cares about.
//
// It exists mostly to get Windows right. OpenSSH on Windows reads
// %USERPROFILE%\.ssh\config, but if HOME happens to be unset — the default on a
// stock Windows box — ssh can end up looking for a literal //.ssh/id_rsa. Every
// path hoppr touches is resolved through here so that oddity is handled once.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// Home returns the user's home directory.
//
// os.UserHomeDir consults HOME on Unix and USERPROFILE on Windows, which is the
// same order OpenSSH uses, so the two agree on where ~/.ssh lives.
func Home() (string, error) {
	return os.UserHomeDir()
}

// SSHDir returns ~/.ssh.
func SSHDir() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh"), nil
}

// Config returns the path to the user's ssh_config.
//
// HOPPR_CONFIG overrides it, which is what the tests use and what lets a user
// point hoppr at a work config without disturbing their default one.
func Config() (string, error) {
	if p := os.Getenv("HOPPR_CONFIG"); p != "" {
		return Expand(p), nil
	}
	dir, err := SSHDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config"), nil
}

// KeyDir returns ~/.ssh/keys, the directory hoppr adopts stray keys into.
//
// Keys downloaded from a cloud console land in Downloads and stay there, which
// is how people end up with a production .pem sitting in a directory their
// browser writes to. Giving them one obvious home is half the fix; locking down
// its permissions is the other half.
func KeyDir() (string, error) {
	dir, err := SSHDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "keys"), nil
}

// StateDir returns the directory for hoppr's own state, which today is just
// connection history. Nothing in here is authoritative: delete it and hoppr
// still works, it just forgets what you connected to last.
func StateDir() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".hoppr"), nil
}

// Expand resolves a leading ~ and any environment variables in p.
func Expand(p string) string {
	p = os.ExpandEnv(p)
	return expandTilde(p)
}

func expandTilde(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return p
	}
	home, err := Home()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// Tildify is the inverse of Expand: it rewrites a path under the home directory
// back to ~/... form, always with forward slashes.
//
// This matters more than it looks. Writing an absolute IdentityFile like
// C:\Users\sarthak\.ssh\keys\prod.pem into ssh_config breaks that config the
// moment it is synced to a Linux box or into WSL — a real, repeatedly reported
// failure in other managers. A tilde path with forward slashes is understood by
// OpenSSH on every platform, so that is what hoppr writes.
func Tildify(p string) string {
	home, err := Home()
	if err != nil || home == "" {
		return filepath.ToSlash(p)
	}

	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	rel, err := filepath.Rel(home, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(p)
	}
	if rel == "." {
		return "~"
	}
	return "~/" + filepath.ToSlash(rel)
}
