package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

func hostsFrom(t *testing.T, src string) ([]*host.Host, *sshconfig.File) {
	t.Helper()
	f := sshconfig.ParseBytes([]byte(src), "config")
	tree := &sshconfig.Tree{Root: f, Files: []*sshconfig.File{f}}
	return host.Load(tree), f
}

// find returns the first finding whose title contains substr.
func find(r *Report, substr string) (Finding, bool) {
	for _, f := range r.Findings {
		if strings.Contains(f.Title, substr) {
			return f, true
		}
	}
	return Finding{}, false
}

func TestDetectsDuplicateAliases(t *testing.T) {
	hosts, _ := hostsFrom(t, "Host prod\n  HostName 1.1.1.1\n\nHost prod\n  HostName 2.2.2.2\n")

	r := &Report{}
	checkDuplicateAliases(r, hosts)

	f, ok := find(r, "declared more than once")
	if !ok {
		t.Fatal("a duplicate alias was not reported")
	}
	if f.Severity != Warn {
		t.Errorf("severity = %v, want warn", f.Severity)
	}
	if !strings.Contains(f.Detail, "applies the first match") {
		t.Errorf("the detail should explain the consequence, got %q", f.Detail)
	}
}

func TestDetectsMissingIdentityFile(t *testing.T) {
	hosts, _ := hostsFrom(t, "Host prod\n  HostName 1.1.1.1\n  IdentityFile ~/.ssh/keys/gone.pem\n")

	r := &Report{}
	checkMissingKeys(r, hosts)

	f, ok := find(r, "identity file not found")
	if !ok {
		t.Fatal("a missing identity file was not reported")
	}
	if f.Severity != Fail {
		t.Errorf("severity = %v, want fail", f.Severity)
	}
	// The point of the check is that ssh's own error never mentions the file.
	if !strings.Contains(f.Detail, "publickey") {
		t.Errorf("the detail should connect this to the error the user will see, got %q", f.Detail)
	}
}

func TestDetectsMissingIdentitiesOnly(t *testing.T) {
	hosts, f := hostsFrom(t, "Host prod\n  HostName 1.1.1.1\n  IdentityFile ~/.ssh/keys/prod.pem\n")

	r := &Report{}
	checkIdentitiesOnly(r, hosts)

	finding, ok := find(r, "IdentitiesOnly")
	if !ok {
		t.Fatal("a host naming a key without IdentitiesOnly was not reported")
	}
	if !finding.Fixable() {
		t.Fatal("this finding should be automatically fixable")
	}
	if !strings.Contains(finding.Detail, "Too many authentication failures") {
		t.Errorf("the detail should name the error this produces, got %q", finding.Detail)
	}

	// Applying the fix should set the keyword. Point the file somewhere
	// writable first, since the fix saves.
	dir := t.TempDir()
	f.Path = filepath.Join(dir, "config")
	if err := finding.Fix(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.String(), "IdentitiesOnly yes") {
		t.Errorf("the fix did not set IdentitiesOnly:\n%s", f.String())
	}

	raw, err := os.ReadFile(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "IdentitiesOnly yes") {
		t.Error("the fix did not persist to disk")
	}
}

func TestIdentitiesOnlyAlreadySetIsNotReported(t *testing.T) {
	hosts, _ := hostsFrom(t, "Host prod\n  HostName 1.1.1.1\n  IdentityFile ~/k.pem\n  IdentitiesOnly yes\n")

	r := &Report{}
	checkIdentitiesOnly(r, hosts)

	if _, ok := find(r, "IdentitiesOnly"); ok {
		t.Error("a host that already sets IdentitiesOnly should not be flagged")
	}
}

func TestHostWithNoKeyIsNotFlaggedForIdentitiesOnly(t *testing.T) {
	// Without an explicit IdentityFile, offering agent keys is the whole point.
	hosts, _ := hostsFrom(t, "Host prod\n  HostName 1.1.1.1\n")

	r := &Report{}
	checkIdentitiesOnly(r, hosts)

	if _, ok := find(r, "IdentitiesOnly"); ok {
		t.Error("a host that names no key should not be flagged")
	}
}

func TestDetectsBOMAndCRLF(t *testing.T) {
	dir := t.TempDir()

	bom := filepath.Join(dir, "bom")
	if err := os.WriteFile(bom, append([]byte{0xEF, 0xBB, 0xBF}, []byte("Host a\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Report{}
	checkConfigLineEndings(r, bom)
	if _, ok := find(r, "byte-order mark"); !ok {
		t.Error("a BOM at the start of ssh_config was not reported")
	}

	crlf := filepath.Join(dir, "crlf")
	if err := os.WriteFile(crlf, []byte("Host a\r\n  HostName 1.1.1.1\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r = &Report{}
	checkConfigLineEndings(r, crlf)
	if _, ok := find(r, "CRLF"); !ok {
		t.Error("CRLF line endings were not reported")
	}
}

func TestCleanConfigProducesNoProblems(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	if err := os.WriteFile(cfg, []byte("Host prod\n  HostName 1.1.1.1\n  User ec2-user\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts, _ := hostsFrom(t, "Host prod\n  HostName 1.1.1.1\n  User ec2-user\n")

	r := &Report{}
	checkConfigLineEndings(r, cfg)
	checkDuplicateAliases(r, hosts)
	checkMissingKeys(r, hosts)
	checkIdentitiesOnly(r, hosts)

	if got := r.Problems(); len(got) != 0 {
		t.Fatalf("a clean config should raise nothing, got %v", got)
	}
}
