package sshconfig

import (
	"strings"
	"testing"
)

func TestMetaRoundTrip(t *testing.T) {
	f := ParseBytes([]byte("Host prod\n    HostName 10.0.0.1\n"), "test")
	b := f.FindHost("prod")

	b.SetMeta(Meta{Tags: []string{"aws", "prod"}, Note: "main API box", Color: "red"})

	got := f.String()
	if !strings.Contains(got, "# hoppr: tags=aws,prod; note=main API box; color=red") {
		t.Fatalf("annotation not written as expected:\n%s", got)
	}

	// Reparse from text: the annotation has to survive a trip through disk.
	f2 := ParseBytes([]byte(got), "test")
	m := f2.FindHost("prod").Meta()

	if len(m.Tags) != 2 || m.Tags[0] != "aws" || m.Tags[1] != "prod" {
		t.Errorf("tags did not round trip: %v", m.Tags)
	}
	if m.Note != "main API box" {
		t.Errorf("note did not round trip: %q", m.Note)
	}
	if m.Color != "red" {
		t.Errorf("color did not round trip: %q", m.Color)
	}
}

func TestAnnotationSitsDirectlyUnderTheHostLine(t *testing.T) {
	f := ParseBytes([]byte("Host prod\n    HostName 10.0.0.1\n"), "test")
	f.FindHost("prod").SetMeta(Meta{Tags: []string{"aws"}})

	want := "Host prod\n    # hoppr: tags=aws\n    HostName 10.0.0.1\n"
	if got := f.String(); got != want {
		t.Fatalf("want:\n%q\ngot:\n%q", want, got)
	}
}

func TestSetMetaReplacesRatherThanAccumulates(t *testing.T) {
	f := ParseBytes([]byte("Host prod\n    HostName 10.0.0.1\n"), "test")
	b := f.FindHost("prod")

	b.SetMeta(Meta{Tags: []string{"aws"}})
	b.SetMeta(Meta{Tags: []string{"gcp"}})

	if n := strings.Count(f.String(), "hoppr:"); n != 1 {
		t.Fatalf("want exactly one annotation, got %d:\n%s", n, f.String())
	}
	if !strings.Contains(f.String(), "tags=gcp") {
		t.Fatalf("annotation was not updated:\n%s", f.String())
	}
}

func TestZeroMetaRemovesTheAnnotation(t *testing.T) {
	f := ParseBytes([]byte("Host prod\n    HostName 10.0.0.1\n"), "test")
	b := f.FindHost("prod")

	b.SetMeta(Meta{Tags: []string{"aws"}})
	b.SetMeta(Meta{})

	if strings.Contains(f.String(), "hoppr:") {
		t.Fatalf("clearing the last tag left an empty annotation behind:\n%s", f.String())
	}
	// Back to exactly the original bytes.
	if got := f.String(); got != "Host prod\n    HostName 10.0.0.1\n" {
		t.Fatalf("removal was not clean:\n%q", got)
	}
}

func TestHostWithNoAnnotationHasZeroMeta(t *testing.T) {
	f := ParseBytes([]byte("Host prod\n    HostName 10.0.0.1\n    # just a normal comment\n"), "test")
	if m := f.FindHost("prod").Meta(); !m.IsZero() {
		t.Fatalf("want zero Meta, got %+v", m)
	}
}

func TestMetaIgnoresUnknownFields(t *testing.T) {
	// A config written by a future hoppr must not break this one.
	m := parseMeta("tags=a; futurefield=whatever; note=hi")
	if len(m.Tags) != 1 || m.Tags[0] != "a" || m.Note != "hi" {
		t.Fatalf("unknown field was not skipped cleanly: %+v", m)
	}
}

func TestNoteSemicolonIsNeutralized(t *testing.T) {
	f := ParseBytes([]byte("Host a\n  HostName x\n"), "test")
	b := f.FindHost("a")

	b.SetMeta(Meta{Note: "first; second"})

	// Re-read it: the note must not have split into a bogus second field.
	m := ParseBytes([]byte(f.String()), "test").FindHost("a").Meta()
	if m.Note != "first, second" {
		t.Fatalf("a semicolon in a note corrupted the annotation: %q", m.Note)
	}
}

func TestHasTagIsCaseInsensitive(t *testing.T) {
	m := Meta{Tags: []string{"Prod"}}
	if !m.HasTag("prod") || !m.HasTag("PROD") {
		t.Error("HasTag should be case-insensitive")
	}
	if m.HasTag("dev") {
		t.Error("HasTag matched a tag that is not there")
	}
}
