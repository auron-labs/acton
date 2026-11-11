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

func (s *Session) stepSummary(c *ui.Context, j *act.Job) {
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
			if s.logUI.filter == f {
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
				s.logUI.filter = f
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
			s.jumpToFailure(j)
		}
	})
}

func (s *Session) jumpToFailure(j *act.Job) {
	var failed *act.Step
	walkSteps(j.Steps, func(s *act.Step) {
		if failed == nil && len(s.Children) == 0 && s.Status == act.StatusFailure {
			failed = s
		}
	})
	if failed == nil {
		return
	}
	s.logUI.filter = filterAll
	for s := failed; s != nil; s = s.Parent {
		s.Open = true
		s.UserToggled = true
	}
	for i, row := range flatRows(j, filterAll) {
		if row.step == failed {
			s.logUI.list.ScrollTo(i, ui.Start)
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
	if !j.Started.IsZero() {
		start = j.Started
		end = j.Ended
		if end.IsZero() {
			end = now
		}
	}
	span := end.Sub(start)
	if start.IsZero() || span <= 0 {
		span = time.Second
	}
	tick := time.Second
	for _, candidate := range []time.Duration{time.Second / 5, time.Second / 2, time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 5 * time.Hour, 10 * time.Hour, 24 * time.Hour} {
		tick = candidate
		if span/tick <= 6 {
			break
		}
	}
	return timeAxis{start: start, span: span, tick: tick, now: now}
}

func timelineColumns(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Shrink(0).Height(28).Gap(8).Padding(0, 12).AlignItems(ui.Center).Children(func() {
		components.Text(c, "#").Width(32).TextAlign(ui.End).Font("JetBrains Mono").FontSize(10).TextColor(t.TextMuted).Shrink(0)
		ui.Box(c).Width(16).Shrink(0)
		components.Text(c, "STEP").Grow(1).Basis(0).MinWidth(0).FontSize(10).TextColor(t.TextMuted)
		components.Text(c, "SHARE OF JOB TIME").Grow(1).Basis(0).MinWidth(0).FontSize(10).TextColor(t.TextMuted).SingleLine()
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

func stepHeaderRow(c *ui.Context, row rowRef, axis timeAxis) {
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
				components.Badge(c, fmt.Sprintf("%d steps", len(s.Children))).Background(t.SurfaceHover).TextColor(t.TextMuted).Font("JetBrains Mono").FontSize(10).Shrink(0)
			}
		})
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Height(30).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Grow(1).MinWidth(0).Height(30).Label("Share of job time for " + s.Name).Draw(func(p *ui.Painter, r ui.Rect) { drawShare(p, r, s, axis, t) })
			components.Text(c, shareLabel(s.Elapsed(axis.now), axis.span)).FontSize(10).TextColor(t.TextMuted).Width(32).Shrink(0)
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

func stepOutputHeader(c *ui.Context, s *act.Step, tab *int) {
	ui.Row(c).FillWidth().Height(34).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		if len(s.Annotations) > 0 {
			ui.Segmented(c, tab, fmt.Sprintf("Problems %d", len(s.Annotations)), "Log").Label("Step output mode")
		} else {
			components.Text(c, "Log").FontSize(11)
		}
		ui.Box(c).Grow(1)
		copyColor := c.Theme().Text
		if len(s.Annotations) > 0 && *tab == 0 {
			copyColor = c.Theme().TextMuted
		}
		if components.Button(c, "Copy", components.ButtonGhost).Label("Copy output for " + s.Name).FontSize(11).TextColor(copyColor).Clicked() {
			c.WriteClipboard(strings.Join(s.Lines, "\n"))
		}
	})
}

func (s *Session) stepOutput(c *ui.Context, row rowRef) {
	step := row.step
	if s.logUI.output == nil {
		s.logUI.output = map[*act.Step]*components.ListState{}
	}
	state := s.logUI.output[step]
	if state == nil {
		state = &components.ListState{}
		s.logUI.output[step] = state
	}
	state.FollowEnd = step.Status == act.StatusRunning
	if s.logUI.tabs == nil {
		s.logUI.tabs = map[*act.Step]*int{}
	}
	tab := s.logUI.tabs[step]
	if tab == nil {
		tab = new(int)
		s.logUI.tabs[step] = tab
	}
	lines := 0
	for _, line := range step.Lines {
		lines += strings.Count(line, "\n") + 1
	}
	height := min(320, max(100, float32(max(lines, len(step.Annotations)*3))*18+42))
	outputTheme := logTheme(c)
	background := outputTheme.Background
	if len(step.Annotations) > 0 && *tab == 0 {
		outputTheme = c.Theme()
		background = panelBackground(c)
	}
	ui.Column(c).FillWidth().Padding(3, 12, 8, 76+float32(row.depth)*20).Children(func() {
		ui.Themed(c, outputTheme).FillWidth().Height(height).Shrink(0).Background(background).Radius(6).Border(1, outputTheme.Border).Clip().Children(func() {
			stepOutputHeader(c, step, tab)
			if len(step.Annotations) > 0 && *tab == 0 {
				components.ScrollView(c, components.ScrollViewOptions{}).Fill().MinHeight(0).Children(func() { annotationList(c, step.Annotations) })
			} else {
				components.List(c, state, len(step.Lines), func(i int) { s.logLineRow(c, rowRef{step: step, line: step.Lines[i]}) }).Fill().MinHeight(0).Selectable()
			}
		})
	})
}

func drawShare(p *ui.Painter, r ui.Rect, s *act.Step, axis timeAxis, t *ui.Theme) {
	if r.W <= 0 || axis.span <= 0 {
		return
	}
	x := r.X
	var draw func(*act.Step)
	draw = func(step *act.Step) {
		if len(step.Children) > 0 {
			for _, child := range step.Children {
				draw(child)
			}
			return
		}
		d := step.Elapsed(axis.now)
		if d <= 0 {
			return
		}
		w := min(r.X+r.W-x, max(2, r.W*float32(float64(d)/float64(axis.span))))
		if w <= 0 {
			return
		}
		color := t.TextMuted
		switch {
		case step.Status == act.StatusFailure:
			color = t.Danger
		case step.Status == act.StatusRunning:
			color = t.Accent
		case d > slowStep:
			color = t.Warning
		case step.Status == act.StatusSuccess:
			color = t.Success
		}
		p.Fill(ui.Rect{X: x, Y: r.Y + (r.H-6)/2, W: w, H: 6}, color, 2)
		x += w + 1
	}
	draw(s)
}

func durationSummary(c *ui.Context, j *act.Job, axis timeAxis) {
	t := c.Theme()
	ui.Column(c).FillWidth().Shrink(0).Padding(12, 24).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(sectionBorder(c)).Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			components.Text(c, "WHERE THE TIME WENT").FontSize(10).TextColor(t.TextMuted).Grow(1)
			components.Text(c, act.FormatDuration(axis.span)+" total").FontSize(10).TextColor(t.TextMuted)
		})
		ui.Box(c).FillWidth().Height(14).Label("Job duration breakdown").Draw(func(p *ui.Painter, r ui.Rect) {
			drawShare(p, r, &act.Step{Children: j.Steps}, axis, t)
		})
		var longest *act.Step
		for _, s := range j.Steps {
			if longest == nil || s.Elapsed(axis.now) > longest.Elapsed(axis.now) {
				longest = s
			}
		}
		if longest != nil && longest.Elapsed(axis.now) > 0 {
			components.Text(c, longest.Name+" · "+act.FormatDuration(longest.Elapsed(axis.now))+" · "+shareLabel(longest.Elapsed(axis.now), axis.span)).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		}
	})
}

func shareLabel(d, total time.Duration) string {
	if d <= 0 || total <= 0 {
		return "0%"
	}
	pct := float64(d) * 100 / float64(total)
	if pct < 1 {
		return "<1%"
	}
	return fmt.Sprintf("%.0f%%", pct)
}
