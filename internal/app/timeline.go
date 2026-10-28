package app

import (
	"acton/internal/act"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

const slowStep = 30 * time.Second

type stepFilter int

const (
	filterAll stepFilter = iota
	filterPassed
	filterFailed
	filterSlow
)

func stepDuration(s *act.Step) time.Duration { return s.Elapsed(time.Now()) }

func (f stepFilter) matches(s *act.Step) bool {
	if len(s.Children) > 0 {
		for _, child := range s.Children {
			if f.matches(child) {
				return true
			}
		}
		return false
	}
	switch f {
	case filterAll:
		return true
	case filterPassed:
		return s.Status == act.StatusSuccess
	case filterFailed:
		return s.Status == act.StatusFailure
	case filterSlow:
		return stepDuration(s) > slowStep
	}
	return false
}

func walkSteps(steps []*act.Step, visit func(*act.Step)) {
	for _, s := range steps {
		visit(s)
		walkSteps(s.Children, visit)
	}
}

func (a *App) stepSummary(c *ui.Context, j *act.Job) {
	t := c.Theme()
	total, passed, failed, slow := 0, 0, 0, 0
	walkSteps(j.Steps, func(s *act.Step) {
		if len(s.Children) > 0 {
			return
		}
		total++
		if s.Status == act.StatusSuccess {
			passed++
		}
		if s.Status == act.StatusFailure {
			failed++
		}
		if filterSlow.matches(s) {
			slow++
		}
	})
	ui.Row(c).FillWidth().Shrink(0).Wrap().Gap(8).Padding(10, 12).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(sectionBorder(c)).Children(func() {
		for i, label := range []string{fmt.Sprintf("All %d steps", total), fmt.Sprintf("%d passed", passed), fmt.Sprintf("%d failed", failed), fmt.Sprintf("%d slow · over 30s", slow)} {
			f := stepFilter(i)
			button := components.Button(c, label, components.ButtonGhost).FontSize(11).Radius(12).Padding(3, 9)
			if a.logUI.filter == f {
				button.Background(t.SurfaceHover).Border(1, t.Border).Label(label + " (selected)")
			}
			switch f {
			case filterAll:
				button.TextColor(t.Text)
			case filterPassed:
				button.TextColor(t.Success)
			case filterFailed:
				button.TextColor(t.Danger)
			case filterSlow:
				button.TextColor(t.Warning)
			}
			button.OnClick(func() {
				a.logUI.filter = f
				if f != filterAll {
					walkSteps(j.Steps, func(s *act.Step) {
						if len(s.Children) > 0 && f.matches(s) {
							s.Open = true
							s.UserToggled = true
						}
					})
				}
			})
		}
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(20, 12).Draw(func(p *ui.Painter, r ui.Rect) { drawBracket(p, r, t.Border) })
			components.Text(c, "Composite action").FontSize(11).TextColor(t.TextMuted).SingleLine()
			components.Icon(c, lucide.CloudDownload()).Size(13, 13).TextColor(t.TextMuted)
			components.Text(c, "Action cloned this run").FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
		if components.Button(c, "Collapse all", components.ButtonOutline).FontSize(11).Clicked() {
			walkSteps(j.Steps, func(s *act.Step) { s.Open = false; s.UserToggled = true })
		}
		if failed > 0 && components.Button(c, "Jump to failure", components.ButtonOutline).FontSize(11).TextColor(t.Danger).Clicked() {
			a.jumpToFailure(j)
		}
	})
}

func (a *App) jumpToFailure(j *act.Job) {
	var failed *act.Step
	walkSteps(j.Steps, func(s *act.Step) {
		if failed == nil && len(s.Children) == 0 && s.Status == act.StatusFailure {
			failed = s
		}
	})
	if failed == nil {
		return
	}
	a.logUI.filter = filterAll
	for s := failed; s != nil; s = s.Parent {
		s.Open = true
		s.UserToggled = true
	}
	for i, row := range flatRows(j, filterAll) {
		if row.step == failed {
			a.logUI.list.ScrollTo(i, ui.Start)
			return
		}
	}
}

type timeAxis struct {
	start      time.Time
	span, tick time.Duration
	now        time.Time
}

func jobTimeAxis(j *act.Job, now time.Time) timeAxis {
	var start, end time.Time
	walkSteps(j.Steps, func(s *act.Step) {
		if s.Started.IsZero() {
			return
		}
		if start.IsZero() || s.Started.Before(start) {
			start = s.Started
		}
		until := s.Ended
		if until.IsZero() {
			until = now
		}
		if until.After(end) {
			end = until
		}
	})
	span := max(time.Second, end.Sub(start))
	tick := time.Second
	for _, candidate := range []time.Duration{time.Second / 5, time.Second / 2, time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 5 * time.Hour, 10 * time.Hour, 24 * time.Hour} {
		tick = candidate
		if span/tick <= 6 {
			break
		}
	}
	return timeAxis{start: start, span: span, tick: tick, now: now}
}

func axisLabel(d time.Duration) string {
	if d > 0 && d%time.Second != 0 {
		return act.FormatDuration(d)
	}
	seconds := int(d / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds%60 == 0 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
}

func timelineColumns(c *ui.Context, axis timeAxis) {
	t := c.Theme()
	ui.Row(c).FillWidth().Shrink(0).Height(28).Gap(8).Padding(0, 12).AlignItems(ui.Center).Children(func() {
		components.Text(c, "#").Width(32).TextAlign(ui.End).Font("JetBrains Mono").FontSize(10).TextColor(t.TextMuted).Shrink(0)
		ui.Box(c).Width(16).Shrink(0)
		components.Text(c, "STEP").Grow(1).Basis(0).MinWidth(0).FontSize(10).TextColor(t.TextMuted)
		ui.Box(c).Grow(1).Basis(0).MinWidth(0).Height(28).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
			for d := time.Duration(0); d <= axis.span; d += axis.tick {
				x := r.X + r.W*float32(float64(d)/float64(axis.span))
				label := axisLabel(d)
				p.Text(max(r.X, min(x+3, r.X+r.W-float32(len(label))*6)), r.Y+7, label, 10, t.TextMuted)
			}
		})
		components.Text(c, "DURATION").Width(64).TextAlign(ui.End).FontSize(10).TextColor(t.TextMuted).SingleLine().Shrink(0)
		ui.Box(c).Width(14).Shrink(0)
	})
}

