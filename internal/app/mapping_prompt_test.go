package app

import (
	"acton/internal/act"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func projectWithRunner(t *testing.T, runsOn string) *Session {
	t.Helper()
	dir := t.TempDir()
	wf := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wf, 0o750); err != nil {
		t.Fatal(err)
	}
	yml := "on: push\njobs:\n  a:\n    runs-on: " + runsOn + "\n    steps: [{run: x}]\n"
	if err := os.WriteFile(filepath.Join(wf, "ci.yml"), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newTestSession()
	s.setDir(dir)
	return s
}

func TestUnmappedRunnerLabel(t *testing.T) {
	s := projectWithRunner(t, "ubuntu-24.04")
	s.requestRun()
	if s.running || !s.mappingOpen || len(s.mappingPrompt) != 1 ||
		s.mappingPrompt[0] != (mappingSuggestion{"ubuntu-24.04", "catthehacker/ubuntu:act-24.04"}) {
		t.Fatalf("prompt not shown: open=%v %+v", s.mappingOpen, s.mappingPrompt)
	}
	err := s.startRunChecked()
	if err == nil || s.running || !strings.Contains(err.Error(), "-P ubuntu-24.04=catthehacker/ubuntu:act-24.04") {
		t.Fatalf("non-interactive error: %v", err)
	}
	s = projectWithRunner(t, "ubuntu-latest")
	if labels := s.unmappedLabels(); len(labels) != 0 {
		t.Fatalf("mapped label reported: %v", labels)
	}
}

func TestUnsavedSettingsNotice(t *testing.T) {
	s := projectWithRunner(t, "ubuntu-latest")
	if s.settingsUnsaved() {
		t.Fatal("fresh project reports unsaved settings")
	}
	s.settings.Platforms = append(s.settings.Platforms, "custom=img")
	if !s.settingsUnsaved() {
		t.Fatal("edit not detected")
	}
	if err := s.settings.SaveActrc(); err != nil {
		t.Fatal(err)
	}
	s.markSettingsSaved()
	if s.settingsUnsaved() {
		t.Fatal("still unsaved after save")
	}
	s.settings.Platforms = append(s.settings.Platforms, "other=img")
	s.setDir(s.settings.Dir)
	if s.settingsUnsaved() {
		t.Fatal("still unsaved after reload")
	}
}

func TestRerunJobFromLogPane(t *testing.T) {
	s := projectWithRunner(t, "ubuntu-24.04")
	j := &act.Job{Key: "k", JobID: "a", Name: "a", Status: act.StatusFailure}
	s.run.Jobs = []*act.Job{j}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		s.logPane(c, j)
	}, 800, 400)
	if err := tt.Click("Re-run job a"); err != nil {
		t.Fatal(err)
	}
	if !s.mappingOpen || s.jobOnly != "a" {
		t.Fatalf("re-run did not target the job: open=%v job=%q", s.mappingOpen, s.jobOnly)
	}
	st := s.runSettings()
	args, err := st.Args()
	if err != nil || !slices.Contains(args, "-j") || !slices.Contains(args, "a") {
		t.Fatalf("args do not select the job: %v %v", args, err)
	}
	if s.settings.Job != "" {
		t.Fatalf("configured job filter changed: %q", s.settings.Job)
	}
	s.running = true
	tt.Frame()
	if tt.HasText("Re-run job a") {
		t.Fatal("re-run offered while a run is active")
	}
}
