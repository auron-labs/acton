package app

import (
	"reflect"
	"testing"
)

func TestTokenizeYAMLLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []yamlSegment
	}{
		{"empty", "", nil},
		{"comment", "  # note", []yamlSegment{{"  ", yamlPlain}, {"# note", yamlComment}}},
		{"key value", "name: CI", []yamlSegment{{"name", yamlKey}, {":", yamlPunct}, {" CI", yamlPlain}}},
		{"nested key bool", "  fail-fast: true", []yamlSegment{{"  ", yamlPlain}, {"fail-fast", yamlKey}, {":", yamlPunct}, {" ", yamlPlain}, {"true", yamlScalar}}},
		{"list item key", "  - uses: actions/checkout@v4", []yamlSegment{{"  ", yamlPlain}, {"-", yamlPunct}, {" ", yamlPlain}, {"uses", yamlKey}, {":", yamlPunct}, {" actions/checkout@v4", yamlPlain}}},
		{"quoted hash", `run: "echo # hi" # real`, []yamlSegment{{"run", yamlKey}, {":", yamlPunct}, {" ", yamlPlain}, {`"echo # hi"`, yamlString}, {" ", yamlPlain}, {"# real", yamlComment}}},
		{"single quote escape", `a: 'it''s #1'`, []yamlSegment{{"a", yamlKey}, {":", yamlPunct}, {" ", yamlPlain}, {`'it''s #1'`, yamlString}}},
		{"number", "go: 1.22", []yamlSegment{{"go", yamlKey}, {":", yamlPunct}, {" ", yamlPlain}, {"1.22", yamlScalar}}},
		{"hash inside word", "url: a#b", []yamlSegment{{"url", yamlKey}, {":", yamlPunct}, {" a#b", yamlPlain}}},
		{"plain list item", "- 42", []yamlSegment{{"-", yamlPunct}, {" ", yamlPlain}, {"42", yamlScalar}}},
		{"flow list", "on: [push, pull_request]", []yamlSegment{{"on", yamlKey}, {":", yamlPunct}, {" ", yamlPlain}, {"[", yamlPunct}, {"push", yamlPlain}, {",", yamlPunct}, {" ", yamlPlain}, {"pull_request", yamlPlain}, {"]", yamlPunct}}},
		{"expression braces", "if: ${{ a }}", []yamlSegment{{"if", yamlKey}, {":", yamlPunct}, {" ${{ a ", yamlPlain}, {"}}", yamlPunct}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenizeYAMLLine(tc.line)
			// Compare after merging adjacent same-kind segments for readability.
			if !reflect.DeepEqual(got, merge(tc.want)) {
				t.Errorf("got %#v\nwant %#v", got, merge(tc.want))
			}
		})
	}
}

func merge(segs []yamlSegment) []yamlSegment {
	var out []yamlSegment
	for _, s := range segs {
		if n := len(out); n > 0 && out[n-1].kind == s.kind {
			out[n-1].text += s.text
			continue
		}
		out = append(out, s)
	}
	return out
}

func TestHighlightYAMLBlockScalar(t *testing.T) {
	lines := []string{
		"steps:",
		"  - run: |",
		"      echo # not a comment",
		"",
		"      key: value",
		"  - name: after",
	}
	got := highlightYAML(lines)
	for _, i := range []int{2, 4} {
		if len(got[i]) != 1 || got[i][0].kind != yamlString || got[i][0].text != lines[i] {
			t.Errorf("line %d = %#v, want one string segment", i, got[i])
		}
	}
	if got[3] != nil {
		t.Errorf("blank line = %#v, want nil", got[3])
	}
	if got[5][1].kind != yamlPunct || got[5][3].kind != yamlKey {
		t.Errorf("line after block = %#v, want tokenized normally", got[5])
	}
}
