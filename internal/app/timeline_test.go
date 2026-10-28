package app

import (
	"actgui/internal/act"
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/kballard/go-shellquote"
)

func TestTimelineFailureNavigation(t *testing.T) {
	a := newApp()
	a.settings.Dir = "/tmp/project"
	j := &act.Job{Key: "CI/test", Name: "test", Status: act.StatusFailure}
	for i := range 40 {
		s := &act.Step{Name: fmt.Sprintf("Step %d", i), Status: act.StatusSuccess, Dur: "1s"}
		j.Steps = append(j.Steps, s)
		j.Items = append(j.Items, act.Item{Step: s})
	}
	failed := j.Steps[0]
	failed.Status, failed.Dur, failed.Lines = act.StatusFailure, "31s", []string{"Error: test failed", "", "exit code 2"}
	a.run.Jobs = []*act.Job{j}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.view(c)
	}, 1500, 900)
	for _, text := range []string{"40 steps", "39 passed", "1 failed", "1 slow · over 30s"} {
		if !tt.HasText(text) {
			t.Fatalf("missing summary %q", text)
		}
	}
	if err := tt.Click("Jump to failure"); err != nil {
		t.Fatal(err)
	}
	if !failed.Open || !tt.HasText("Error: test failed") {
		t.Fatal("jump did not open the failed step")
	}
	if bounds, ok := tt.Find("Step Step 0"); !ok || bounds.H == 0 || bounds.Y < 0 || bounds.Y > 900 {
		t.Fatalf("failed step was not brought into view: %+v", bounds)
	}
	if err := tt.Click("Copy output for Step 0"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != "Error: test failed\n\nexit code 2" {
		t.Fatalf("copy lost output or blank lines: %q", got)
	}
	if err := tt.Click("Collapse all"); err != nil {
		t.Fatal(err)
	}
	if failed.Open || tt.HasText("Error: test failed") {
		t.Fatal("collapse all retained expanded output")
	}
}

func TestCommandPreviewRedactsAndQuotes(t *testing.T) {
	a := newApp()
	a.settings.Secrets = []string{"TOKEN=private secret"}
	a.settings.Env = []string{"PRICE=$HOME `whoami`"}
	a.settings.ExtraArgs = "--secret EXTRA=private --secret=INLINE=private -sSHORT=private"
	command := a.cmdline()
	if strings.Contains(command, "private") {
		t.Fatal("secret leaked into command preview")
	}
	words, err := shellquote.Split(command)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(words, "PRICE=$HOME `whoami`") || !contains(words, "TOKEN=••••••••") {
		t.Fatalf("preview changed literal argument values: %q", words)
	}
	args, err := a.settings.Args()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(args, "TOKEN=private secret") {
		t.Fatal("preview redaction changed executable settings")
	}
}
