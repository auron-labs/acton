package app

import (
	"actgui/internal/act"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

const slowStep = 30 * time.Second

func stepDuration(s *act.Step) time.Duration {
	d, _ := time.ParseDuration(s.Duration())
	return max(0, d)
}

func (a *App) stepSummary(c *ui.Context, j *act.Job) {
	t := c.Theme()
	passed, failed, slow := 0, 0, 0
	for _, s := range j.Steps {
		if s.Status == act.StatusSuccess {
			passed++
		}
		if s.Status == act.StatusFailure {
			failed++
		}
		if stepDuration(s) > slowStep {
			slow++
		}
	}
	ui.Row(c).FillWidth().Shrink(0).Wrap().Gap(t.Space(2)).Padding(t.Space(2.5), t.Space(3)).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(sectionBorder(c)).Children(func() {
		components.Badge(c, fmt.Sprintf("%d steps", len(j.Steps)))
		components.Badge(c, fmt.Sprintf("%d passed", passed), components.BadgeOptions{Tone: components.BadgeSuccess})
		components.Badge(c, fmt.Sprintf("%d failed", failed), components.BadgeOptions{Tone: components.BadgeDanger})
		if slow > 0 {
			components.Badge(c, fmt.Sprintf("%d slow · over 30s", slow), components.BadgeOptions{Tone: components.BadgeWarning})
		}
		if components.Button(c, "Collapse all", components.ButtonOutline).Clicked() {
			for _, s := range j.Steps {
				s.Open = false
			}
		}
		if failed > 0 && components.Button(c, "Jump to failure", components.ButtonOutline).TextColor(t.Danger).Clicked() {
			a.jumpToFailure(j)
		}
	})
}

func (a *App) jumpToFailure(j *act.Job) {
	for _, s := range j.Steps {
		if s.Status != act.StatusFailure {
			continue
		}
		s.Open = true
		for i, row := range flatRows(j) {
			if row.step == s {
				a.logUI.list.ScrollTo(i, ui.Start)
				return
			}
		}
	}
}

func timelineColumns(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Shrink(0).Gap(t.Space(2)).Padding(t.Space(3), t.Space(6.5), t.Space(1.5), t.Space(9)).Children(func() {
		components.Text(c, "STEP").FontSize(t.Rem(10.0 / 13)).TextColor(t.TextMuted).Grow(1).Basis(0).MinWidth(0)
		components.Text(c, "TIME SPENT").FontSize(t.Rem(10.0 / 13)).TextColor(t.TextMuted).Grow(1).Basis(0).MinWidth(0).SingleLine().Ellipsis("…")
		components.Text(c, "DURATION").FontSize(t.Rem(10.0 / 13)).TextColor(t.TextMuted).Width(t.Space(21)).SingleLine()
	})
}

func (a *App) stepHeaderRow(c *ui.Context, s *act.Step, longest time.Duration) {
	t := c.Theme()
	color := t.Success
	switch {
	case s.Status == act.StatusFailure:
		color = t.Danger
	case s.Status == act.StatusRunning:
		color = t.Accent
	case stepDuration(s) > slowStep:
		color = t.Warning
	case s.Status == act.StatusSkipped || s.Status == act.StatusCancelled:
		color = t.TextMuted
	}
	row := ui.ButtonBase(c).FillWidth().Padding(t.Space(1.5), t.Space(2.5)).Gap(t.Space(2)).AlignItems(ui.Center).Radius(t.Radius)
	if s.Status == act.StatusFailure {
		row.Background(t.Danger.Alpha(.1))
	} else if row.Hovered() {
		row.Background(t.Text.Alpha(.04))
	}
	row.Label("Step " + s.Name).Cursor(ui.CursorPointer).OnClick(func() { s.Open = !s.Open })
	row.Children(func() {
		statusIcon(c, s.Status, 4)
		components.Text(c, s.Name).SingleLine().Grow(1).Basis(0).MinWidth(0).Ellipsis("…")
		bar := ui.Box(c).Grow(1).Basis(0).MinWidth(0).Height(t.Space(1)).Label("Relative duration of " + s.Name)
		fraction := float32(0)
		if longest > 0 {
			fraction = min(1, float32(float64(stepDuration(s))/float64(longest)))
		}
		bar.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, sectionBorder(c), r.H/2)
			if r.W > 0 {
				p.Fill(ui.Rect{X: r.X, Y: r.Y, W: min(r.W, max(t.Space(.5), r.W*fraction)), H: r.H}, color, r.H/2)
			}
		})
		components.Text(c, s.Duration()).FontSize(t.Rem(11.0 / 13)).TextColor(color).Font("JetBrains Mono").SingleLine().Width(t.Space(21)).Shrink(0)
		chevron := lucide.ChevronRight()
		if s.Open {
			chevron = lucide.ChevronDown()
		}
		components.Icon(c, chevron).Size(t.Space(3.5), t.Space(3.5)).TextColor(t.TextMuted).Shrink(0)
	})
}

func stepOutputHeader(c *ui.Context, s *act.Step) {
	t := c.Theme()
	ui.Row(c).FillWidth().Padding(t.Space(2), t.Space(3), t.Space(2), t.Space(9)).Gap(t.Space(2)).AlignItems(ui.Center).Background(ui.Hex("#07090C")).Children(func() {
		components.Text(c, "OUTPUT").FontSize(t.Rem(10.0 / 13)).TextColor(ui.Hex("#C6CFDA")).Grow(1)
		if components.Button(c, "Copy", components.ButtonGhost).Label("Copy output for " + s.Name).TextColor(ui.Hex("#C6CFDA")).Clicked() {
			c.WriteClipboard(strings.Join(s.Lines, "\n"))
		}
	})
}
