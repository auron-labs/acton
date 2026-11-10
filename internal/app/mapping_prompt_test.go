package app

import (
	"os"
	"path/filepath"
	"testing"
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
