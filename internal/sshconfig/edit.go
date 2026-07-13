package sshconfig

import "strings"

// defaultIndent is used for directives added to a block that has none of its
// own to copy a style from.
const defaultIndent = "    "

// Get returns the value of the first directive in the block matching key, and
// whether it was present.
//
// "First" rather than "last" because that is what OpenSSH itself does: for most
// keywords the earliest value in a file wins, and later ones are ignored.
func (b *Block) Get(key string) (string, bool) {
	if d := b.find(key); d != nil {
		return unquote(d.Value), true
	}
	return "", false
}

// GetArgs returns the value of key split into arguments.
func (b *Block) GetArgs(key string) []string {
	if d := b.find(key); d != nil {
		return splitArgs(d.Value)
	}
	return nil
}

// GetAll returns the values of every directive matching key. IdentityFile and
// LocalForward are legitimately repeatable, so they need this rather than Get.
func (b *Block) GetAll(key string) []string {
	var out []string
	for _, n := range b.Nodes {
		if d, ok := n.(*Directive); ok && d.Is(key) {
			out = append(out, unquote(d.Value))
		}
	}
	return out
}

func (b *Block) find(key string) *Directive {
	for _, n := range b.Nodes {
		if d, ok := n.(*Directive); ok && d.Is(key) {
			return d
		}
	}
	return nil
}

// Set assigns key to value, updating the existing directive in place if there
// is one and appending a new one otherwise.
//
// Updating in place preserves the keyword's original casing, its separator, and
// any same-line comment. Setting the value a user wrote as "Port = 2222" to
// 2200 yields "Port = 2200", not "Port 2200".
func (b *Block) Set(key, value string) {
	v := quoteArg(value)
	if d := b.find(key); d != nil {
		d.Value = v
		return
	}
	b.appendDirective(key, v)
}

// SetAll replaces every directive matching key with one directive per value.
// Used for repeatable keywords where the whole set is being rewritten.
func (b *Block) SetAll(key string, values []string) {
	b.Unset(key)
	for _, v := range values {
		b.appendDirective(key, quoteArg(v))
	}
}

// Unset removes every directive matching key. It reports how many it removed.
func (b *Block) Unset(key string) int {
	kept := b.Nodes[:0]
	removed := 0
	for _, n := range b.Nodes {
		if d, ok := n.(*Directive); ok && d.Is(key) {
			removed++
			continue
		}
		kept = append(kept, n)
	}
	b.Nodes = kept
	return removed
}

// appendDirective inserts a new directive after the block's last directive,
// rather than at the very end of the block.
//
// A block usually ends with trailing blank lines that separate it from the next
// Host. Appending blindly would put the new keyword after that gap, visually
// orphaning it under the wrong stanza even though OpenSSH would still read it
// correctly.
func (b *Block) appendDirective(key, value string) {
	d := &Directive{Indent: b.indentStyle(), Key: key, Sep: " ", Value: value}

	last := -1
	for i, n := range b.Nodes {
		if _, ok := n.(*Directive); ok {
			last = i
		}
	}
	at := last + 1
	b.Nodes = append(b.Nodes, nil)
	copy(b.Nodes[at+1:], b.Nodes[at:])
	b.Nodes[at] = d
}

// indentStyle copies the indentation the block already uses, so a config
// indented with tabs keeps getting tabs and one indented with four spaces keeps
// getting four spaces.
func (b *Block) indentStyle() string {
	for _, n := range b.Nodes {
		if d, ok := n.(*Directive); ok && d.Indent != "" {
			return d.Indent
		}
	}
	return defaultIndent
}

// SetPatterns rewrites the Host line's patterns, which is how a host is renamed.
func (b *Block) SetPatterns(patterns []string) {
	b.Header.Value = joinArgs(patterns)
}

// unquote strips one layer of surrounding double quotes from a value.
func unquote(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return s[1 : len(s)-1]
	}
	return s
}

// FindHost returns the first Host block whose patterns include alias exactly.
//
// This is a literal match, not a glob match: it answers "which stanza declares
// this alias" for editing purposes. Resolving which stanza OpenSSH would apply
// to a connection is a different question, and lives in resolve.go.
func (f *File) FindHost(alias string) *Block {
	for _, b := range f.HostBlocks() {
		for _, p := range b.Patterns() {
			if p == alias {
				return b
			}
		}
	}
	return nil
}

// AddHost appends a new Host stanza and returns it.
//
// A blank line is inserted first when the file does not already end in one, so
// stanzas never end up glued together.
func (f *File) AddHost(alias string) *Block {
	if f.needsSeparator() {
		f.Nodes = append(f.Nodes, &Blank{})
	}
	b := &Block{
		Header: &Directive{Key: "Host", Sep: " ", Value: quoteArg(alias)},
	}
	f.Nodes = append(f.Nodes, b)
	return b
}

// needsSeparator reports whether a blank line is required before appending a
// new stanza.
func (f *File) needsSeparator() bool {
	if len(f.Nodes) == 0 {
		return false
	}
	switch n := f.Nodes[len(f.Nodes)-1].(type) {
	case *Blank:
		return false
	case *Block:
		// A block owns its trailing trivia, so look inside it.
		if len(n.Nodes) == 0 {
			return true
		}
		_, blank := n.Nodes[len(n.Nodes)-1].(*Blank)
		return !blank
	default:
		return true
	}
}

// RemoveHost deletes the Host stanza declaring alias. It reports whether one
// was found.
func (f *File) RemoveHost(alias string) bool {
	for i, n := range f.Nodes {
		b, ok := n.(*Block)
		if !ok || !b.IsHost() {
			continue
		}
		for _, p := range b.Patterns() {
			if p != alias {
				continue
			}
			f.Nodes = append(f.Nodes[:i], f.Nodes[i+1:]...)
			return true
		}
	}
	return false
}
