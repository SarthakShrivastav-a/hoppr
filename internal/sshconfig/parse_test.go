package sshconfig

import (
	"strings"
	"testing"
)

// nastyConfig exercises every shape hoppr must not mangle: tabs and spaces,
// "=" separators, same-line comments, quoted values with spaces and a '#',
// unknown keywords, a Match block, global directives before any Host, and a
// missing trailing newline.
const nastyConfig = `# managed by hand, do not reformat
Include ~/.ssh/conf.d/*.conf

Host   bastion   jump-*
	HostName=bastion.example.com
	User   ec2-user
	IdentityFile "C:/Users/A B/keys/my key.pem"   # has spaces
	SomeFutureKeyword yes

Host prod
    HostName 10.0.0.1
    Port = 2222
    # a comment inside the block
    ProxyJump bastion

Match host *.internal
    ForwardAgent yes

Host quoted
    RemoteCommand "echo #notacomment"`

func TestRoundTripIsLossless(t *testing.T) {
	f := ParseBytes([]byte(nastyConfig), "test")
	if got := f.String(); got != nastyConfig {
		t.Fatalf("round trip changed the file\n--- want ---\n%q\n--- got ---\n%q", nastyConfig, got)
	}
}

func TestRoundTripPreservesTrailingNewline(t *testing.T) {
	for _, src := range []string{"Host a\n", "Host a", "", "\n\n"} {
		f := ParseBytes([]byte(src), "test")
		if got := f.String(); got != src {
			t.Errorf("round trip of %q produced %q", src, got)
		}
	}
}

func TestParseBlocks(t *testing.T) {
	f := ParseBytes([]byte(nastyConfig), "test")

	hosts := f.HostBlocks()
	if len(hosts) != 3 {
		t.Fatalf("want 3 Host blocks, got %d", len(hosts))
	}
	if len(f.Blocks()) != 4 {
		t.Fatalf("want 4 blocks including Match, got %d", len(f.Blocks()))
	}

	// The Match block must not be mistaken for a Host.
	for _, b := range hosts {
		if b.Header.Is("Match") {
			t.Error("a Match block leaked into HostBlocks")
		}
	}

	want := [][]string{{"bastion", "jump-*"}, {"prod"}, {"quoted"}}
	for i, b := range hosts {
		got := b.Patterns()
		if strings.Join(got, ",") != strings.Join(want[i], ",") {
			t.Errorf("block %d: want patterns %v, got %v", i, want[i], got)
		}
	}
}

func TestParseSeparatorsAndComments(t *testing.T) {
	f := ParseBytes([]byte(nastyConfig), "test")
	bastion := f.HostBlocks()[0]

	var hostname, identity *Directive
	for _, n := range bastion.Nodes {
		d, ok := n.(*Directive)
		if !ok {
			continue
		}
		switch {
		case d.Is("HostName"):
			hostname = d
		case d.Is("IdentityFile"):
			identity = d
		}
	}

	if hostname == nil {
		t.Fatal("HostName not found in the bastion block")
	}
	// Written as "HostName=bastion.example.com" — the '=' must survive.
	if hostname.Sep != "=" {
		t.Errorf("want the '=' separator preserved, got %q", hostname.Sep)
	}
	if hostname.Value != "bastion.example.com" {
		t.Errorf("want value bastion.example.com, got %q", hostname.Value)
	}

	if identity == nil {
		t.Fatal("IdentityFile not found in the bastion block")
	}
	if identity.Trail != "   # has spaces" {
		t.Errorf("same-line comment not preserved, got %q", identity.Trail)
	}
}

func TestHashInsideQuotesIsNotAComment(t *testing.T) {
	f := ParseBytes([]byte(nastyConfig), "test")
	quoted := f.HostBlocks()[2]

	for _, n := range quoted.Nodes {
		if d, ok := n.(*Directive); ok && d.Is("RemoteCommand") {
			if d.Value != `"echo #notacomment"` {
				t.Fatalf("a '#' inside quotes was treated as a comment: value=%q trail=%q", d.Value, d.Trail)
			}
			if d.Trail != "" {
				t.Fatalf("want empty trail, got %q", d.Trail)
			}
			return
		}
	}
	t.Fatal("RemoteCommand not found")
}

func TestSplitArgs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`a b c`, []string{"a", "b", "c"}},
		{`  a   b  `, []string{"a", "b"}},
		{`"one arg"`, []string{"one arg"}},
		{`a "two words" c`, []string{"a", "two words", "c"}},
		{`"C:/Users/A B/k.pem"`, []string{"C:/Users/A B/k.pem"}},
		{``, nil},
	}
	for _, tc := range cases {
		got := splitArgs(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitArgs(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitArgs(%q) = %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
}

func TestQuoteArg(t *testing.T) {
	cases := map[string]string{
		"plain":         "plain",
		"has space":     `"has space"`,
		`"already"`:     `"already"`,
		"":              `""`,
		"C:/keys/k.pem": "C:/keys/k.pem",
		"C:/a b/k.pem":  `"C:/a b/k.pem"`,
	}
	for in, want := range cases {
		if got := quoteArg(in); got != want {
			t.Errorf("quoteArg(%q) = %q, want %q", in, got, want)
		}
	}
}
