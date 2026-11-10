package app

import (
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"
)

type yamlKind int

const (
	yamlPlain yamlKind = iota
	yamlComment
	yamlKey
	yamlString
	yamlScalar // numbers, booleans and null
	yamlPunct
	yamlMeta // anchors, aliases and tags
)

type yamlSegment struct {
	text string
	kind yamlKind
}

var (
	yamlScalarRe    = regexp.MustCompile(`^(?:[-+]?(?:\d[\d_]*)(?:\.\d+)?(?:[eE][-+]?\d+)?|0x[0-9a-fA-F]+|0o[0-7]+|\.inf|\.nan|true|false|yes|no|on|off|null|~)$`)
	yamlBlockHeadRe = regexp.MustCompile(`^[|>][-+0-9]*$`)
)

// highlightYAML tokenizes every line of a YAML document. Block scalar bodies
// (after `key: |` or `key: >`) are tracked across lines and rendered as strings.
func highlightYAML(lines []string) [][]yamlSegment {
	out := make([][]yamlSegment, len(lines))
	blockIndent := -1 // indent of the line that opened the block scalar; -1 when outside one
	for i, line := range lines {
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if blockIndent >= 0 {
			if strings.TrimSpace(line) == "" || indent > blockIndent {
				if line != "" {
					out[i] = []yamlSegment{{line, yamlString}}
				}
				continue
			}
			blockIndent = -1
		}
		out[i] = tokenizeYAMLLine(line)
		if opensBlockScalar(out[i]) {
			blockIndent = indent
		}
	}
	return out
}

// opensBlockScalar reports whether the line ends in a `|` or `>` indicator
// after a key colon or list dash.
func opensBlockScalar(segs []yamlSegment) bool {
	for i := len(segs) - 1; i > 0; i-- {
		if segs[i].kind == yamlComment || strings.TrimSpace(segs[i].text) == "" {
			continue
		}
		return yamlBlockHeadRe.MatchString(strings.TrimSpace(segs[i].text)) &&
			segs[i-1].kind != yamlString && strings.TrimSpace(segs[i].text)[0] != '"'
	}
	return false
}

func tokenizeYAMLLine(line string) []yamlSegment {
	var segs []yamlSegment
	add := func(text string, kind yamlKind) {
		if text == "" {
			return
		}
		if n := len(segs); n > 0 && segs[n-1].kind == kind {
			segs[n-1].text += text
			return
		}
		segs = append(segs, yamlSegment{text, kind})
	}

	rest := line
	// Leading whitespace, document markers and list dashes.
	for {
		trimmed := strings.TrimLeft(rest, " \t")
		add(rest[:len(rest)-len(trimmed)], yamlPlain)
		rest = trimmed
		if rest == "---" || rest == "..." || strings.HasPrefix(rest, "--- ") {
			add(rest[:3], yamlPunct)
			rest = rest[3:]
			continue
		}
		if rest == "-" || strings.HasPrefix(rest, "- ") {
			add("-", yamlPunct)
			rest = rest[1:]
			continue
		}
		break
	}
	if rest == "" {
		return segs
	}
	if strings.HasPrefix(rest, "#") {
		add(rest, yamlComment)
		return segs
	}

	// Mapping key: a plain or quoted scalar followed by a colon and a space or end.
	if end := yamlKeyEnd(rest); end > 0 {
		add(rest[:end], yamlKey)
		add(":", yamlPunct)
		rest = rest[end+1:]
	}
	tokenizeYAMLValue(rest, add)
	return segs
}

// yamlKeyEnd returns the length of a mapping key at the start of s, or 0.
func yamlKeyEnd(s string) int {
	i := 0
	if s[0] == '"' || s[0] == '\'' {
		i = quotedEnd(s)
		if i < 0 {
			return 0
		}
	} else {
		if strings.ContainsRune("#[{&*!|>%@`", rune(s[0])) {
			return 0
		}
		for ; i < len(s); i++ {
			if s[i] == ':' && (i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\t') {
				return i
			}
			if s[i] == '#' && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
				return 0
			}
		}
		return 0
	}
	// Quoted key: allow spaces before the colon.
	j := i
	for j < len(s) && s[j] == ' ' {
		j++
	}
	if j < len(s) && s[j] == ':' && (j+1 == len(s) || s[j+1] == ' ' || s[j+1] == '\t') {
		return j
	}
	return 0
}

// quotedEnd returns the index just past the closing quote of the string
// starting at s[0], or -1 when it is not closed on this line.
func quotedEnd(s string) int {
	q := s[0]
	for i := 1; i < len(s); i++ {
		switch {
		case q == '"' && s[i] == '\\':
			i++
		case s[i] == q:
			if q == '\'' && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			return i + 1
		}
	}
	return -1
}

// tokenizeYAMLValue splits the part of a line after any key.
func tokenizeYAMLValue(s string, add func(string, yamlKind)) {
	for len(s) > 0 {
		switch c := s[0]; {
		case c == ' ' || c == '\t':
			n := len(s) - len(strings.TrimLeft(s, " \t"))
			add(s[:n], yamlPlain)
			s = s[n:]
			if strings.HasPrefix(s, "#") {
				add(s, yamlComment)
				return
			}
		case c == '"' || c == '\'':
			end := quotedEnd(s)
			if end < 0 {
				end = len(s)
			}
			add(s[:end], yamlString)
			s = s[end:]
		case strings.ContainsRune("[]{},:", rune(c)):
			add(s[:1], yamlPunct)
			s = s[1:]
		default:
			end := plainEnd(s)
			word := s[:end]
			switch {
			case c == '&' || c == '*' || c == '!':
				add(word, yamlMeta)
			case yamlScalarRe.MatchString(word):
				add(word, yamlScalar)
			default:
				add(word, yamlPlain)
			}
			s = s[end:]
		}
	}
}

// plainEnd returns the length of the unquoted word at the start of s.
func plainEnd(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', ',', ']', '}':
			return i
		}
	}
	return len(s)
}

// yamlSpans converts segments to coloured RichText spans using the theme.
func yamlSpans(t *ui.Theme, line string, segs []yamlSegment) []ui.Span {
	if len(segs) == 0 {
		segs = []yamlSegment{{line, yamlPlain}}
	}
	spans := make([]ui.Span, len(segs))
	for i, seg := range segs {
		spans[i] = ui.Span{Text: seg.text, Color: yamlColor(t, seg.kind)}
	}
	return spans
}

func yamlColor(t *ui.Theme, kind yamlKind) ui.Color {
	switch kind {
	case yamlPlain:
		return t.Text
	case yamlComment:
		return t.TextMuted
	case yamlKey:
		return t.Accent.Mix(t.Text, .25)
	case yamlString:
		return t.Success
	case yamlScalar:
		return t.Warning
	case yamlPunct:
		return t.TextMuted
	case yamlMeta:
		return t.Danger
	}
	return t.Text
}
