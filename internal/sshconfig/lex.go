package sshconfig

import "strings"

// splitArgs splits an ssh_config value into arguments the way OpenSSH does:
// whitespace-separated, with double quotes grouping a single argument.
//
// OpenSSH has no escape character inside quotes — a quote simply toggles
// quoting — so this deliberately does not implement backslash escapes.
func splitArgs(s string) []string {
	var (
		args    []string
		cur     strings.Builder
		quoted  bool
		started bool
	)
	flush := func() {
		if started {
			args = append(args, cur.String())
			cur.Reset()
			started = false
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			// A bare "" is a real, empty argument.
			started = true
		case !quoted && (r == ' ' || r == '\t'):
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	flush()
	return args
}

// quoteArg wraps s in double quotes if it contains whitespace, so that values
// like a Windows key path with spaces survive a round trip through OpenSSH.
func quoteArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t") {
		return s
	}
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) > 1 {
		return s // already quoted
	}
	return `"` + s + `"`
}

// joinArgs renders args back into a single value string, quoting as needed.
func joinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = quoteArg(a)
	}
	return strings.Join(parts, " ")
}

// splitLine breaks a raw config line into its indent, keyword, separator,
// value, and trailing text. It reports ok=false for blank and comment lines,
// which the parser handles separately.
//
// The separator is captured verbatim rather than normalized because OpenSSH
// accepts "Key value", "Key=value", and "Key = value", and rewriting a user's
// chosen style would be exactly the kind of churn this package exists to avoid.
func splitLine(line string) (indent, key, sep, value, trail string, ok bool) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	indent = line[:i]
	rest := line[i:]
	if rest == "" || rest[0] == '#' {
		return "", "", "", "", "", false
	}

	// Keyword runs until whitespace or '='.
	j := 0
	for j < len(rest) && rest[j] != ' ' && rest[j] != '\t' && rest[j] != '=' {
		j++
	}
	key = rest[:j]
	if key == "" {
		return "", "", "", "", "", false
	}
	rest = rest[j:]

	// Separator is the whitespace and at most one '=' that follows.
	k := 0
	sawEq := false
	for k < len(rest) {
		c := rest[k]
		if c == ' ' || c == '\t' {
			k++
			continue
		}
		if c == '=' && !sawEq {
			sawEq = true
			k++
			continue
		}
		break
	}
	sep = rest[:k]
	rest = rest[k:]

	// A keyword with no value at all, e.g. a stray "Compression".
	if rest == "" {
		return indent, key, sep, "", "", true
	}

	value, trail = splitValueAndTrail(rest)
	return indent, key, sep, value, trail, true
}

// splitValueAndTrail separates the value from a trailing same-line comment.
//
// A '#' inside double quotes is part of the value, not the start of a comment.
func splitValueAndTrail(rest string) (value, trail string) {
	quoted := false
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '"':
			quoted = !quoted
		case '#':
			if !quoted {
				return strings.TrimRight(rest[:i], " \t"), rest[len(strings.TrimRight(rest[:i], " \t")):]
			}
		}
	}
	trimmed := strings.TrimRight(rest, " \t")
	return trimmed, rest[len(trimmed):]
}
