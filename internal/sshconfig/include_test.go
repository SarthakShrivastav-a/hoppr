package sshconfig

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile is a small helper so the include fixtures read as a directory tree.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadTreeFollowsIncludeGlobs(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config")

	writeFile(t, root, "Include conf.d/*.conf\n\nHost root-host\n  HostName 1.1.1.1\n")
	writeFile(t, filepath.Join(dir, "conf.d", "work.conf"), "Host work\n  HostName 2.2.2.2\n")
	writeFile(t, filepath.Join(dir, "conf.d", "personal.conf"), "Host personal\n  HostName 3.3.3.3\n")

	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(tree.Files) != 3 {
		t.Fatalf("want root + 2 includes, got %d files", len(tree.Files))
	}

	for _, alias := range []string{"root-host", "work", "personal"} {
		if _, ok := tree.FindHost(alias); !ok {
			t.Errorf("host %q was not found across the tree", alias)
		}
	}
}

func TestIncludedHostIsSavedBackToItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config")
	work := filepath.Join(dir, "conf.d", "work.conf")

	rootSrc := "Include conf.d/*.conf\n\nHost root-host\n  HostName 1.1.1.1\n"
	writeFile(t, root, rootSrc)
	writeFile(t, work, "Host work\n  HostName 2.2.2.2\n")

	tree, err := LoadTree(root)
	if err != nil {
		t.Fatal(err)
	}

	tb, ok := tree.FindHost("work")
	if !ok {
		t.Fatal("work not found")
	}
	tb.Block.Set("User", "deploy")

	if err := tree.Save(); err != nil {
		t.Fatal(err)
	}

	// The edit lands in conf.d/work.conf...
	got, err := os.ReadFile(work)
	if err != nil {
		t.Fatal(err)
	}
	want := "Host work\n  HostName 2.2.2.2\n  User deploy\n"
	if string(got) != want {
		t.Errorf("include file:\nwant %q\ngot  %q", want, got)
	}

	// ...and the root config is not touched at all.
	gotRoot, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotRoot) != rootSrc {
		t.Errorf("root config was rewritten even though nothing in it changed:\n%q", gotRoot)
	}
}

func TestIncludeCycleTerminates(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "config")
	b := filepath.Join(dir, "b.conf")

	writeFile(t, a, "Include b.conf\nHost a\n  HostName 1.1.1.1\n")
	writeFile(t, b, "Include config\nHost b\n  HostName 2.2.2.2\n")

	// The point of the test is that this returns at all.
	tree, err := LoadTree(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Files) != 2 {
		t.Fatalf("a cycle should read each file exactly once, got %d", len(tree.Files))
	}
}

func TestIncludeMatchingNothingIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config")
	writeFile(t, root, "Include conf.d/*.conf\nHost a\n  HostName 1.1.1.1\n")

	tree, err := LoadTree(root)
	if err != nil {
		t.Fatalf("an Include glob matching nothing should be tolerated: %v", err)
	}
	if len(tree.Files) != 1 {
		t.Fatalf("want just the root file, got %d", len(tree.Files))
	}
}

func TestMissingConfigLoadsAsEmpty(t *testing.T) {
	tree, err := LoadTree(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("a missing config should behave like an empty one: %v", err)
	}
	if len(tree.HostBlocks()) != 0 {
		t.Fatal("a missing config produced hosts out of nowhere")
	}
}
