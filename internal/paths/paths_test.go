package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTildifyRewritesPathsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// The point of Tildify: an absolute path like C:\Users\me\.ssh\keys\prod.pem
	// written into ssh_config breaks that config the moment it is synced to a
	// Linux box or read from WSL. A tilde path with forward slashes works
	// everywhere OpenSSH runs.
	got := Tildify(filepath.Join(home, ".ssh", "keys", "prod.pem"))
	if got != "~/.ssh/keys/prod.pem" {
		t.Errorf("Tildify = %q, want ~/.ssh/keys/prod.pem", got)
	}

	if strings.Contains(got, `\`) {
		t.Errorf("Tildify emitted a backslash, which is not portable: %q", got)
	}
}

func TestTildifyLeavesPathsOutsideHomeAlone(t *testing.T) {
	home := t.TempDir()
	other := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	p := filepath.Join(other, "key.pem")
	got := Tildify(p)

	if strings.HasPrefix(got, "~") {
		t.Errorf("a path outside home should not be tildified, got %q", got)
	}
	// It should still be slash-normalized, since ssh_config reads those fine on
	// every platform.
	if got != filepath.ToSlash(p) {
		t.Errorf("Tildify = %q, want %q", got, filepath.ToSlash(p))
	}
}

func TestTildifyHandlesHomeItself(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if got := Tildify(home); got != "~" {
		t.Errorf("Tildify(home) = %q, want ~", got)
	}
}

func TestExpandIsTheInverse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	want := filepath.Join(home, ".ssh", "keys", "prod.pem")
	if got := Expand("~/.ssh/keys/prod.pem"); got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}

	// A round trip must be stable, or repeatedly saving a host would keep
	// rewriting the same line.
	if got := Tildify(Expand("~/.ssh/keys/prod.pem")); got != "~/.ssh/keys/prod.pem" {
		t.Errorf("round trip changed the path: %q", got)
	}
}

func TestExpandLeavesPlainPathsAlone(t *testing.T) {
	for _, p := range []string{"prod.pem", "/etc/ssh/config", "./key"} {
		if got := Expand(p); got != p {
			t.Errorf("Expand(%q) = %q, want it unchanged", p, got)
		}
	}
}

func TestConfigHonoursTheEnvironmentOverride(t *testing.T) {
	t.Setenv("HOPPR_CONFIG", "/tmp/other-config")

	got, err := Config()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/other-config" {
		t.Errorf("Config() = %q, want the HOPPR_CONFIG override", got)
	}
}

func TestConfigDefaultsToSSHDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOPPR_CONFIG", "")

	got, err := Config()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".ssh", "config"); got != want {
		t.Errorf("Config() = %q, want %q", got, want)
	}
}
