package app

import (
	"actgui/internal/act"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/theme"
)

func feed(r *act.Run, lines ...string) {
	for _, l := range lines {
		r.Feed(l)
	}
}

// TestViewSmoke builds the app view headlessly and exercises the jobs pane.
func TestViewSmoke(t *testing.T) {
	a := newApp()
	a.settings.Dir = "/tmp/proj"
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		a.view(c)
	}, 1120, 720)

	if !tt.HasText("Nothing has run yet") {
		t.Fatalf("expected empty state, texts %q", tt.Texts())
	}

	feed(&a.run,
		`[CI/build] ⭐  Run Set up job`,
		`[CI/build]   ✅  Success - Set up job`,
		`[CI/build] ⭐  Run Main go test`,
		`[CI/build]   | ok`,
		`[CI/build]   ✅  Success - Main go test [1s]`,
		`[CI/build] 🏁  Job succeeded`,
		`act: some runner warning`,
	)
	tt.Frame()

	if !tt.HasText("Jobs") || !tt.HasText("build") || !tt.HasText("go test") {
		t.Fatalf("missing job/step text: %q", tt.Texts())
	}
	// Succeeded steps render collapsed like GitHub; expanding shows lines.
	if tt.HasText("ok") {
		t.Fatalf("collapsed step should hide lines: %q", tt.Texts())
	}
	if err := tt.Click("go test"); err != nil {
		t.Fatalf("click step: %v", err)
	}
	if !tt.HasText("ok") {
		t.Fatalf("open step lines should render: %q", tt.Texts())
	}
	// Selecting the runner entry shows its untagged output.
	if err := tt.Click("Runner output"); err != nil {
		t.Fatalf("click runner row: %v", err)
	}
	if !tt.HasText("act: some runner warning") {
		t.Fatalf("runner pane should show untagged lines: %q", tt.Texts())
	}

	// Back on the job, the Raw log view keeps every verbatim line.
	if err := tt.Click("build"); err != nil {
		t.Fatalf("click job row: %v", err)
	}
	if err := tt.Click("Raw log"); err != nil {
		t.Fatalf("click raw log: %v", err)
	}
	tt.Frame()
	raw := a.rawLogFor(a.run.Jobs[0]).buf.String()
	for _, want := range []string{"⭐  Run Set up job", "✅  Success - Main go test", "🏁  Job succeeded"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("raw log missing %q:\n%s", want, raw)
		}
	}
	feed(&a.run, `[CI/build] ⭐  Run Post cleanup`)
	tt.Frame()
	if !strings.Contains(a.rawLogFor(a.run.Jobs[0]).buf.String(), "Post cleanup") {
		t.Fatalf("raw buffer did not append new lines")
	}
}
