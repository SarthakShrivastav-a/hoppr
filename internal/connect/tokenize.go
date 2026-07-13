package connect

import (
	"fmt"
	"strings"
)

// tokenize splits a pasted command line into arguments.
//
// It handles both quoting styles, because the command being pasted came from
// either a POSIX shell or a Windows one and hoppr does not get to choose. A
// backslash is only treated as an escape inside double quotes or when it
// precedes a quote — otherwise `C:\Users\me\dev.pem` would lose every
// separator it has, which is precisely the paste we most expect to receive.
func tokenize(s string) ([]string, error) {
	var (
		args []string
		cur  strings.Builder
		has  bool
	)
	flush := func() {
		if has {
			args = append(args, cur.String())
			cur.Reset()
			has = false
		}
	}

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		switch r {
		case ' ', '\t', '\n', '\r':
			flush()

		case '\'':
			// Single quotes are literal all the way to the closing quote.
			i++
			start := i
			for i < len(runes) && runes[i] != '\'' {
				i++
			}
			if i >= len(runes) {
				return nil, fmt.Errorf("unbalanced single quote")
			}
			cur.WriteString(string(runes[start:i]))
			has = true

		case '"':
			i++
			for i < len(runes) && runes[i] != '"' {
				// Inside double quotes a backslash escapes the next character,
				// which is how a quoted Windows path with a trailing separator
				// survives.
				if runes[i] == '\\' && i+1 < len(runes) {
					next := runes[i+1]
					if next == '"' || next == '\\' {
						cur.WriteRune(next)
						i += 2
						continue
					}
				}
				cur.WriteRune(runes[i])
				i++
			}
			if i >= len(runes) {
				return nil, fmt.Errorf("unbalanced double quote")
			}
			has = true

		case '\\':
			// Outside quotes, a backslash only escapes a quote character.
			// Everywhere else it is a path separator and must be kept.
			if i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\'') {
				cur.WriteRune(runes[i+1])
				i++
			} else {
				cur.WriteRune(r)
			}
			has = true

		case '&', ';', '|':
			// A shell operator joining `cd ... && ssh ...`. Treat it as a
			// separator; trimToSSH then discards everything before the ssh.
			flush()

		default:
			cur.WriteRune(r)
			has = true
		}
	}
	flush()
	return args, nil
}
