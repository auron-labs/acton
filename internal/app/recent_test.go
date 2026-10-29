package app

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPushRecentDedupesAndLimits(t *testing.T) {
	var list []string
	for i := range 7 {
		list = pushRecent(list, "/p"+strconv.Itoa(i))
	}
	list = pushRecent(list, "/p4")
	want := []string{"/p4", "/p6", "/p5", "/p3", "/p2"}
	if len(list) != len(want) {
		t.Fatalf("got %v, want %v", list, want)
	}
	for i := range want {
		if list[i] != want[i] {
			t.Fatalf("got %v, want %v", list, want)
		}
	}
}

func TestRecentPersistsAndSkipsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "recent.json")
	keep, gone := t.TempDir(), filepath.Join(t.TempDir(), "gone")

	a := newApp()
	a.recentPath = path
	a.setDir(gone)
	a.setDir(keep)

	b := newApp()
	b.recent = loadRecent(path)
	if got := b.recentProjects(); len(got) != 1 || got[0] != keep {
		t.Fatalf("recentProjects = %v, want [%s]", got, keep)
	}
}

func TestRecentListOnFirstScreen(t *testing.T) {
	dir := t.TempDir()
	a := newApp()
	a.recent = []string{dir}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(designTheme(c))
		a.view(c)
	}, 1120, 720)

	if !tt.HasText("Recent projects") || !tt.HasText(abbrevHome(dir)) {
		t.Fatalf("recent list missing: %q", tt.Texts())
	}
	if err := tt.Click(abbrevHome(dir)); err != nil {
		t.Fatalf("click recent: %v", err)
	}
	tt.Frame()
	if a.settings.Dir != dir {
		t.Fatalf("Dir = %q, want %q", a.settings.Dir, dir)
	}
}
