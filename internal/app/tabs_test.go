package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func newProject(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	writeWorkflow(t, dir)
	return dir
}

func TestOpenProjectReusesTab(t *testing.T) {
	app := newApp()
	dir := newProject(t, "api")
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	first := app.openProject(dir)
	other := app.openProject(newProject(t, "web"))
	if again := app.openProject(link); again != first || len(app.sessions) != 2 || app.current != first {
		t.Fatalf("project opened twice: %d sessions, current=%v", len(app.sessions), app.current)
	}
	if app.addSession(newProject(t, "docs")); app.current != first {
		t.Fatal("adding a session changed the selected tab")
	}
	app.closeSession(first)
	if len(app.sessions) != 2 || app.current != app.sessions[0] || app.sessions[0] != other {
		t.Fatalf("closing the selected tab: %d sessions, current=%v", len(app.sessions), app.current)
	}
	other.running = true
	app.closeSession(other)
	if len(app.sessions) != 2 {
		t.Fatal("closed a tab with an active run")
	}
}

func TestTabsAndHome(t *testing.T) {
	if err := registerFonts(); err != nil {
		t.Fatal(err)
	}
	app := newApp()
	api := app.openProject(newProject(t, "api"))
	web := app.openProject(newProject(t, "web"))
	web.running, web.started, web.runID = true, time.Now(), 1
	feed(&web.run, `{"job":"CI/build","step":"go test","stepID":["test"],"stage":"Main","msg":"⭐ Run Main go test"}`)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		app.view(c)
	}, 1120, 720)
	for _, want := range []string{"Home", "Project api", "Project web"} {
		if _, ok := tt.Find(want); !ok {
			t.Fatalf("tab %q missing: %q", want, tt.Texts())
		}
	}
	if !tt.HasText("Nothing has run yet") && !tt.HasText("Jobs") {
		t.Fatalf("selected project not shown: %q", tt.Texts())
	}
	if err := tt.Click("Home"); err != nil {
		t.Fatal(err)
	}
	if app.current != nil || !tt.HasText("1 running") || !tt.HasText("Running · 0 of 1 jobs finished") || !tt.HasText("Not run yet") {
		t.Fatalf("home does not summarize the projects: %q", tt.Texts())
	}
	if err := tt.Click("Open api"); err != nil {
		t.Fatal(err)
	}
	if app.current != api {
		t.Fatal("project row did not open its tab")
	}
	if err := tt.Click("Project web"); err != nil {
		t.Fatal(err)
	}
	if app.current != web {
		t.Fatal("tab did not select its project")
	}
	if err := tt.Click("Close api"); err != nil {
		t.Fatal(err)
	}
	if len(app.sessions) != 1 || app.sessions[0] != web {
		t.Fatalf("close button kept the tab: %d sessions", len(app.sessions))
	}
}

func TestSameNamedProjectsAreDistinguished(t *testing.T) {
	app := newApp()
	app.openProject(newProject(t, "app"))
	app.openProject(newProject(t, "app"))
	labels := app.tabLabels()
	if labels[0] == labels[1] || !strings.HasSuffix(labels[0], "app") || !strings.HasSuffix(labels[1], "app") {
		t.Fatalf("labels: %q", labels)
	}
}

func TestSelectMultipleWorkflows(t *testing.T) {
	if err := registerFonts(); err != nil {
		t.Fatal(err)
	}
	dir := newProject(t, "api")
	lint := "name: Lint\non: pull_request\njobs:\n  lint:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo lint\n"
	if err := os.WriteFile(filepath.Join(dir, ".github", "workflows", "lint.yml"), []byte(lint), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newApp()
	s := app.openProject(dir)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		app.view(c)
	}, 1120, 720)
	if err := tt.Click("ci.yml"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("lint.yml"); err != nil {
		t.Fatal(err)
	}
	want := []string{".github/workflows/ci.yml", ".github/workflows/lint.yml"}
	if got := s.settings.Workflows; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("workflows = %q, want %q", got, want)
	}
	if !contains(s.workflowJobs, "build") || !contains(s.workflowJobs, "lint") || !strings.HasPrefix(s.workflowMeta, "2 workflows") {
		t.Fatalf("jobs and summary should cover both workflows: %q %q", s.workflowJobs, s.workflowMeta)
	}
	if args, err := s.settings.Args(); err != nil || strings.Count(strings.Join(args, " "), " -W ") != 2 {
		t.Fatalf("args = %q, %v", args, err)
	}
	if err := tt.Click("ci.yml"); err != nil {
		t.Fatal(err)
	}
	if got := s.settings.Workflows; len(got) != 1 || got[0] != want[1] {
		t.Fatalf("after clearing ci.yml: %q", got)
	}
}

func TestRunningWorkflowIsViewable(t *testing.T) {
	if err := registerFonts(); err != nil {
		t.Fatal(err)
	}
	dir := newProject(t, "api")
	lint := "name: Lint\non: pull_request\njobs:\n  lint:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo lint\n"
	if err := os.WriteFile(filepath.Join(dir, ".github", "workflows", "lint.yml"), []byte(lint), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newApp().openProject(dir)
	// Without a selection only workflows triggered by the event run.
	sources := s.workflowSources()
	if len(sources) != 1 || sources[0].name != ".github/workflows/ci.yml" {
		t.Fatalf("sources for push: %+v", sources)
	}
	s.settings.Workflows = s.workflows
	if sources := s.workflowSources(); len(sources) != 2 {
		t.Fatalf("selected workflows run whatever their trigger: %+v", sources)
	}
	s.runWorkflows = s.workflowSources()
	feed(&s.run, `{"job":"CI/build","step":"go test","stepID":["test"],"stage":"Main","msg":"⭐ Run Main go test"}`)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		s.app.view(c)
	}, 1120, 720)
	if err := tt.Click("Workflow"); err != nil {
		t.Fatal(err)
	}
	if s.selJob != workflowJobKey || !tt.HasText("name: CI") || !tt.HasText("- run: echo ok") {
		t.Fatalf("workflow source not shown: %q", tt.Texts())
	}
	s.workflowTab = 1
	tt.Frame()
	if !tt.HasText("name: Lint") {
		t.Fatalf("second workflow not shown: %q", tt.Texts())
	}
	s.clearRun()
	if s.runWorkflows != nil {
		t.Fatal("clearing the run kept its workflow")
	}
}
