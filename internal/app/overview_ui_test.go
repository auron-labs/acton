package app

import (
	"acton/internal/act"
	"fmt"
	"image/color"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestOverviewNavigationAndAnnotations(t *testing.T) {
	a := newTestSession()
	a.settings.Dir = "/tmp/project"
	a.workflows = []string{"ci.yml"}
	j := &act.Job{Key: "build", Name: "build", Annotations: []act.Annotation{{Severity: "error", File: "src/main.go", Line: "12", Message: "broken"}}}
	a.run.Jobs = []*act.Job{j}
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(designTheme(c)); a.app.view(c) }, 1200, 800)
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
	a := newTestSession()
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
			a := newTestSession()
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

func TestStepOutputThemeAndCopy(t *testing.T) {
	for _, problems := range []bool{false, true} {
		t.Run(fmt.Sprintf("problems=%t", problems), func(t *testing.T) {
			a := newTestSession()
			s := &act.Step{Name: "test", Lines: []string{"first", "", "last"}}
			if problems {
				s.Annotations = []act.Annotation{{Severity: "error", Message: "broken"}}
			}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(designTheme(c))
				windowTheme := c.Theme()
				a.stepOutput(c, rowRef{step: s})
				if c.Theme() != windowTheme {
					t.Fatal("output theme escaped into the rest of the window")
				}
			}, 600, 400)
			tt.SetDark(false)
			hoverColor := func() color.RGBA {
				t.Helper()
				bounds, ok := tt.Find("Copy output for test")
				if !ok {
					t.Fatal("copy action is missing")
				}
				tt.Move(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
				tt.Advance(time.Second)
				return tt.Image().RGBAAt(int(bounds.X+4), int(bounds.Y+bounds.H/2))
			}
			var problemHover color.RGBA
			if problems {
				problemHover = hoverColor()
				if !tt.HasText("broken") {
					t.Fatal("annotated step did not open on Problems")
				}
				if err := tt.Click("Log"); err != nil {
					t.Fatal(err)
				}
			}
			lightHover := hoverColor()
			if problems && lightHover == problemHover {
				t.Fatal("log controls inherited the light Problems theme")
			}
			for _, dark := range []bool{false, true} {
				tt.SetDark(dark)
				if got := hoverColor(); got != lightHover {
					t.Fatalf("dark log controls changed with window appearance: light=%v dark=%v", lightHover, got)
				}
				if err := tt.Click("Copy output for test"); err != nil {
					t.Fatal(err)
				}
				tt.SetClipboard("")
				tt.Key(0, ui.KeyEnter)
				if tt.Clipboard() != "first\n\nlast" {
					t.Fatalf("keyboard copy lost output: %q", tt.Clipboard())
				}
			}
			if problems {
				tt.SetDark(false)
				if err := tt.Click("Problems 1"); err != nil {
					t.Fatal(err)
				}
				if !tt.HasText("broken") || hoverColor() != problemHover {
					t.Fatal("Problems did not restore the window theme")
				}
			}
		})
	}
}
