package app

import (
	"acton/internal/act"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/kballard/go-shellquote"
)

func TestTimelineFailureNavigation(t *testing.T) {
	a := newApp()
	a.settings.Dir = "/tmp/project"
	j := &act.Job{Key: "CI/test", Name: "test", Status: act.StatusFailure}
	for i := range 40 {
		s := &act.Step{Name: fmt.Sprintf("Step %d", i), Status: act.StatusSuccess, Dur: time.Second}
		j.Steps = append(j.Steps, s)
		j.Items = append(j.Items, act.Item{Step: s})
	}
	failed := j.Steps[0]
	failed.Status, failed.Dur, failed.Lines = act.StatusFailure, 31*time.Second, []string{"Error: test failed", "", "exit code 2"}
	a.run.Jobs = []*act.Job{j}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.view(c)
	}, 1500, 900)
	for _, text := range []string{"All 40 steps", "39 passed", "1 failed", "1 slow · over 30s"} {
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

func TestTimelineCompositeFilters(t *testing.T) {
	a := newApp()
	a.settings.Dir = "/tmp/project"
	start := time.Now().Add(-2 * time.Minute)
	parent := &act.Step{Name: "setup", Source: "./.github/actions/setup", Status: act.StatusFailure, Started: start, Ended: start.Add(80 * time.Second), Open: true}
	for i, child := range []*act.Step{
		{Name: "Restore cache", Source: "actions/cache@55c8345", Fetched: true, Status: act.StatusSuccess, Dur: 5 * time.Second},
		{Name: "Install tools", Status: act.StatusSuccess, Dur: 74 * time.Second},
		{Name: "Check formatting", Status: act.StatusFailure, Dur: time.Second, Open: true, Lines: []string{"Error: fixture failed"}},
	} {
		child.Parent = parent
		child.Started = start.Add(time.Duration(i) * 5 * time.Second)
		child.Ended = child.Started.Add(child.Dur)
		parent.Children = append(parent.Children, child)
	}
	j := &act.Job{Key: "build", Name: "build", Status: act.StatusFailure, Steps: []*act.Step{parent}, Items: []act.Item{{Step: parent}}, Started: start, Ended: parent.Ended}
	a.run.Jobs = []*act.Job{j}
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(designTheme(c)); a.view(c) }, 1500, 900)
	for _, label := range []string{"All 3 steps", "2 passed", "1 failed", "1 slow · over 30s", "1.1", "1.2", "1.3", "3 steps"} {
		if !tt.HasText(label) {
			t.Fatalf("missing %q: %q", label, tt.Texts())
		}
	}
	if err := tt.Click("Step setup"); err != nil {
		t.Fatal(err)
	}
	if parent.Open || !parent.UserToggled || tt.HasText("Restore cache") {
		t.Fatal("parent did not collapse children")
	}
	for _, tc := range []struct{ filter, label, absent string }{
		{"2 passed", "Restore cache", "Check formatting"},
		{"1 slow · over 30s", "Install tools", "Restore cache"},
		{"1 failed", "Check formatting", "Install tools"},
	} {
		if err := tt.Click(tc.filter); err != nil {
			t.Fatal(err)
		}
		if !tt.HasText("setup") || !tt.HasText(tc.label) || tt.HasText(tc.absent) {
			t.Fatalf("filter %s: %q", tc.filter, tt.Texts())
		}
	}
	if err := tt.Click("Step setup"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Check formatting") {
		t.Fatal("filtered parent did not collapse")
	}
	if err := tt.Click("Jump to failure"); err != nil {
		t.Fatal(err)
	}
	if a.logUI.filter != filterAll || !parent.Open || !tt.HasText("Error: fixture failed") {
		t.Fatal("jump did not reveal failure through parent/filter")
	}
}

func TestTimelineOutputBoundAndFollow(t *testing.T) {
	a := newApp()
	s := &act.Step{Name: "Log stream", Status: act.StatusRunning, Open: true, Started: time.Now()}
	for i := range 100 {
		s.Lines = append(s.Lines, fmt.Sprintf("Output %03d", i))
	}
	after := &act.Step{Name: "After log", Status: act.StatusSuccess}
	j := &act.Job{Steps: []*act.Step{s, after}, Items: []act.Item{{Step: s}, {Step: after}}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		ui.Column(c).Fill().Children(func() { a.stepsView(c, j, jobTimeAxis(j, time.Now())) })
	}, 1000, 800)
	if !tt.HasText("Output 099") {
		t.Fatal("running output is not following its end")
	}
	bounds, ok := tt.Find("Copy output for Log stream")
	if !ok || bounds.Y > 100 {
		t.Fatalf("output header not visible: %+v", bounds)
	}
	if next, ok := tt.Find("Step After log"); !ok || next.Y > 400 {
		t.Fatalf("output exceeded its height cap: %+v", next)
	}
	// The panel stays bounded even as more lines arrive.
	s.Lines = append(s.Lines, "Output 100")
	tt.Frame()
	if !tt.HasText("Output 100") {
		t.Fatal("new line did not follow")
	}
	if err := tt.Click("Step Log stream"); err != nil {
		t.Fatal(err)
	}
	if s.Open || !s.UserToggled || tt.HasText("Output 100") {
		t.Fatal("manual collapse did not hide output")
	}
}

func TestJobTimeAxis(t *testing.T) {
	for _, span := range []time.Duration{time.Second, 8 * time.Second, 49 * time.Second, 203 * time.Second, 22 * time.Minute, 2 * time.Hour} {
		t.Run(span.String(), func(t *testing.T) {
			start := time.Unix(100, 0)
			s := &act.Step{Started: start, Ended: start.Add(span)}
			j := &act.Job{Steps: []*act.Step{s}}
			axis := jobTimeAxis(j, start.Add(span))
			count := int(axis.span/axis.tick) + 1
			if axis.start != start || axis.span != span || count < 4 || count > 7 {
				t.Fatalf("axis=%+v ticks=%d", axis, count)
			}
			s.Ended = time.Time{}
			axis = jobTimeAxis(j, start.Add(span))
			if axis.span != span {
				t.Fatalf("running span=%s", axis.span)
			}
		})
	}
}
