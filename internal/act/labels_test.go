package act

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestUnmappedLabels(t *testing.T) {
	dir := t.TempDir()
	wf := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wf, 0o750); err != nil {
		t.Fatal(err)
	}
	const yml = `on: push
jobs:
  a: {runs-on: ubuntu-latest, steps: [{run: x}]}
  b: {runs-on: My-Runner, steps: [{run: x}]}
  c: {runs-on: [self-hosted, linux], steps: [{run: x}]}
  d: {runs-on: "${{ matrix.os }}", steps: [{run: x}]}
  e: {runs-on: {group: g, labels: [mapped]}, steps: [{run: x}]}
  f: {runs-on: ubuntu-24.04, steps: [{run: x}]}
`
	if err := os.WriteFile(filepath.Join(wf, "ci.yml"), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	got := UnmappedLabels(dir, nil, "push", "", []string{"Mapped=img"})
	slices.Sort(got)
	if want := []string{"my-runner", "self-hosted", "ubuntu-24.04"}; !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := UnmappedLabels(dir, nil, "push", "a", nil); len(got) != 0 {
		t.Fatalf("job filter ignored: %v", got)
	}
	if got := UnmappedLabels(dir, nil, "pull_request", "", nil); len(got) != 0 {
		t.Fatalf("event ignored: %v", got)
	}
}