func drawBracket(p *ui.Painter, r ui.Rect, color ui.Color) {
	y := r.Y + r.H/2
	p.Fill(ui.Rect{X: r.X, Y: y - 1, W: r.W, H: 2}, color, 0)
	for _, x := range []float32{r.X, r.X + r.W - 2} {
		p.Fill(ui.Rect{X: x, Y: y - 5, W: 2, H: 10}, color, 0)
	}
}

func (a *App) stepHeaderRow(c *ui.Context, row rowRef, axis timeAxis) {
	s := row.step
	t := c.Theme()
	color := t.TextMuted
	switch {
	case s.Status == act.StatusFailure:
		color = t.Danger
	case s.Status == act.StatusRunning:
		color = t.Accent
	case s.Status == act.StatusSuccess && s.Elapsed(axis.now) > slowStep:
		color = t.Warning
	case s.Status == act.StatusSuccess:
		color = t.Success
	}
	composite := len(s.Children) > 0
	button := ui.ButtonBase(c).FillWidth().Height(30).Padding(0, 12).Gap(8).AlignItems(ui.Center).Radius(6)
	if s.Status == act.StatusFailure {
		button.Background(t.Danger.Alpha(.12))
	} else if button.Hovered() {
		button.Background(t.Text.Alpha(.04))
	}
	button.Label("Step " + s.Name).Expanded(s.Open).Cursor(ui.CursorPointer).OnClick(func() { s.Open = !s.Open; s.UserToggled = true })
	button.Children(func() {
		components.Text(c, row.number).Font("JetBrains Mono").FontSize(11).TextColor(t.TextMuted).TextAlign(ui.End).Width(32).Shrink(0).SingleLine()
		// Indentation lives inside the name column so every track stays aligned.
		ui.Box(c).Size(16, 30).Shrink(0).Children(func() {
			if row.depth == 0 {
				ui.Row(c).Height(30).AlignItems(ui.Center).Children(func() {
					if composite {
						components.Icon(c, lucide.Layers()).Size(14, 14).TextColor(t.TextMuted)
					} else {
						statusIcon(c, s.Status, 3.5)
					}
				})
			}
		})
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Height(30).Gap(8).AlignItems(ui.Center).Clip().Children(func() {
			if row.depth > 0 {
				ui.Box(c).Width(float32(row.depth-1)*20+12).Height(30).Shrink(0).BorderWidth(0, 0, 0, 1).BorderColor(t.Border)
				if composite {
					components.Icon(c, lucide.Layers()).Size(13, 13).TextColor(t.TextMuted)
				} else {
					statusIcon(c, s.Status, 3.25)
				}
			}
			name := components.Text(c, s.Name).SingleLine().MinWidth(0).Ellipsis("…")
			if s.Status == act.StatusFailure {
				name.TextColor(t.Danger)
			}
			if s.Source != "" {
				components.Text(c, s.Source).Font("JetBrains Mono").FontSize(11).TextColor(t.TextMuted).SingleLine().MinWidth(0).Shrink(10).Ellipsis("…")
			}
			if s.Fetched {
				components.Icon(c, lucide.CloudDownload()).Size(13, 13).TextColor(t.TextMuted).Shrink(0)
			}
			if composite {
				components.Badge(c, fmt.Sprintf("%d steps", len(s.Children))).Font("JetBrains Mono").FontSize(10).Shrink(0)
			}
		})
		ui.Box(c).Grow(1).Basis(0).MinWidth(0).Height(30).Label("Timeline of " + s.Name).Draw(func(p *ui.Painter, r ui.Rect) {
			for d := time.Duration(0); d <= axis.span; d += axis.tick {
				x := r.X + r.W*float32(float64(d)/float64(axis.span))
				p.Fill(ui.Rect{X: x, Y: r.Y, W: 1, H: r.H}, sectionBorder(c).Alpha(.5), 0)
			}
			if s.Started.IsZero() || axis.start.IsZero() || r.W <= 0 {
				return
			}
			offset := float32(float64(s.Started.Sub(axis.start)) / float64(axis.span))
			width := float32(float64(s.Elapsed(axis.now)) / float64(axis.span))
			x := min(max(0, r.W-3), max(0, r.W*offset))
			bar := ui.Rect{X: r.X + x, Y: r.Y + (r.H-6)/2, W: min(r.W-x, max(3, r.W*width)), H: 6}
			if composite {
				drawBracket(p, bar, t.Border)
			} else {
				p.Fill(bar, color, 3)
			}
		})
		durationColor := t.TextMuted
		if s.Status == act.StatusFailure || s.Status == act.StatusRunning || s.Status == act.StatusSuccess && s.Elapsed(axis.now) > slowStep {
			durationColor = color
		}
		components.Text(c, s.Duration()).FontSize(11).TextColor(durationColor).Font("JetBrains Mono").SingleLine().TextAlign(ui.End).Width(64).Shrink(0)
		chevron := lucide.ChevronRight()
		if s.Open {
			chevron = lucide.ChevronDown()
		}
		components.Icon(c, chevron).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
	})
}

