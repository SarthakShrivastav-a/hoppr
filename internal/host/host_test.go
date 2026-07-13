package host

import (
	"testing"

	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

const sample = `Host *
    ServerAliveInterval 60

Host web-* !web-99
    User deploy

Host prod prod-api
    # hoppr: tags=aws,prod; note=main API box
    HostName 203.0.113.10
    User ec2-user
    Port 2222
    IdentityFile ~/.ssh/keys/prod.pem

Host dev
    HostName dev-api.example.com
    User ec2-user
    ProxyJump prod
`

func load(t *testing.T) []*Host {
	t.Helper()
	f := sshconfig.ParseBytes([]byte(sample), "config")
	tree := &sshconfig.Tree{Root: f, Files: []*sshconfig.File{f}}
	return Load(tree)
}

func TestPatternStanzasAreNotConnectableHosts(t *testing.T) {
	hosts := load(t)

	// `Host *` and `Host web-* !web-99` configure other hosts; they are not
	// hosts. Listing "!web-99" in the picker is a bug other managers ship.
	if len(hosts) != 2 {
		var got []string
		for _, h := range hosts {
			got = append(got, h.Alias)
		}
		t.Fatalf("want 2 connectable hosts (prod, dev), got %d: %v", len(hosts), got)
	}
	for _, h := range hosts {
		if IsPattern(h.Alias) {
			t.Errorf("pattern %q leaked into the host list", h.Alias)
		}
	}
}

func TestIsPattern(t *testing.T) {
	patterns := []string{"*", "web-*", "web-?", "!web-99"}
	literals := []string{"prod", "dev", "prod-api", "10.0.0.1"}

	for _, p := range patterns {
		if !IsPattern(p) {
			t.Errorf("%q should be recognized as a pattern", p)
		}
	}
	for _, l := range literals {
		if IsPattern(l) {
			t.Errorf("%q is a literal alias, not a pattern", l)
		}
	}
}

func TestParseFields(t *testing.T) {
	hosts := load(t)
	prod, ok := Find(hosts, "prod")
	if !ok {
		t.Fatal("prod not found")
	}

	if prod.HostName != "203.0.113.10" {
		t.Errorf("HostName = %q", prod.HostName)
	}
	if prod.User != "ec2-user" {
		t.Errorf("User = %q", prod.User)
	}
	if prod.Port != 2222 {
		t.Errorf("Port = %d", prod.Port)
	}
	if len(prod.IdentityFiles) != 1 || prod.IdentityFiles[0] != "~/.ssh/keys/prod.pem" {
		t.Errorf("IdentityFiles = %v", prod.IdentityFiles)
	}
	if prod.Target() != "ec2-user@203.0.113.10" {
		t.Errorf("Target() = %q", prod.Target())
	}
	if prod.Addr() != "203.0.113.10:2222" {
		t.Errorf("Addr() = %q", prod.Addr())
	}
	if !prod.Meta.HasTag("aws") || prod.Meta.Note != "main API box" {
		t.Errorf("annotation not read: %+v", prod.Meta)
	}
}

func TestSecondaryAliasIsFindable(t *testing.T) {
	hosts := load(t)
	// `Host prod prod-api` — connecting by the second name must work. Other
	// managers have shipped bugs where only the first alias is connectable.
	h, ok := Find(hosts, "prod-api")
	if !ok {
		t.Fatal("prod-api (a secondary alias) was not findable")
	}
	if h.Alias != "prod" {
		t.Errorf("want the stanza's primary alias prod, got %q", h.Alias)
	}
}

func TestDefaultsWhenConfigIsSilent(t *testing.T) {
	hosts := load(t)
	dev, _ := Find(hosts, "dev")

	if dev.EffectivePort() != 22 {
		t.Errorf("a host with no Port should default to 22, got %d", dev.EffectivePort())
	}
	if dev.ProxyJump != "prod" {
		t.Errorf("ProxyJump = %q", dev.ProxyJump)
	}
}

func TestAllTags(t *testing.T) {
	got := AllTags(load(t))
	if len(got) != 2 || got[0] != "aws" || got[1] != "prod" {
		t.Fatalf("want [aws prod], got %v", got)
	}
}
