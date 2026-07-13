package forward

import (
	"testing"
	"time"
)

func TestParseLocalForward(t *testing.T) {
	s, err := ParseSpec(Local, "8080:localhost:80")
	if err != nil {
		t.Fatal(err)
	}
	if s.BindPort != 8080 || s.DestHost != "localhost" || s.DestPort != 80 {
		t.Fatalf("got %+v", s)
	}
	// An empty bind address means localhost. Binding to 0.0.0.0 would expose
	// the tunnel to the whole network, and nobody should get that by accident.
	if s.BindAddr != "" {
		t.Errorf("BindAddr = %q, want empty (localhost)", s.BindAddr)
	}
	if s.Arg() != "8080:localhost:80" {
		t.Errorf("Arg() = %q", s.Arg())
	}
	if s.Flag() != "-L" {
		t.Errorf("Flag() = %q", s.Flag())
	}
}

func TestParseExplicitBindAddress(t *testing.T) {
	s, err := ParseSpec(Local, "127.0.0.1:8080:db.internal:5432")
	if err != nil {
		t.Fatal(err)
	}
	if s.BindAddr != "127.0.0.1" || s.BindPort != 8080 {
		t.Fatalf("bind side parsed wrong: %+v", s)
	}
	if s.DestHost != "db.internal" || s.DestPort != 5432 {
		t.Fatalf("destination parsed wrong: %+v", s)
	}
	if s.Arg() != "127.0.0.1:8080:db.internal:5432" {
		t.Errorf("Arg() = %q", s.Arg())
	}
}

func TestParseDynamicForward(t *testing.T) {
	s, err := ParseSpec(Dynamic, "1080")
	if err != nil {
		t.Fatal(err)
	}
	if s.BindPort != 1080 {
		t.Fatalf("got %+v", s)
	}
	if s.Flag() != "-D" || s.Arg() != "1080" {
		t.Errorf("Flag()=%q Arg()=%q", s.Flag(), s.Arg())
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	bad := []struct {
		kind Kind
		spec string
	}{
		{Local, "8080"},                  // missing destination
		{Local, "8080:localhost"},        // missing destination port
		{Local, "notaport:localhost:80"}, // non-numeric
		{Local, "99999:localhost:80"},    // out of range
		{Local, "8080:localhost:0"},      // out of range
		{Dynamic, "1080:localhost:80"},   // dynamic takes only a port
		{Dynamic, "nope"},
	}
	for _, tc := range bad {
		if _, err := ParseSpec(tc.kind, tc.spec); err == nil {
			t.Errorf("%s %q should not have parsed", tc.kind, tc.spec)
		}
	}
}

func TestConfigKeyword(t *testing.T) {
	local, _ := ParseSpec(Local, "8080:localhost:80")
	kw, args := local.ConfigKeyword()
	if kw != "LocalForward" || len(args) != 2 || args[0] != "8080" || args[1] != "localhost:80" {
		t.Errorf("LocalForward: got %q %v", kw, args)
	}

	remote, _ := ParseSpec(Remote, "9000:localhost:3000")
	kw, args = remote.ConfigKeyword()
	if kw != "RemoteForward" || args[0] != "9000" || args[1] != "localhost:3000" {
		t.Errorf("RemoteForward: got %q %v", kw, args)
	}

	dyn, _ := ParseSpec(Dynamic, "1080")
	kw, args = dyn.ConfigKeyword()
	if kw != "DynamicForward" || len(args) != 1 || args[0] != "1080" {
		t.Errorf("DynamicForward: got %q %v", kw, args)
	}
}

func TestJitterStaysInRange(t *testing.T) {
	// Jitter must spread reconnects out without ever exceeding the backoff, or
	// the cap stops meaning anything.
	d := 8 * time.Second
	for i := 0; i < 200; i++ {
		j := jitter(d)
		if j < d/2 || j > d {
			t.Fatalf("jitter(%s) = %s, want between %s and %s", d, j, d/2, d)
		}
	}
	if jitter(0) != 0 {
		t.Error("jitter(0) should be 0")
	}
}

func TestSpecStringIsReadable(t *testing.T) {
	local, _ := ParseSpec(Local, "5432:localhost:5432")
	if got := local.String(); got != "localhost:5432 → localhost:5432" {
		t.Errorf("String() = %q", got)
	}

	dyn, _ := ParseSpec(Dynamic, "1080")
	if got := dyn.String(); got != "SOCKS proxy on localhost:1080" {
		t.Errorf("String() = %q", got)
	}
}
