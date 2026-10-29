package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSidebarEditors(t *testing.T) {
	a := newApp()
	a.settings.Dir = "/tmp/project"
	a.settings.Secrets = []string{"TOKEN=original=value"}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.configView(c)
	}, 320, 1052)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
	}
	edit := func(label, value string) {
		t.Helper()
		click(label)
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type(value)
	}
	if tt.HasText("Container architecture") || tt.HasText("Dry run (plan only)") {
		t.Fatal("settings sections should start collapsed")
	}
	click("Images & platforms")
	edit("Runner label 1", "custom-runner")
	edit("Container image 1", "ubuntu:24.04")
	if got := a.settings.Platforms[0]; got != "custom-runner=ubuntu:24.04" {
		t.Fatalf("mapping edits lost: %q", got)
	}
	click("arm64")
	if a.settings.Arch != "linux/arm64" {
		t.Fatalf("architecture not applied: %q", a.settings.Arch)
	}
	click("+ Add mapping")
	edit("Runner label 2", "second")
	click("Remove mapping 1")
	if !slices.Equal(a.settings.Platforms, []string{"second="}) {
		t.Fatalf("wrong mapping removed: %q", a.settings.Platforms)
	}
	// The surviving row must not inherit the removed row's edits.
	edit("Runner label 1", "second-runner")
	edit("Container image 1", "ubuntu:22.04")
	if !slices.Equal(a.settings.Platforms, []string{"second-runner=ubuntu:22.04"}) {
		t.Fatalf("mapping edit after removal corrupted values: %q", a.settings.Platforms)
	}
	click("Images & platforms")
	click("Environment")
	if tt.HasText("original=value") || strings.Contains(a.cmdline(), "original=value") {
		t.Fatal("secret exposed in sidebar or command preview")
	}
	click("Show secret 1")
	click("Secrets value 1")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "original=value" {
		t.Fatal("revealed secret could not be copied")
	}
	click("Hide secret 1")
	tt.SetClipboard("unchanged")
	click("Secrets value 1")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "unchanged" {
		t.Fatal("masked secret was copied")
	}
	edit("Secrets value 1", "new=value")
	if a.settings.Secrets[0] != "TOKEN=new=value" {
		t.Fatalf("secret with equals sign corrupted: %q", a.settings.Secrets)
	}
	click("Env")
	click("+ Add env")
	edit("Env name 1", "MODE")
	edit("Env value 1", "test")
	click("Secrets 1")
	if a.settings.Secrets[0] != "TOKEN=new=value" || !slices.Equal(a.settings.Env, []string{"MODE=test"}) {
		t.Fatal("switching tabs lost values")
	}
	click("Copy command")
	if strings.Contains(tt.Clipboard(), "new=value") || !strings.Contains(tt.Clipboard(), "MODE=test") {
		t.Fatalf("unsafe or incomplete command copy: %q", tt.Clipboard())
	}
	click("Environment")
	click("Options")
	click("Dry run (plan only)")
	if !a.settings.DryRun || a.optionCount() != 2 {
		t.Fatalf("option toggle/count incorrect: %+v", a.settings)
	}
	// Enter toggles the focused section header through the native disclosure.
	click("Options")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	if !a.configOpen[2] {
		t.Fatal("section did not open from keyboard")
	}
}

func TestWorkflowMetadataAndJobSelection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ci.yml")
	if err := os.WriteFile(path, []byte("on: [push, pull_request]\njobs:\n  test: {}\n  build: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.settings.Dir, a.settings.Workflow = dir, "ci.yml"
	a.workflows = []string{"ci.yml"}
	a.loadWorkflow()
	if a.workflowMeta != "2 jobs · on push, pull_request" || !slices.Equal(a.workflowJobs, []string{"build", "test"}) {
		t.Fatalf("incorrect metadata: %q, %q", a.workflowMeta, a.workflowJobs)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.configView(c)
	}, 320, 720)
	if err := tt.Click("Job ID filter"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("test"); err != nil {
		t.Fatal(err)
	}
	if a.settings.Job != "test" {
		t.Fatal("job menu did not apply the job ID")
	}
	a.settings.Workflow = "missing.yml"
	a.loadWorkflow()
	if a.workflowErr == "" || len(a.workflowJobs) != 0 {
		t.Fatal("unreadable workflow retained stale metadata")
	}
}

func TestWorkflowTriggerForms(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
	}{
		{"scalar", "on: push\njobs: {}", "0 jobs · on push"},
		{"mapping", "on:\n  workflow_dispatch:\n    inputs: {}\njobs: {}", "0 jobs · on workflow_dispatch"},
		{"invalid", "jobs: [", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(tc.source), 0o600); err != nil {
				t.Fatal(err)
			}
			a := newApp()
			a.settings.Dir, a.settings.Workflow = dir, "ci.yml"
			a.loadWorkflow()
			if a.workflowMeta != tc.want || (a.workflowErr != "") != (tc.name == "invalid") {
				t.Fatalf("metadata=%q, error=%q", a.workflowMeta, a.workflowErr)
			}
		})
	}
}

func TestSaveSettingsButton(t *testing.T) {
	a := newApp()
	a.settings.Dir = t.TempDir()
	a.settings.Event = "release"
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.configView(c)
	}, 320, 720)
	if err := tt.Click("Save settings to .actrc"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(a.settings.Dir, ".actrc"))
	if err != nil || !strings.Contains(string(data), "release") || a.fileErr != "" {
		t.Fatalf("save: %q, %v, %s", data, err, a.fileErr)
	}
	a.settings.ExtraArgs = `--env "unclosed`
	if err := tt.Click("Save settings to .actrc"); err != nil {
		t.Fatal(err)
	}
	if a.fileErr == "" {
		t.Fatal("save failure was not reported")
	}
}
