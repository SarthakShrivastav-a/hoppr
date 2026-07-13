package keys

import (
	"path/filepath"
	"testing"
)

func TestLocateFindsABareKeyNameInDownloads(t *testing.T) {
	home := tempHome(t)
	want := writeKey(t, filepath.Join(home, "Downloads"), "dev.pem")

	// This is the whole point: the pasted command said `-i dev.pem`, which is
	// relative to a directory we are not standing in.
	got, ok := Locate("dev.pem")
	if !ok {
		t.Fatal("a key sitting in Downloads was not found by name")
	}
	if !samePath(got, want) {
		t.Fatalf("located %s, want %s", got, want)
	}
}

func TestLocatePrefersTheManagedKeyDirectory(t *testing.T) {
	home := tempHome(t)

	// The same file name in two places. The adopted copy should win, otherwise
	// adopting a key and then re-importing its command would drag the config
	// back to the stale copy in Downloads.
	writeKey(t, filepath.Join(home, "Downloads"), "dev.pem")
	managed := writeKey(t, filepath.Join(home, ".ssh", "keys"), "dev.pem")

	got, ok := Locate("dev.pem")
	if !ok {
		t.Fatal("not found")
	}
	if !samePath(got, managed) {
		t.Fatalf("located %s, want the managed copy %s", got, managed)
	}
}

func TestLocateAcceptsAPathThatAlreadyResolves(t *testing.T) {
	home := tempHome(t)
	p := writeKey(t, filepath.Join(home, ".ssh"), "id_ed25519")

	got, ok := Locate(p)
	if !ok || !samePath(got, p) {
		t.Fatalf("an absolute path that exists should be taken as-is, got %q (ok=%v)", got, ok)
	}
}

func TestLocateExpandsTilde(t *testing.T) {
	home := tempHome(t)
	p := writeKey(t, filepath.Join(home, ".ssh", "keys"), "prod.pem")

	got, ok := Locate("~/.ssh/keys/prod.pem")
	if !ok || !samePath(got, p) {
		t.Fatalf("a tilde path should resolve, got %q (ok=%v)", got, ok)
	}
}

func TestLocateDoesNotGuessAtAWrongDirectory(t *testing.T) {
	home := tempHome(t)
	writeKey(t, filepath.Join(home, "Downloads"), "dev.pem")

	// The user named a specific directory and got it wrong. Silently
	// substituting a same-named key from somewhere else would be worse than
	// admitting we could not find it.
	if _, ok := Locate("~/some/other/place/dev.pem"); ok {
		t.Fatal("Locate substituted a different file for an explicit path that does not exist")
	}
}

func TestLocateReportsMissing(t *testing.T) {
	tempHome(t)

	if _, ok := Locate("nonexistent.pem"); ok {
		t.Fatal("a key that is nowhere should not be located")
	}
	if _, ok := Locate(""); ok {
		t.Fatal("an empty path should not be located")
	}
}