func stepOutputHeader(c *ui.Context, s *act.Step) {
	ui.Row(c).FillWidth().Height(34).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		components.Text(c, "OUTPUT").FontSize(10).TextColor(ui.Hex("#8C96A5")).Grow(1)
		if components.Button(c, "Copy", components.ButtonGhost).Label("Copy output for " + s.Name).FontSize(11).TextColor(ui.Hex("#C6CFDA")).Clicked() {
			c.WriteClipboard(strings.Join(s.Lines, "\n"))
		}
	})
}

func (a *App) stepOutput(c *ui.Context, row rowRef) {
	s := row.step
	if a.logUI.output == nil {
		a.logUI.output = map[*act.Step]*components.ListState{}
	}
	state := a.logUI.output[s]
	if state == nil {
		state = &components.ListState{}
		a.logUI.output[s] = state
	}
	state.FollowEnd = s.Status == act.StatusRunning
	if state.FollowEnd {
		state.ScrollToEnd()
	}
	lines := 0
	for _, line := range s.Lines {
		lines += strings.Count(line, "\n") + 1
	}
	height := min(320, float32(lines)*18+42)
	ui.Column(c).FillWidth().Padding(3, 12, 8, 76+float32(row.depth)*20).Children(func() {
		ui.Column(c).FillWidth().Height(height).Shrink(0).Background(ui.Hex("#07090C")).Radius(6).Border(1, c.Theme().Border).Clip().Children(func() {
			stepOutputHeader(c, s)
			components.List(c, state, len(s.Lines), func(i int) { a.logLineRow(c, rowRef{step: s, line: s.Lines[i]}) }).Fill().MinHeight(0)
		})
	})
}
