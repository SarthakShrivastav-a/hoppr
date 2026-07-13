package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetUpdatesInPlaceAndKeepsStyle(t *testing.T) {
	src := "Host prod\n    HostName 10.0.0.1\n    Port = 2222   # non standard\n"
	f := ParseBytes([]byte(src), "test")

	b := f.FindHost("prod")
	if b == nil {
		t.Fatal("prod not found")
	}
	b.Set("Port", "2200")

	got := f.String()
	// The '=' separator and the same-line comment both survive the edit.
	if !strings.Contains(got, "Port = 2200   # non standard") {
		t.Fatalf("Set did not preserve the line's style:\n%s", got)
	}
	if strings.Contains(got, "2222") {
		t.Fatalf("old value still present:\n%s", got)
	}
}

func TestSetAppendsAfterLastDirectiveNotAfterTrailingBlank(t *testing.T) {
	// The blank line belongs to the prod block, so a naive append would put
	// User below it and visually orphan it from the stanza.
	src := "Host prod\n    HostName 10.0.0.1\n\nHost other\n    HostName 10.0.0.2\n"
	f := ParseBytes([]byte(src), "test")

	f.FindHost("prod").Set("User", "ec2-user")

	want := "Host prod\n    HostName 10.0.0.1\n    User ec2-user\n\nHost other\n    HostName 10.0.0.2\n"
	if got := f.String(); got != want {
		t.Fatalf("want:\n%q\ngot:\n%q", want, got)
	}
}

func TestSetCopiesTheBlockIndentStyle(t *testing.T) {
	f := ParseBytes([]byte("Host prod\n\tHostName 10.0.0.1\n"), "test")
	f.FindHost("prod").Set("User", "root")

	if !strings.Contains(f.String(), "\tUser root") {
		t.Fatalf("tab indentation not copied:\n%q", f.String())
	}
}

func TestEditingOneHostLeavesEverythingElseByteIdentical(t *testing.T) {
	f := ParseBytes([]byte(nastyConfig), "test")
	f.FindHost("prod").Set("HostName", "10.0.0.9")

	got := f.String()
	// Everything that is not the line we changed must be untouched.
	want := strings.Replace(nastyConfig, "HostName 10.0.0.1", "HostName 10.0.0.9", 1)
	if got != want {
		t.Fatalf("editing prod disturbed the rest of the file\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestGetAllReadsRepeatableKeywords(t *testing.T) {
	src := "Host a\n  IdentityFile ~/.ssh/one\n  IdentityFile ~/.ssh/two\n"
	f := ParseBytes([]byte(src), "test")

	got := f.FindHost("a").GetAll("IdentityFile")
	if len(got) != 2 || got[0] != "~/.ssh/one" || got[1] != "~/.ssh/two" {
		t.Fatalf("want both identity files, got %v", got)
	}
}

func TestSetAllReplacesRepeatableKeywords(t *testing.T) {
	src := "Host a\n  IdentityFile ~/.ssh/one\n  IdentityFile ~/.ssh/two\n"
	f := ParseBytes([]byte(src), "test")

	f.FindHost("a").SetAll("IdentityFile", []string{"~/.ssh/three"})

	got := f.FindHost("a").GetAll("IdentityFile")
	if len(got) != 1 || got[0] != "~/.ssh/three" {
		t.Fatalf("want a single replaced identity file, got %v", got)
	}
}

func TestUnquoteOnGet(t *testing.T) {
	f := ParseBytes([]byte(`Host a`+"\n"+`  IdentityFile "C:/a b/k.pem"`+"\n"), "test")
	got, ok := f.FindHost("a").Get("IdentityFile")
	if !ok || got != "C:/a b/k.pem" {
		t.Fatalf("want the unquoted path, got %q (ok=%v)", got, ok)
	}
}

func TestSetRequotesValuesWithSpaces(t *testing.T) {
	f := ParseBytes([]byte("Host a\n  HostName x\n"), "test")
	f.FindHost("a").Set("IdentityFile", `C:/Users/A B/k.pem`)

	if !strings.Contains(f.String(), `IdentityFile "C:/Users/A B/k.pem"`) {
		t.Fatalf("a path with spaces was not quoted:\n%s", f.String())
	}
}

func TestAddAndRemoveHost(t *testing.T) {
	f := ParseBytes([]byte("Host a\n  HostName 1.1.1.1\n"), "test")

	b := f.AddHost("b")
	b.Set("HostName", "2.2.2.2")
	b.Set("User", "ubuntu")

	want := "Host a\n  HostName 1.1.1.1\n\nHost b\n    HostName 2.2.2.2\n    User ubuntu\n"
	if got := f.String(); got != want {
		t.Fatalf("AddHost:\nwant %q\ngot  %q", want, got)
	}

	if !f.RemoveHost("b") {
		t.Fatal("RemoveHost reported b was missing")
	}
	if f.FindHost("b") != nil {
		t.Fatal("b survived removal")
	}
	if f.RemoveHost("nope") {
		t.Fatal("RemoveHost invented a host that was never there")
	}
}

func TestDirtyTracking(t *testing.T) {
	f := ParseBytes([]byte(nastyConfig), "test")
	if f.Dirty() {
		t.Fatal("a freshly parsed file reports dirty")
	}
	f.FindHost("prod").Set("HostName", "10.0.0.9")
	if !f.Dirty() {
		t.Fatal("an edited file reports clean")
	}
}

func TestSaveIsAtomicAndPreservesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")

	if err := os.WriteFile(path, []byte(nastyConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	f.FindHost("prod").Set("HostName", "10.0.0.9")
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(nastyConfig, "HostName 10.0.0.1", "HostName 10.0.0.9", 1)
	if string(raw) != want {
		t.Fatalf("saved file does not match\nwant:\n%s\ngot:\n%s", want, raw)
	}
	if f.Dirty() {
		t.Error("file still reports dirty after a successful save")
	}

	// The temp file must not be left lying around next to the config.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".hoppr-config-") {
			t.Errorf("Save leaked a temp file: %s", e.Name())
		}
	}
}
