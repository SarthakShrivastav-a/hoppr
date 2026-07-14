package host

import (
	"testing"

	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

// A hostile config, of the kind you get from a shared dotfiles repo or a
// colleague's conf.d file that you Include.
//
// `ssh -oProxyCommand=...` executes the command it is given. An alias beginning
// with '-' is therefore not a host at all: it is a flag. Rendering it in a list
// of servers turns it into one-keypress command execution that looks completely
// ordinary.
const hostileConfig = `Host -oProxyCommand=calc.exe
    HostName evil.example.com

Host --version
    HostName also-not-a-host.example.com

Host legit
    HostName real.example.com
`

func TestOptionShapedAliasIsNeverListed(t *testing.T) {
	f := sshconfig.ParseBytes([]byte(hostileConfig), "config")
	tree := &sshconfig.Tree{Root: f, Files: []*sshconfig.File{f}}

	hosts := Load(tree)

	if len(hosts) != 1 {
		var got []string
		for _, h := range hosts {
			got = append(got, h.Alias)
		}
		t.Fatalf("only the legit host should be connectable, got %v", got)
	}
	if hosts[0].Alias != "legit" {
		t.Fatalf("wrong host survived: %q", hosts[0].Alias)
	}
}

func TestOptionShapedAliasIsNotFindable(t *testing.T) {
	f := sshconfig.ParseBytes([]byte(hostileConfig), "config")
	tree := &sshconfig.Tree{Root: f, Files: []*sshconfig.File{f}}
	hosts := Load(tree)

	// Even asking for it by name must not produce it.
	if _, ok := Find(hosts, "-oProxyCommand=calc.exe"); ok {
		t.Fatal("an option-shaped alias was findable by name")
	}
}

func TestOptionShapedAliasNeverMatchesASearch(t *testing.T) {
	f := sshconfig.ParseBytes([]byte(hostileConfig), "config")
	tree := &sshconfig.Tree{Root: f, Files: []*sshconfig.File{f}}
	hosts := Load(tree)

	// Fuzzy search must not surface it either — the whole risk is that it looks
	// like a normal row you can press enter on.
	for _, m := range Filter(hosts, "proxy", nil) {
		if IsUnsafeAlias(m.Host.Alias) {
			t.Fatalf("search surfaced an option-shaped alias: %q", m.Host.Alias)
		}
	}
	for _, m := range Filter(hosts, "", nil) {
		if IsUnsafeAlias(m.Host.Alias) {
			t.Fatalf("the unfiltered list contains an option-shaped alias: %q", m.Host.Alias)
		}
	}
}

func TestIsUnsafeAlias(t *testing.T) {
	unsafe := []string{"-oProxyCommand=calc", "--version", "-v", "-"}
	safe := []string{"prod", "dev-api", "web1", "203.0.113.10", "a-b-c"}

	for _, s := range unsafe {
		if !IsUnsafeAlias(s) {
			t.Errorf("%q should be rejected: ssh would read it as a flag", s)
		}
		if Connectable(s) {
			t.Errorf("%q must not be connectable", s)
		}
	}
	for _, s := range safe {
		if IsUnsafeAlias(s) {
			t.Errorf("%q is a legitimate alias and was rejected", s)
		}
		if !Connectable(s) {
			t.Errorf("%q should be connectable", s)
		}
	}
}

func TestConnectableRejectsPatternsAndEmpty(t *testing.T) {
	for _, s := range []string{"", "*", "web-*", "!web-9", "-flag"} {
		if Connectable(s) {
			t.Errorf("%q must not be connectable", s)
		}
	}
}
