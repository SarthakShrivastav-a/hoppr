package keys

import (
	"bytes"
	"crypto/ed25519"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

// testKey is a real, throwaway ed25519 private key in OpenSSH format.
//
// It is generated from a fixed seed rather than pasted in as a literal, so it
// is genuinely well-formed — a hand-mangled PEM body parses as far as the
// header and then fails deep inside the cipher with something unhelpful like
// "padding not as expected". It protects nothing and is used nowhere.
var testKey = func() []byte {
	seed := bytes.Repeat([]byte{0x7}, ed25519.SeedSize)
	block, err := ssh.MarshalPrivateKey(ed25519.NewKeyFromSeed(seed), "test@hoppr.local")
	if err != nil {
		panic(err)
	}
	return pem.EncodeToMemory(block)
}()

func writeKey(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, testKey, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// tempHome gives a test its own HOME so that Adopt writes into a sandbox rather
// than the real ~/.ssh.
//
// The ACL reset is not incidental. Hardening a key on Windows strips inherited
// permissions from the file and its directory, and a temp tree in that state
// cannot be deleted — t.TempDir's own cleanup fails with "Access is denied".
// Cleanups run last-registered-first, so this one lands before it.
func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Cleanup(func() { resetACLs(home) })
	return home
}

// resetACLs restores inheritable permissions across dir. It is a no-op anywhere
// that is not Windows, where the mode bits we set are the whole story.
func resetACLs(dir string) {
	if runtime.GOOS != "windows" {
		return
	}
	_ = exec.Command("icacls", dir, "/reset", "/T", "/C", "/Q").Run()
}

func TestLooksLikePrivateKeyDetectsByContentNotExtension(t *testing.T) {
	// A key from ssh-keygen has no extension; one from AWS is a .pem; a .pem
	// might also be a certificate. Only the PEM header is reliable.
	if !looksLikePrivateKey(testKey) {
		t.Error("a real private key was not recognized")
	}
	if looksLikePrivateKey([]byte("-----BEGIN CERTIFICATE-----\nMIIB...\n")) {
		t.Error("a certificate was mistaken for a private key")
	}
	if looksLikePrivateKey([]byte("just some text")) {
		t.Error("plain text was mistaken for a private key")
	}
	if looksLikePrivateKey([]byte("ssh-ed25519 AAAAC3Nz... me@box")) {
		t.Error("a public key was mistaken for a private key")
	}
}

func TestLoadRejectsNonKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(p, []byte("shopping list"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(p); err != ErrNotAKey {
		t.Fatalf("want ErrNotAKey, got %v", err)
	}
}

func TestScanFindsKeysAndIgnoresEverythingElse(t *testing.T) {
	root := t.TempDir()

	writeKey(t, root, "prod.pem")
	writeKey(t, filepath.Join(root, "aws-keys"), "dev.pem")
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A .pub must not be reported as a key in its own right.
	if err := os.WriteFile(filepath.Join(root, "prod.pem.pub"), []byte("ssh-ed25519 AAAA me@box"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A skipped directory must not be descended into.
	writeKey(t, filepath.Join(root, "node_modules", "pkg"), "sneaky.pem")

	found := Scan([]string{root})

	var names []string
	for _, k := range found {
		names = append(names, k.Name())
	}
	if len(found) != 2 {
		t.Fatalf("want prod.pem and dev.pem, got %v", names)
	}
	for _, n := range names {
		if n == "sneaky.pem" {
			t.Error("the scan descended into node_modules")
		}
	}
}

func TestScanReadsFingerprintAndType(t *testing.T) {
	root := t.TempDir()
	writeKey(t, root, "prod.pem")

	found := Scan([]string{root})
	if len(found) != 1 {
		t.Fatalf("want 1 key, got %d", len(found))
	}
	k := found[0]

	if k.Type != "ssh-ed25519" {
		t.Errorf("Type = %q, want ssh-ed25519", k.Type)
	}
	if !strings.HasPrefix(k.Fingerprint, "SHA256:") {
		t.Errorf("Fingerprint = %q, want a SHA256: fingerprint", k.Fingerprint)
	}
	if k.Encrypted {
		t.Error("an unencrypted key was reported as encrypted")
	}
}

// hostsFrom builds a host list from config text, for the index and adopt tests.
func hostsFrom(t *testing.T, src string) ([]*host.Host, *sshconfig.File) {
	t.Helper()
	f := sshconfig.ParseBytes([]byte(src), "config")
	tree := &sshconfig.Tree{Root: f, Files: []*sshconfig.File{f}}
	return host.Load(tree), f
}

func TestIndexMapsKeysToTheHostsUsingThem(t *testing.T) {
	root := t.TempDir()
	used := writeKey(t, root, "prod.pem")
	orphan := writeKey(t, root, "ancient.pem")

	hosts, _ := hostsFrom(t, "Host prod\n  HostName 1.1.1.1\n  IdentityFile "+filepath.ToSlash(used)+"\n")
	ks := []*Key{mustLoad(t, used), mustLoad(t, orphan)}

	idx := Index(ks, hosts)
	if len(idx) != 2 {
		t.Fatalf("want 2 entries, got %d", len(idx))
	}

	for _, u := range idx {
		switch u.Key.Name() {
		case "prod.pem":
			if u.Orphaned() {
				t.Error("prod.pem is referenced by a host but was reported orphaned")
			}
			if len(u.Hosts) != 1 || u.Hosts[0].Alias != "prod" {
				t.Errorf("prod.pem should map to host prod, got %v", u.Hosts)
			}
		case "ancient.pem":
			if !u.Orphaned() {
				t.Error("ancient.pem is referenced by nothing and should be an orphan")
			}
		}
	}
}

func TestMissingFindsConfigPointingAtKeysThatAreGone(t *testing.T) {
	hosts, _ := hostsFrom(t, "Host prod\n  HostName 1.1.1.1\n  IdentityFile ~/.ssh/keys/vanished.pem\n")

	missing := Missing(hosts)
	if len(missing) != 1 {
		t.Fatalf("want 1 missing key, got %v", missing)
	}
	if hs := missing["~/.ssh/keys/vanished.pem"]; len(hs) != 1 || hs[0].Alias != "prod" {
		t.Errorf("the missing key should be attributed to host prod, got %v", missing)
	}
}

func TestAdoptRepointsTheConfigAndSetsIdentitiesOnly(t *testing.T) {
	// Pretend Downloads and the managed key dir both live under a temp HOME.
	home := tempHome(t)

	downloads := filepath.Join(home, "Downloads")
	src := writeKey(t, downloads, "prod.pem")

	hosts, f := hostsFrom(t, "Host prod\n  HostName 1.1.1.1\n  IdentityFile "+filepath.ToSlash(src)+"\n")
	k := mustLoad(t, src)

	res, err := Adopt(k, hosts)
	if err != nil {
		t.Fatal(err)
	}

	// The key moved out of Downloads...
	if fileExists(src) {
		t.Error("the original key is still sitting in Downloads")
	}
	want := filepath.Join(home, ".ssh", "keys", "prod.pem")
	if !samePath(res.To, want) {
		t.Errorf("adopted to %s, want %s", res.To, want)
	}
	if !fileExists(want) {
		t.Fatal("the key is not at its new home")
	}

	// ...the config points at its new home, in portable tilde form...
	got := f.String()
	if !strings.Contains(got, "IdentityFile ~/.ssh/keys/prod.pem") {
		t.Errorf("config was not repointed in tilde form:\n%s", got)
	}
	if strings.Contains(got, "Downloads") {
		t.Errorf("config still references the old Downloads path:\n%s", got)
	}

	// ...and IdentitiesOnly is set, so ssh does not exhaust its auth attempts
	// offering every agent key before the right one.
	if !strings.Contains(got, "IdentitiesOnly yes") {
		t.Errorf("IdentitiesOnly was not set:\n%s", got)
	}

	if len(res.Rewritten) != 1 || res.Rewritten[0] != "prod" {
		t.Errorf("want host prod reported as rewritten, got %v", res.Rewritten)
	}
}

func TestAdoptDoesNotClobberAnExistingKeyOfTheSameName(t *testing.T) {
	home := tempHome(t)

	// A key already lives in the managed directory under this name.
	existing := writeKey(t, filepath.Join(home, ".ssh", "keys"), "id_rsa.pem")
	src := writeKey(t, filepath.Join(home, "Downloads"), "id_rsa.pem")

	res, err := Adopt(mustLoad(t, src), nil)
	if err != nil {
		t.Fatal(err)
	}

	if samePath(res.To, existing) {
		t.Fatal("adopt overwrote an unrelated key that shared its name")
	}
	if !fileExists(existing) {
		t.Fatal("the pre-existing key was destroyed")
	}
	if !fileExists(res.To) {
		t.Fatal("the adopted key is missing")
	}
}

func TestStrayDetection(t *testing.T) {
	home := tempHome(t)

	inDownloads := mustLoad(t, writeKey(t, filepath.Join(home, "Downloads"), "a.pem"))
	inSSH := mustLoad(t, writeKey(t, filepath.Join(home, ".ssh"), "b.pem"))
	inKeyDir := mustLoad(t, writeKey(t, filepath.Join(home, ".ssh", "keys"), "c.pem"))

	if !inDownloads.Stray() {
		t.Error("a key in Downloads is a stray")
	}
	if inSSH.Stray() {
		t.Error("a key in ~/.ssh is not a stray")
	}
	if inKeyDir.Stray() {
		t.Error("a key in the managed key dir is not a stray")
	}
}

func mustLoad(t *testing.T, p string) *Key {
	t.Helper()
	k, err := Load(p)
	if err != nil {
		t.Fatalf("load %s: %v", p, err)
	}
	return k
}
