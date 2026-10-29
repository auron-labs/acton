package app

import (
	"acton/internal/act"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestOverviewNavigationAndAnnotations(t *testing.T) {
	a := newApp()
	a.settings.Dir = "/tmp/project"
	a.workflows = []string{"ci.yml"}
	j := &act.Job{Key: "build", Name: "build", Annotations: []act.Annotation{{Severity: "error", File: "src/main.go", Line: "12", Message: "broken"}}}
	a.run.Jobs = []*act.Job{j}
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(designTheme(c)); a.view(c) }, 1200, 800)
	if err := tt.Click("Overview"); err != nil {
		t.Fatal(err)
	}
	if a.selJob != overviewJobKey || !tt.HasText("src/main.go  (1)") || !tt.HasText("broken") {
		t.Fatalf("overview: %v", tt.Texts())
	}
	if err := tt.Click("Job build"); err != nil {
		t.Fatal(err)
	}
	if a.selJob != j.Key {
		t.Fatal("job selection failed")
	}
	a.clearRun()
	if a.selJob != "" {
		t.Fatal("clear did not reset selection")
	}
}

func TestLiveStepOutputRetainsManualScroll(t *testing.T) {
	a := newApp()
	s := &act.Step{Name: "stream", Status: act.StatusRunning, Open: true, Started: time.Now()}
	for i := range 100 {
		s.Lines = append(s.Lines, fmt.Sprintf("line %03d", i))
	}
	j := &act.Job{Steps: []*act.Step{s}, Items: []act.Item{{Step: s}}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		ui.Column(c).Fill().Children(func() { a.stepsView(c, j, jobTimeAxis(j, time.Now())) })
	}, 1000, 800)
	if !tt.HasText("line 099") {
		t.Fatal("initial end not visible")
	}
	tt.Scroll(300, 150, 0, -700)
	tt.Frame()
	if tt.HasText("line 099") {
		t.Fatal("scroll up did not move output")
	}
	s.Lines = append(s.Lines, "new line")
	tt.Frame()
	if tt.HasText("new line") {
		t.Fatal("new output forced the reader back to the end")
	}
}

func TestRawAndRunnerOutputScroll(t *testing.T) {
	for _, mode := range []string{"raw", "runner"} {
		t.Run(mode, func(t *testing.T) {
			a := newApp()
			j := &act.Job{}
			for i := range 100 {
				line := fmt.Sprintf("entry %03d", i)
				j.Raw = append(j.Raw, line)
				a.run.Runner = append(a.run.Runner, line)
			}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(designTheme(c))
				ui.Column(c).Fill().Children(func() {
					if mode == "raw" {
						a.rawLogView(c, j)
					} else {
						a.linesView(c, a.run.Runner)
					}
				})
			}, 600, 400)
			if !tt.HasText("entry 099") {
				t.Fatal("output did not start at end")
			}
			tt.Scroll(200, 200, 0, -700)
			tt.Frame()
			if tt.HasText("entry 099") {
				t.Fatal("output did not scroll")
			}
			j.Raw = append(j.Raw, "new entry")
			a.run.Runner = append(a.run.Runner, "new entry")
			tt.Frame()
			if tt.HasText("new entry") {
				t.Fatal("new output forced scroll to end")
			}
		})
	}
}
