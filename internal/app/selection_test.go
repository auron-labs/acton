package app

import (
	"acton/internal/act"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestStepOutputSelectsAcrossLines(t *testing.T) {
	a := newTestSession()
	step := &act.Step{Name: "test", Lines: []string{"first line", "second line", "third line"}}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.stepOutput(c, rowRef{step: step})
	}, 600, 300)
	tt.Frame()
	first, ok1 := tt.Find("first line")
	last, ok2 := tt.Find("third line")
	if !ok1 || !ok2 {
		t.Fatalf("lines not rendered: %q", tt.Texts())
	}
	tt.Press(first.X+1, first.Y+first.H/2)
	tt.Move(last.X+last.W-1, last.Y+last.H/2)
	tt.Release(last.X+last.W-1, last.Y+last.H/2)
	tt.Command("copy")
	got := tt.Clipboard()
	if !strings.Contains(got, "first line") || !strings.Contains(got, "second line") || !strings.Contains(got, "third line") {
		t.Fatalf("copied %q", got)
	}
}
