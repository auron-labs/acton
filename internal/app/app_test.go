package app

import (
	"acton/internal/act"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	a.workflows = []string{".github/workflows/ci.yml"}
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
	dir := t.TempDir()
	writeWorkflow(t, dir)
	a.setDir(dir)
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
	tt.Scroll(150, 250, 0, 700)
	if err := tt.Click("Advanced"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Scroll(150, 250, 0, 1400)
	if err := tt.Click("Additional act flags"); err != nil {
		t.Fatal(err)
	}
	tt.Type("--offline")
	if a.settings.ExtraArgs != "--offline" {
		t.Fatalf("last configuration field not editable: %q", a.settings.ExtraArgs)
	}
}

// writeWorkflow creates a minimal .github/workflows file in dir.
func writeWorkflow(t *testing.T, dir string) {
	t.Helper()
	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o750); err != nil {
		t.Fatal(err)
	}
	workflow := "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n"
	if err := os.WriteFile(filepath.Join(wfDir, "ci.yml"), []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestNoWorkflowsState covers the dedicated empty state: it replaces the
// panes, blocks runs, and its Check again rescan reaches the configured
// state without resetting settings.
func TestNoWorkflowsState(t *testing.T) {
	a := newApp()
	a.setDir(t.TempDir())
	a.settings.Job = "build"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.view(c)
	}, 1120, 720)
	for _, want := range []string{"No GitHub Actions found", ".github/workflows", "Check again", "Choose another project…"} {
		if !tt.HasText(want) {
			t.Fatalf("empty state missing %q: %q", want, tt.Texts())
		}
	}
	if tt.HasText("Nothing has run yet") || tt.HasText("Workflow") {
		t.Fatalf("no-workflow state should replace the config and run panes: %q", tt.Texts())
	}
	// Run is unavailable without workflow files, however it is invoked.
	ht := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.headerActions(c)
	}, 400, 64)
	if err := ht.Click("Run workflow"); err != nil {
		t.Fatalf("header Run missing: %v", err)
	}
	a.startRun()
	if a.running || a.runner != nil {
		t.Fatal("run started without workflow files")
	}
	if err := tt.Click("Check again"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("No GitHub Actions found") || a.settings.Job != "build" {
		t.Fatal("empty rescan changed the state or reset settings")
	}

	// Check again picks up newly added workflow files while keeping settings.
	writeWorkflow(t, a.settings.Dir)
	if err := tt.Click("Check again"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Workflow") || !tt.HasText("Nothing has run yet") || tt.HasText("No GitHub Actions found") {
		t.Fatalf("rescan did not reach the configured state: %q", tt.Texts())
	}
	if a.settings.Job != "build" || len(a.workflows) != 1 {
		t.Fatalf("rescan lost settings or workflows: %q, %q", a.settings.Job, a.workflows)
	}
}

func TestRescanPreservesExplicitWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir)
	if err := os.Rename(filepath.Join(dir, ".github", "workflows", "ci.yml"), filepath.Join(dir, "custom.yaml")); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.setDir(dir)
	a.settings.Workflow = "custom.yaml"
	a.rescanWorkflows()
	if len(a.workflows) != 1 || a.workflows[0] != "custom.yaml" || a.settings.Workflow != "custom.yaml" || a.workflowErr != "" || !contains(a.workflowJobs, "build") {
		t.Fatalf("explicit workflow lost on rescan: workflows=%q selection=%q jobs=%q error=%q", a.workflows, a.settings.Workflow, a.workflowJobs, a.workflowErr)
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
	a.workflows = []string{".github/workflows/ci.yml"}
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

func TestShutdownStopsRunAndWaitsForCleanup(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o750); err != nil {
		t.Fatal(err)
	}
	workflow := `name: CI
on: push
jobs:
  wait:
    runs-on: ubuntu-latest
    steps:
      - run: printf 'waiting\n'; exec sleep 60
`
	if err := os.WriteFile(filepath.Join(dir, ".github", "workflows", "ci.yml"), []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.runner = &act.Runner{}
	logged := make(chan struct{}, 1)
	if err := a.runner.Start(dir, []string{"push", "-P", "ubuntu-latest=-self-hosted", "--no-cache-server", "--action-cache-path", t.TempDir()}, func() {
		lines, _, _ := a.runner.Drain()
		for _, line := range lines {
			if strings.Contains(line, `"raw_output":true`) && strings.Contains(line, "waiting") {
				select {
				case logged <- struct{}{}:
				default:
				}
			}
		}
	}, func() {}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.runner.Stop(); a.runner.Wait() })
	select {
	case <-logged:
	case <-time.After(15 * time.Second):
		t.Fatal("workflow did not start")
	}
	// Shutdown owns the runner lifecycle even when the view's flag is stale.
	a.running = false
	a.shutdown()
	_, done, err := a.runner.Drain()
	if !done || !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown returned before cancellation and cleanup: done=%v, err=%v", done, err)
	}
	select {
	case <-a.done:
	default:
		t.Fatal("shutdown did not close the application lifetime")
	}
}
