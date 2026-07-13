package connect

import (
	"strings"
	"testing"

	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

func TestParseTheCommandPeopleActuallyPaste(t *testing.T) {
	// Lifted verbatim from a clipboard manager, `cd` prefix and all. That
	// prefix exists precisely because the key is somewhere inconvenient, which
	// is the case we most want to import.
	s, err := ParseCommand(`cd .\Downloads\ ssh -i dev.pem ec2-user@dev-api.example.com`)
	if err != nil {
		t.Fatal(err)
	}

	if s.HostName != "dev-api.example.com" {
		t.Errorf("HostName = %q", s.HostName)
	}
	if s.User != "ec2-user" {
		t.Errorf("User = %q", s.User)
	}
	if len(s.IdentityFiles) != 1 || s.IdentityFiles[0] != "dev.pem" {
		t.Errorf("IdentityFiles = %v", s.IdentityFiles)
	}
	if s.Alias != "dev-api" {
		t.Errorf("Alias = %q, want dev-api", s.Alias)
	}
}

func TestParsePlainCommand(t *testing.T) {
	s, err := ParseCommand(`ssh -i prod.pem ec2-user@203.0.113.10`)
	if err != nil {
		t.Fatal(err)
	}

	if s.HostName != "203.0.113.10" {
		t.Errorf("HostName = %q", s.HostName)
	}
	// An IP is kept whole — truncating it at the first dot would be nonsense.
	if s.Alias != "203.0.113.10" {
		t.Errorf("Alias = %q, want the IP kept whole", s.Alias)
	}
}

func TestParseFlags(t *testing.T) {
	s, err := ParseCommand(`ssh -p 2222 -J bastion -L 8080:localhost:80 -o SetEnv=TERM=xterm ubuntu@10.0.0.5`)
	if err != nil {
		t.Fatal(err)
	}

	if s.Port != 2222 {
		t.Errorf("Port = %d", s.Port)
	}
	if s.ProxyJump != "bastion" {
		t.Errorf("ProxyJump = %q", s.ProxyJump)
	}
	if len(s.Forwards) != 1 || s.Forwards[0] != "-L 8080:localhost:80" {
		t.Errorf("Forwards = %v", s.Forwards)
	}
	// -o options must survive rather than being quietly dropped, which is a
	// documented failure in other importers.
	if s.Extra["SetEnv"] != "TERM=xterm" {
		t.Errorf("Extra = %v", s.Extra)
	}
}

func TestParseJoinedFlagForms(t *testing.T) {
	// ssh accepts -p2222 and -idev.pem just as happily as the separated form.
	s, err := ParseCommand(`ssh -p2222 -idev.pem ec2-user@host.example.com`)
	if err != nil {
		t.Fatal(err)
	}
	if s.Port != 2222 {
		t.Errorf("Port = %d", s.Port)
	}
	if len(s.IdentityFiles) != 1 || s.IdentityFiles[0] != "dev.pem" {
		t.Errorf("IdentityFiles = %v", s.IdentityFiles)
	}
}

func TestParseOldStyleUserFlag(t *testing.T) {
	s, err := ParseCommand(`ssh -l ec2-user host.example.com`)
	if err != nil {
		t.Fatal(err)
	}
	if s.User != "ec2-user" || s.HostName != "host.example.com" {
		t.Errorf("got user=%q host=%q", s.User, s.HostName)
	}
}

func TestParseWindowsPathIsNotMangled(t *testing.T) {
	// The backslashes in a Windows path must survive tokenizing. Treating '\'
	// as a general escape character would silently produce "C:Userssarthakdev.pem".
	s, err := ParseCommand(`ssh -i "C:\Users\sarthak\keys\dev.pem" ec2-user@host.example.com`)
	if err != nil {
		t.Fatal(err)
	}
	want := `C:\Users\sarthak\keys\dev.pem`
	if len(s.IdentityFiles) != 1 || s.IdentityFiles[0] != want {
		t.Fatalf("IdentityFiles = %v, want %q", s.IdentityFiles, want)
	}
}

func TestParseShellOperatorPrefix(t *testing.T) {
	for _, in := range []string{
		`cd ~/Downloads && ssh -i dev.pem ec2-user@host.example.com`,
		`cd /keys; ssh -i dev.pem ec2-user@host.example.com`,
		`/usr/bin/ssh -i dev.pem ec2-user@host.example.com`,
		`ssh.exe -i dev.pem ec2-user@host.example.com`,
	} {
		s, err := ParseCommand(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if s.HostName != "host.example.com" || s.User != "ec2-user" {
			t.Errorf("%q parsed to user=%q host=%q", in, s.User, s.HostName)
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "cd ~/Downloads", "ssh -i dev.pem"} {
		if _, err := ParseCommand(in); err == nil {
			t.Errorf("%q should not have parsed as a connection", in)
		}
	}
}

func TestParseRemoteCommandIsNotTreatedAsAHost(t *testing.T) {
	s, err := ParseCommand(`ssh ec2-user@host.example.com sudo systemctl restart nginx`)
	if err != nil {
		t.Fatal(err)
	}
	if s.HostName != "host.example.com" {
		t.Fatalf("the remote command leaked into the destination: %q", s.HostName)
	}
}

func TestApplyWritesAStanza(t *testing.T) {
	f := sshconfig.ParseBytes([]byte("Host existing\n  HostName 1.1.1.1\n"), "config")

	s, err := ParseCommand(`ssh -i ~/.ssh/keys/dev.pem -p 2222 ec2-user@dev-api.example.com`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(f); err != nil {
		t.Fatal(err)
	}

	got := f.String()
	for _, want := range []string{
		"Host dev-api",
		"HostName dev-api.example.com",
		"User ec2-user",
		"Port 2222",
		"IdentityFile ~/.ssh/keys/dev.pem",
		"IdentitiesOnly yes",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// The pre-existing host must be untouched.
	if !strings.Contains(got, "Host existing") {
		t.Error("Apply disturbed the existing config")
	}
}

func TestApplyRefusesToClobberAnExistingAlias(t *testing.T) {
	f := sshconfig.ParseBytes([]byte("Host dev-api\n  HostName old.example.com\n"), "config")

	s, _ := ParseCommand(`ssh ec2-user@dev-api.example.com`)
	if _, err := s.Apply(f); err == nil {
		t.Fatal("Apply silently overwrote an existing host")
	}
	if !strings.Contains(f.String(), "old.example.com") {
		t.Error("the existing host was damaged by the failed apply")
	}
}

func TestApplyConvertsForwardSyntax(t *testing.T) {
	f := sshconfig.ParseBytes(nil, "config")

	s, err := ParseCommand(`ssh -L 8080:localhost:80 -D 1080 ec2-user@host.example.com`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(f); err != nil {
		t.Fatal(err)
	}

	got := f.String()
	// The command line spells it 8080:localhost:80; the config file spells the
	// same thing "8080 localhost:80".
	if !strings.Contains(got, "LocalForward 8080 localhost:80") {
		t.Errorf("LocalForward not converted to config syntax:\n%s", got)
	}
	if !strings.Contains(got, "DynamicForward 1080") {
		t.Errorf("DynamicForward missing:\n%s", got)
	}
}

func TestSuggestAlias(t *testing.T) {
	cases := map[string]string{
		"dev-api.example.com": "dev-api",
		"203.0.113.10":        "203.0.113.10",
		"bastion":             "bastion",
		"a.b.c.example.com":   "a",
	}
	for in, want := range cases {
		if got := SuggestAlias(in); got != want {
			t.Errorf("SuggestAlias(%q) = %q, want %q", in, got, want)
		}
	}
}
