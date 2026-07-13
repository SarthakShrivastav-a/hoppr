// Package sshconfig parses and rewrites OpenSSH client configuration files.
//
// The defining constraint of this package is that a parse/render round trip is
// byte-for-byte lossless. A user's ~/.ssh/config is hand-maintained territory:
// it holds comments, deliberate blank lines, vendor blocks written by other
// tools, and keywords this package has never heard of. hoppr must be able to
// change one HostName without disturbing any of it.
//
// Every line therefore keeps the raw text it was written with. Rendering an
// untouched node replays that text verbatim; only nodes that were actually
// edited are regenerated, and even then the original indentation and key/value
// separator are preserved.
package sshconfig

import "strings"

// Node is a single lexical element of a config file: a blank line, a comment,
// a keyword directive, or a Host/Match block.
type Node interface {
	render(*strings.Builder)
}

// Blank is an empty or whitespace-only line.
type Blank struct {
	Raw string
}

func (b *Blank) render(sb *strings.Builder) {
	sb.WriteString(b.Raw)
	sb.WriteByte('\n')
}

// Comment is a line whose first non-whitespace character is '#'.
//
// Text is the comment body with the leading '#' and surrounding space removed,
// which is what the annotation layer reads hoppr metadata out of.
type Comment struct {
	Raw    string
	Indent string
	Text   string
}

func (c *Comment) render(sb *strings.Builder) {
	sb.WriteString(c.Raw)
	sb.WriteByte('\n')
}

// Directive is a "Keyword value" line.
//
// Sep holds the exact bytes between the keyword and the value — OpenSSH accepts
// whitespace, "=", or whitespace around "=", and users have opinions. Trail
// holds anything after the value, which is where a same-line comment lives.
type Directive struct {
	Indent string
	Key    string
	Sep    string
	Value  string
	Trail  string
}

func (d *Directive) render(sb *strings.Builder) {
	sb.WriteString(d.Indent)
	sb.WriteString(d.Key)
	sb.WriteString(d.Sep)
	sb.WriteString(d.Value)
	sb.WriteString(d.Trail)
	sb.WriteByte('\n')
}

// Is reports whether the directive's keyword matches key. OpenSSH keywords are
// case-insensitive, so "hostname", "HostName", and "HOSTNAME" are one keyword.
func (d *Directive) Is(key string) bool {
	return strings.EqualFold(d.Key, key)
}

// Block is a Host or Match stanza and everything that follows it, up to the
// next Host/Match line or end of file.
//
// Trailing blank lines and comments after the last directive are kept inside
// the block they follow. That is arbitrary but it is consistent, and it is what
// makes rendering lossless without a separate "orphan trivia" list.
type Block struct {
	Header *Directive
	Nodes  []Node
}

func (b *Block) render(sb *strings.Builder) {
	b.Header.render(sb)
	for _, n := range b.Nodes {
		n.render(sb)
	}
}

// IsHost reports whether this block is a Host stanza rather than a Match one.
func (b *Block) IsHost() bool { return b.Header.Is("Host") }

// Patterns returns the host patterns on the Host line, e.g. ["web-*", "!web-9"].
func (b *Block) Patterns() []string {
	return splitArgs(b.Header.Value)
}

// File is one parsed config file.
//
// TrailingNewline records whether the source ended with a newline so that a
// file lacking one does not silently gain one on the next write.
type File struct {
	Path            string
	Nodes           []Node
	TrailingNewline bool
}

// Blocks returns every Host and Match stanza in file order.
func (f *File) Blocks() []*Block {
	var out []*Block
	for _, n := range f.Nodes {
		if b, ok := n.(*Block); ok {
			out = append(out, b)
		}
	}
	return out
}

// HostBlocks returns only the Host stanzas, skipping Match.
func (f *File) HostBlocks() []*Block {
	var out []*Block
	for _, b := range f.Blocks() {
		if b.IsHost() {
			out = append(out, b)
		}
	}
	return out
}
