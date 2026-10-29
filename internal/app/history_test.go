package app

import (
	"acton/internal/act"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunFilesPrivateDistinctAndMasked(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	project := t.TempDir()
	h, err := newRunFiles(project, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	h2, err := newRunFiles(project, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h2.close() }()
	if h.dir == h2.dir {
		t.Fatal("run directories collided")
	}
	for _, line := range []string{`{"msg_masked":true,"job":"ci/build","jobID":"build","matrix":{"os":"linux"},"msg":"***","arg":"secret-value","properties":{"title":"secret-value"}}`, `{"msg_masked":true,"job":"ci/build","jobID":"build","matrix":{"os":"windows"},"msg":"ok"}`, `{"msg_masked":true,"job":"ci/build","jobID":"build","msg":"plain job"}`, `{"msg_masked":true,"msg":"runner diagnostic"}`} {
		if err := h.write(line); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.close(); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(h.dir)
	if err != nil || len(files) != 4 {
		t.Fatalf("files: %v, %v", files, err)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "ci-build.jsonl")); err != nil {
		t.Fatalf("workflow-job filename missing: %v", err)
	}
	if info, _ := os.Stat(h.dir); info.Mode().Perm() != 0o700 {
		t.Fatalf("directory permissions %o", info.Mode().Perm())
	}
	for _, f := range files {
		path := filepath.Join(h.dir, f.Name())
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("file permissions %o", info.Mode().Perm())
		}
		//nolint:gosec // G304: path is from this test's private temporary directory.
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "secret-value") {
			t.Fatal("unmasked metadata saved")
		}
	}
}

func TestPreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acton", "prefs.json")
	p, err := loadPreferences(path)
	if err != nil || p.SaveRuns || p.Notifications {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	p.SaveRuns = true
	if err := savePreferences(path, p); err != nil {
		t.Fatal(err)
	}
	got, err := loadPreferences(path)
	if err != nil || got != p {
		t.Fatalf("roundtrip: %+v %v", got, err)
	}
}

func TestPumpCompletesOnlyOnce(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	project := t.TempDir()
	a := newApp()
	a.started = time.Now()
	a.running = true
	var err error
	a.runFiles, err = newRunFiles(project, a.started)
	if err != nil {
		t.Fatal(err)
	}
	dir := a.runFiles.dir
	a.runner = &act.Runner{}
	if err := a.runner.Start(project, []string{"--dryrun"}, func() {}, func() {}); err != nil {
		t.Fatal(err)
	}
	a.runner.Wait()
	calls := 0
	a.notifyTest = func(act.Status) { calls++ }
	a.pump()
	elapsed := a.elapsed
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var before []byte
	if len(entries) > 0 {
		//nolint:gosec // G304: directory and entries belong to this test fixture.
		before, err = os.ReadFile(filepath.Join(dir, entries[0].Name()))
		if err != nil {
			t.Fatal(err)
		}
	}
	a.pump()
	a.pump()
	if calls != 1 || a.running || a.elapsed != elapsed || a.runFiles != nil {
		t.Fatalf("completion repeated: calls=%d running=%v", calls, a.running)
	}
	if len(entries) > 0 {
		//nolint:gosec // G304: directory and entries belong to this test fixture.
		after, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
		if err != nil || string(before) != string(after) {
			t.Fatalf("history changed after completion: %v", err)
		}
	}
}

func TestHistoryWriteFailureKeepsOutput(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a := newApp()
	var err error
	a.runFiles, err = newRunFiles(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(a.runFiles.dir); err != nil {
		t.Fatal(err)
	}
	a.feedRun(`{"msg":"visible diagnostic"}`)
	if len(a.run.Runner) != 1 || a.run.Runner[0] != "visible diagnostic" || a.preferenceErr == "" || a.runFiles != nil {
		t.Fatalf("output or error lost: %+v %q", a.run.Runner, a.preferenceErr)
	}
}

func TestHistoryOmitsOutputWithoutMaskingGuarantee(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	h, err := newRunFiles(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		`{"msg_masked":false,"job":"ci/build","jobID":"build","msg":"insecure-secret-value"}`,
		`{"msg":"global-logger-secret-value"}`,
		`unstructured-secret-value`,
	} {
		if err := h.write(line); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(h.dir, "runner.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-value") || strings.Count(string(data), "omitted") != 3 {
		t.Fatalf("unmasked data persisted: %s", data)
	}
}
