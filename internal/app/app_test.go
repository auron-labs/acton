package app

import (
	"actgui/internal/act"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
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
		c.SetTheme(designTheme(c))
		a.view(c)
	}, 1120, 720)

	if !tt.HasText("Nothing has run yet") {
		t.Fatalf("expected empty state, texts %q", tt.Texts())
	}

	feed(&a.run,
		`{"job":"CI/build","step":"Set up job","stepid":["--setup-job"],"msg":"⭐ Run Set up job"}`,
		`{"job":"CI/build","step":"Set up job","stepid":["--setup-job"],"stepResult":"success","msg":"✅ Success - Set up job"}`,
		`{"job":"CI/build","step":"go test","stepID":["test"],"stage":"Main","msg":"⭐ Run Main go test"}`,
		`{"job":"CI/build","step":"go test","stepID":["test"],"stage":"Main","raw_output":true,"msg":"ok"}`,
		`{"job":"CI/build","step":"go test","stepID":["test"],"stage":"Main","stepResult":"success","executionTime":1000000000,"msg":"✅ Success - Main go test"}`,
		`{"job":"CI/build","jobResult":"success","msg":"🏁 Job succeeded"}`,
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
	for _, want := range []string{`"msg":"⭐ Run Set up job"`, `"stepResult":"success"`, `"jobResult":"success"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("raw log missing %q:\n%s", want, raw)
		}
	}
	feed(&a.run, `{"job":"CI/build","step":"cleanup","stepID":["cleanup"],"stage":"Post","msg":"⭐ Run Post cleanup"}`)
	tt.Frame()
	if !strings.Contains(a.rawLogFor(a.run.Jobs[0]).buf.String(), "Post cleanup") {
		t.Fatalf("raw buffer did not append new lines")
	}
}

func TestProjectSelectionAndConfiguration(t *testing.T) {
	if err := registerFonts(); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.view(c)
	}, 1120, 720)
	if !tt.HasText("Select a project") || !tt.HasText("Choose project…") || tt.HasText("Workflow") {
		t.Fatalf("expected project picker without settings: %q", tt.Texts())
	}
	a.setDir(t.TempDir())
	tt.Frame()
	if tt.HasText("Select a project") || !tt.HasText("Workflow") || !tt.HasText("Nothing has run yet") {
		t.Fatalf("expected configured project: %q", tt.Texts())
	}
	if err := tt.Click("push"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Custom…"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Custom event name"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("deployment")
	if a.settings.Event != "deployment" {
		t.Fatalf("custom event not applied: %q", a.settings.Event)
	}
	if err := tt.Click("Job ID filter"); err != nil {
		t.Fatal(err)
	}
	tt.Type("build")
	if a.settings.Job != "build" {
		t.Fatalf("job filter not applied: %q", a.settings.Job)
	}
	tt.SetSize(720, 480)
	tt.SetDark(true)
	if !tt.HasText("Nothing has run yet") {
		t.Fatalf("run area missing at minimum size: %q", tt.Texts())
	}
	// The compact pane remains scrollable, so its final field can be edited.
	tt.Scroll(150, 350, 0, 1400)
	if err := tt.Click("Additional act flags"); err != nil {
		t.Fatal(err)
	}
	tt.Type("--offline")
	if a.settings.ExtraArgs != "--offline" {
		t.Fatalf("last configuration field not editable: %q", a.settings.ExtraArgs)
	}
}

func TestFriendlyExitCancellation(t *testing.T) {
	if got := friendlyExit(fmt.Errorf("workflow: %w", context.Canceled)); got != "stopped" {
		t.Fatalf("cancellation: %q", got)
	}
}

func TestMinimumWindowPreservesLogSpace(t *testing.T) {
	a := newApp()
	a.settings.Dir = "/tmp/proj"
	feed(&a.run, `{"job":"CI/build","step":"go test","stepID":["test"],"stage":"Main","msg":"⭐ Run Main go test"}`)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.view(c)
	}, 720, 480)
	log, ok := tt.Find("Step go test")
	if !ok || log.W < 260 {
		t.Fatalf("log pane too narrow at minimum window size: %+v", log)
	}
}
