package connect

import (
	"errors"
	"testing"

	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

// hostFrom builds a Host directly, bypassing host.Load's filtering, so that the
// second line of defence is tested rather than the first.
func hostFrom(t *testing.T, alias string) *host.Host {
	t.Helper()

	f := sshconfig.ParseBytes([]byte("Host placeholder\n  HostName x.example.com\n"), "config")
	tree := &sshconfig.Tree{Root: f, Files: []*sshconfig.File{f}}

	hosts := host.Load(tree)
	if len(hosts) != 1 {
		t.Fatalf("fixture broken: got %d hosts", len(hosts))
	}
	// Rewrite the alias to the hostile value after the fact.
	hosts[0].Alias = alias
	return hosts[0]
}

func TestCommandRefusesAnOptionShapedAlias(t *testing.T) {
	h := hostFrom(t, "-oProxyCommand=calc.exe")

	cmd, err := Command(h, Options{})
	if err == nil {
		t.Fatal("Command built an exec.Cmd for an alias ssh would execute as a flag")
	}
	if cmd != nil {
		t.Fatal("a command was returned alongside the error")
	}
	if !errors.Is(err, ErrUnsafeAlias) {
		t.Fatalf("want ErrUnsafeAlias, got %v", err)
	}
}

func TestCheckRefusesEveryOptionShape(t *testing.T) {
	for _, alias := range []string{"-oProxyCommand=calc", "--version", "-v", "-"} {
		if err := Check(hostFrom(t, alias)); err == nil {
			t.Errorf("Check accepted %q, which ssh would read as a flag", alias)
		}
	}
}

func TestCheckAllowsOrdinaryAliases(t *testing.T) {
	for _, alias := range []string{"prod", "dev-api", "203.0.113.10", "web1"} {
		if err := Check(hostFrom(t, alias)); err != nil {
			t.Errorf("Check rejected the legitimate alias %q: %v", alias, err)
		}
	}
}

func TestCommandBuildsNormallyForASafeAlias(t *testing.T) {
	h := hostFrom(t, "prod")

	cmd, err := Command(h, Options{ConfigPath: "/tmp/config"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd == nil {
		t.Fatal("no command was built for a safe alias")
	}

	// The alias must be the last argument, and nothing may precede it that
	// could be reinterpreted.
	args := Args(h, Options{ConfigPath: "/tmp/config"})
	if args[len(args)-1] != "prod" {
		t.Fatalf("want the alias last, got %v", args)
	}
}
