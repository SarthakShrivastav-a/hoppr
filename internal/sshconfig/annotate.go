package sshconfig

import (
	"fmt"
	"sort"
	"strings"
)

// annotationPrefix marks a comment line as hoppr metadata.
//
// Tags, notes, and colors have no home in ssh_config: OpenSSH would reject an
// unknown keyword inside a Host block. The alternative most tools reach for is
// a sidecar database keyed by alias, which then drifts the moment the user
// edits their config by hand — the alias is renamed, the sidecar still points at
// the old one, and the tags silently vanish.
//
// Storing metadata in a comment keeps ssh_config the single source of truth.
// The annotation lives inside the stanza it describes, so it moves, copies, and
// deletes along with it. To OpenSSH, and to every other tool, it is a comment.
const annotationPrefix = "hoppr:"

// Meta is hoppr's per-host metadata.
type Meta struct {
	Tags  []string
	Note  string
	Color string
}

// IsZero reports whether there is nothing worth writing to disk.
func (m Meta) IsZero() bool {
	return len(m.Tags) == 0 && m.Note == "" && m.Color == ""
}

// HasTag reports whether the host carries tag, case-insensitively.
func (m Meta) HasTag(tag string) bool {
	for _, t := range m.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// Meta reads the block's hoppr annotation. A block with no annotation yields a
// zero Meta, not an error.
func (b *Block) Meta() Meta {
	c := b.findAnnotation()
	if c == nil {
		return Meta{}
	}
	return parseMeta(strings.TrimSpace(strings.TrimPrefix(c.Text, annotationPrefix)))
}

// SetMeta writes the block's hoppr annotation, replacing any existing one.
//
// A zero Meta removes the annotation rather than leaving an empty comment
// behind, so clearing the last tag off a host leaves no trace.
func (b *Block) SetMeta(m Meta) {
	c := b.findAnnotation()

	if m.IsZero() {
		if c != nil {
			b.removeNode(c)
		}
		return
	}

	raw := b.indentStyle() + "# " + annotationPrefix + " " + renderMeta(m)
	if c != nil {
		c.Raw = raw
		c.Text = annotationPrefix + " " + renderMeta(m)
		return
	}

	// New annotations go first in the stanza, directly under the Host line,
	// where they read as a label for the host rather than a stray remark.
	c = &Comment{
		Raw:    raw,
		Indent: b.indentStyle(),
		Text:   annotationPrefix + " " + renderMeta(m),
	}
	b.Nodes = append([]Node{c}, b.Nodes...)
}

func (b *Block) findAnnotation() *Comment {
	for _, n := range b.Nodes {
		if c, ok := n.(*Comment); ok && strings.HasPrefix(c.Text, annotationPrefix) {
			return c
		}
	}
	return nil
}

func (b *Block) removeNode(target Node) {
	kept := b.Nodes[:0]
	for _, n := range b.Nodes {
		if n != target {
			kept = append(kept, n)
		}
	}
	b.Nodes = kept
}

// parseMeta reads "tags=prod,aws; note=Main API box; color=red".
//
// Unknown fields are ignored rather than rejected: a config written by a newer
// hoppr must not break an older one.
func parseMeta(s string) Meta {
	var m Meta
	for _, field := range strings.Split(s, ";") {
		key, val, ok := strings.Cut(strings.TrimSpace(field), "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "tags":
			for _, t := range strings.Split(val, ",") {
				if t = strings.TrimSpace(t); t != "" {
					m.Tags = append(m.Tags, t)
				}
			}
		case "note":
			m.Note = val
		case "color":
			m.Color = val
		}
	}
	return m
}

// renderMeta is the inverse of parseMeta.
//
// A ';' in a note would be read back as a field separator, so it is swapped for
// a ',' on the way out. Notes are prose; losing a semicolon is a fair trade for
// not needing an escaping scheme inside a comment.
func renderMeta(m Meta) string {
	var parts []string
	if len(m.Tags) > 0 {
		tags := append([]string(nil), m.Tags...)
		sort.Strings(tags)
		parts = append(parts, "tags="+strings.Join(tags, ","))
	}
	if m.Note != "" {
		parts = append(parts, "note="+strings.ReplaceAll(m.Note, ";", ","))
	}
	if m.Color != "" {
		parts = append(parts, "color="+m.Color)
	}
	return strings.Join(parts, "; ")
}

// String renders a Meta the way it is stored, which is handy in errors.
func (m Meta) String() string { return renderMeta(m) }

var _ fmt.Stringer = Meta{}
