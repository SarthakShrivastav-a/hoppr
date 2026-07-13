package sshconfig

import (
	"io"
	"os"
	"strings"
)

// Parse reads a config file from r. path is recorded on the File for later
// writes and for error messages; it is not opened.
//
// The whole file is read into memory rather than streamed line by line. Config
// files are kilobytes at most, and it is the only way to tell a file that ends
// with a newline from one that does not — a distinction bufio.Scanner throws
// away, and one that a lossless round trip has to preserve.
func Parse(r io.Reader, path string) (*File, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseBytes(raw, path), nil
}

// ParseBytes parses config source that is already in memory.
func ParseBytes(raw []byte, path string) *File {
	f := &File{Path: path}

	text := string(raw)
	if text == "" {
		f.TrailingNewline = true
		return f
	}

	f.TrailingNewline = strings.HasSuffix(text, "\n")
	body := strings.TrimSuffix(text, "\n")
	lines := strings.Split(body, "\n")

	var cur *Block // block currently being filled; nil means top level
	add := func(n Node) {
		if cur != nil {
			cur.Nodes = append(cur.Nodes, n)
			return
		}
		f.Nodes = append(f.Nodes, n)
	}

	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")

		indent, key, sep, value, trail, ok := splitLine(line)
		if !ok {
			if strings.TrimSpace(line) == "" {
				add(&Blank{Raw: line})
			} else {
				ind := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
				add(&Comment{Raw: line, Indent: ind, Text: body})
			}
			continue
		}

		d := &Directive{Indent: indent, Key: key, Sep: sep, Value: value, Trail: trail}

		// Host and Match open a new block; every node after them belongs to it
		// until the next Host/Match line or end of file.
		if d.Is("Host") || d.Is("Match") {
			cur = &Block{Header: d}
			f.Nodes = append(f.Nodes, cur)
			continue
		}
		add(d)
	}
	return f
}

// Load parses the config file at path.
//
// A missing file is not an error: it yields an empty File. A machine with no
// ~/.ssh/config yet should behave exactly like one with an empty config, so
// that `hoppr add` on a fresh box just works.
func Load(path string) (*File, error) {
	fh, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{Path: path, TrailingNewline: true}, nil
		}
		return nil, err
	}
	defer fh.Close()
	return Parse(fh, path)
}
