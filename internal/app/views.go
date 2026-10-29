package app

import (
	"acton/internal/act"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/plain"
)

// view builds the window content: the resizable configuration | run split.
// The shell header carries the project picker and run controls.
func (a *App) view(c *ui.Context) {
	if a.running {
		// Tick elapsed/duration labels while a run is live.
		c.After(time.Second)
	}
	ui.Column(c).Fill().Background(panelBackground(c)).Padding(plain.HeaderHeight(c), 0, 0, 0).Children(func() {
		ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
			ui.Divider(c)
			if a.settings.Dir == "" {
				a.emptyRunView(c)
				return
			}
			if len(a.workflows) == 0 {
				a.noWorkflowsView(c)
				return
			}
			components.Split(c, &a.cfgW, func() { a.configView(c) }, func() { a.runView(c) },
				components.SplitOptions{FirstMin: 260, SecondMin: 160 + 1 + 260, Default: 320}).Fill()
		})
	})
}

// headerLeading is the shell header's left slot: the project picker and the
// .actrc badge.
func (a *App) headerLeading(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).MinWidth(0).Children(func() {
		folder := components.Button(c, "", components.ButtonSecondary).Label("Choose project folder").Disabled(a.running)
		folder.Children(func() {
			ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).MinWidth(0).Children(func() {
				components.Icon(c, lucide.FolderOpen()).Size(t.Space(4), t.Space(4)).Shrink(0)
				label := "Choose project…"
				if a.settings.Dir != "" {
					label = abbrevHome(a.settings.Dir)
				}
				components.Text(c, label).SingleLine().MaxWidth(400).Ellipsis("…")
			})
		})
		if folder.Clicked() {
			a.pickFolder()
		}
		if n := len(a.settings.ActrcFiles); n > 0 {
			badge := components.Badge(c, fmt.Sprintf(".actrc ×%d", n), components.BadgeOptions{Tone: components.BadgeNeutral, Icon: lucide.FileText()})
			components.Tooltip(c, badge, "Defaults loaded from:\n"+strings.Join(a.settings.ActrcFiles, "\n"))
		}
	})
}

// headerActions is the shell header's right slot: run status and the
// Run/Stop button.
func (a *App) headerActions(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		switch st := a.overallStatus(); {
		case a.running:
			components.Text(c, act.FormatDuration(time.Since(a.started))).TextColor(t.TextMuted).Font("JetBrains Mono").SingleLine()
			stop := components.Button(c, "", components.ButtonSecondary).Label("Stop run")
			stop.Children(func() {
				ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					components.Icon(c, lucide.Square()).Size(t.Space(3.5), t.Space(3.5)).TextColor(t.Danger)
					components.Text(c, "Stop").SingleLine()
				})
			})
			if stop.Clicked() {
				a.stopRun()
			}
		default:
			if st.Done() && len(a.run.Jobs)+len(a.run.Runner) > 0 {
				statusIcon(c, st, 4.5)
				components.Text(c, act.FormatDuration(a.elapsed)).TextColor(t.TextMuted).Font("JetBrains Mono").SingleLine()
			}
			run := components.Button(c, "", components.ButtonPrimary).Label("Run workflow").Disabled(a.settings.Dir == "" || len(a.workflows) == 0)
			run.Children(func() {
				ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					components.Icon(c, lucide.Play()).Size(t.Space(3.5), t.Space(3.5))
					components.Text(c, "Run").SingleLine()
				})
			})
			if run.Clicked() {
				a.startRun()
			}
		}
	})
}

// runView is the right pane: jobs list beside the selected job's log, or a
// placeholder before the first run.
func (a *App) runView(c *ui.Context) {
	t := c.Theme()
	if a.runErr != "" {
		ui.Row(c).FillWidth().Shrink(0).Padding(t.Space(1.5), t.Space(3)).Gap(t.Space(2)).AlignItems(ui.Center).
			Background(t.Danger.Alpha(.1)).Children(func() {
			components.Icon(c, lucide.CircleAlert()).Size(t.Space(4), t.Space(4)).TextColor(t.Danger).Shrink(0)
			components.Text(c, a.runErr).TextColor(t.Danger).FontSize(12)
		})
	}
	if len(a.run.Jobs) == 0 && len(a.run.Runner) == 0 {
		a.emptyRunView(c)
		return
	}
	components.Split(c, &a.jobW, func() { a.jobsPane(c) }, func() {
		if a.selJob == act.RunnerJobKey || (a.selJob == "" && a.selectedJob() == nil && len(a.run.Runner) > 0) {
			a.runnerPane(c)
		} else if j := a.selectedJob(); j != nil {
			a.logPane(c, j)
		}
	}, components.SplitOptions{FirstMin: 160, SecondMin: 260, Default: 220}).Fill()
}

// emptyRunView is the centered placeholder shown before any output exists.
func (a *App) emptyRunView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(t.Space(3)).Padding(t.Space(6)).Children(func() {
		if a.settings.Dir == "" {
			components.Icon(c, lucide.FolderOpen()).Size(t.Space(11), t.Space(11)).TextColor(t.TextMuted)
			components.Text(c, "Select a project").FontSize(t.Rem(22.0 / 13)).Bold()
			components.Text(c, "Pick a project folder to run its workflows with act").TextColor(t.TextMuted)
			choose := components.Button(c, "", components.ButtonSecondary).Label("Choose project folder")
			choose.Children(func() {
				ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					components.Icon(c, lucide.FolderOpen()).Size(t.Space(3.5), t.Space(3.5))
					components.Text(c, "Choose project…").SingleLine()
				})
			})
			if choose.Clicked() {
				a.pickFolder()
			}
			a.recentList(c)
			return
		}
		components.Icon(c, lucide.SquareTerminal()).Size(t.Space(11), t.Space(11)).TextColor(t.TextMuted)
		if a.running {
			components.Spinner(c, components.SpinnerSpokes)
			components.Text(c, "Starting act…").TextColor(t.TextMuted)
		} else {
			components.Text(c, "Nothing has run yet").TextColor(t.TextMuted)
			ui.Row(c).FillWidth().MaxWidth(560).Padding(t.Space(3), t.Space(3.5)).Radius(t.Radius).
				Background(ui.Hex("#07090C")).Border(1, t.Border).Children(func() {
				components.Text(c, a.cmdline()).FillWidth().Font("JetBrains Mono").FontSize(t.Rem(12.0 / 13)).TextColor(ui.Hex("#C6CFDA")).Selectable()
			})
			run := components.Button(c, "", components.ButtonPrimary).Label("Run workflow")
			run.Children(func() {
				ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					components.Icon(c, lucide.Play()).Size(t.Space(3.5), t.Space(3.5))
					components.Text(c, "Run").SingleLine()
				})
			})
			if run.Clicked() {
				a.startRun()
			}
		}
	})
}

// recentList shows the recently opened projects under the picker button;
// clicking one reopens it. It renders nothing when there is no history.
func (a *App) recentList(c *ui.Context) {
	dirs := a.recentProjects()
	if len(dirs) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c).FillWidth().MaxWidth(420).Gap(t.Space(0.5)).Margin(t.Space(3), 0, 0, 0).Children(func() {
		components.Text(c, "Recent projects").FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted).SingleLine().Padding(0, t.Space(2), t.Space(1), t.Space(2))
		for _, dir := range dirs {
			row := ui.Row(c.Key("recent:" + dir)).FillWidth().Padding(t.Space(1.5), t.Space(2)).Gap(t.Space(2)).AlignItems(ui.Center).Radius(t.Radius)
			if row.Hovered() {
				row.Background(t.Text.Alpha(.04))
			}
			row.Label("Open " + dir).Cursor(ui.CursorPointer).OnClick(func() { a.setDir(dir) })
			row.Children(func() {
				components.Icon(c, lucide.Folder()).Size(t.Space(4), t.Space(4)).TextColor(t.TextMuted).Shrink(0)
				ui.Row(c).Grow(1).MinWidth(0).Clip().Children(func() {
					components.Text(c, abbrevHome(dir)).SingleLine()
				})
			})
		}
	})
}

// noWorkflowsView is the dedicated full-content state for a project without
// GitHub Actions: the project picker stays in the header while the content
// explains the gap and offers a rescan or a different folder.
func (a *App) noWorkflowsView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(t.Space(4)).Padding(t.Space(6)).Children(func() {
		components.Icon(c, lucide.Workflow()).Size(t.Space(11), t.Space(11)).TextColor(t.TextMuted)
		components.Text(c, "No GitHub Actions found").FontSize(t.Rem(22.0 / 13)).Bold()
		components.Text(c, "This project has no workflow files in .github/workflows.\nAdd a GitHub Actions workflow, then check again.").
			TextColor(t.TextMuted).FillWidth().MaxWidth(560).TextAlign(ui.Center)
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			check := components.Button(c, "", components.ButtonPrimary).Label("Check for workflow files again")
			check.Children(func() {
				ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					components.Icon(c, lucide.RefreshCw()).Size(t.Space(3.5), t.Space(3.5))
					components.Text(c, "Check again").SingleLine()
				})
			})
			if check.Clicked() {
				a.rescanWorkflows()
			}
			choose := components.Button(c, "", components.ButtonSecondary).Label("Choose another project")
			choose.Children(func() {
				ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					components.Icon(c, lucide.FolderOpen()).Size(t.Space(3.5), t.Space(3.5))
					components.Text(c, "Choose another project…").SingleLine()
				})
			})
			if choose.Clicked() {
				a.pickFolder()
			}
		})
	})
}

// jobsPane lists jobs like GitHub's run sidebar: status icon, name, duration.
func (a *App) jobsPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(t.Space(2), t.Space(3)).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			components.Text(c, "Jobs").Bold().SingleLine()
			ui.Spacer(c)
			if !a.running && (len(a.run.Jobs)+len(a.run.Runner) > 0) {
				clearBtn := components.Button(c, "", components.ButtonGhost).Label("Clear run output")
				clearBtn.Size(t.Space(7), t.Space(7)).Padding(0).Children(func() {
					components.Icon(c, lucide.RotateCcw()).Size(t.Space(3.5), t.Space(3.5)).TextColor(t.TextMuted)
				})
				components.Tooltip(c, clearBtn, "Clear run output")
				if clearBtn.Clicked() {
					a.clearRun()
				}
			}
		})
		ui.Divider(c)
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().Padding(t.Space(1.5)).Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(0.5)).Children(func() {
				selected := ""
				if j := a.selectedJob(); j != nil {
					selected = j.Key
				}
				if a.selJob == act.RunnerJobKey {
					selected = act.RunnerJobKey
				}
				for _, j := range a.run.Jobs {
					a.jobRow(c, j, j.Key == selected)
				}
				if len(a.run.Runner) > 0 {
					a.runnerRow(c, selected == act.RunnerJobKey)
				}
			})
		})
	})
}

func (a *App) jobRow(c *ui.Context, j *act.Job, selected bool) {
	t := c.Theme()
	row := ui.Row(c.Key(j.Key)).FillWidth().Padding(t.Space(1.5), t.Space(2)).Gap(t.Space(2)).AlignItems(ui.Center).Radius(t.Radius)
	switch {
	case selected:
		row.Background(t.Accent.Alpha(.18))
	case row.Hovered():
		row.Background(t.Text.Alpha(.04))
	}
	row.Label("Job " + j.Name).Cursor(ui.CursorPointer).OnClick(func() { a.selJob = j.Key })
	row.Children(func() {
		statusIcon(c, j.Status, 4.5)
		ui.Row(c).Grow(1).MinWidth(0).Clip().Children(func() {
			components.Text(c, j.Name).SingleLine()
		})
		if d := j.Duration(); d != "" {
			components.Text(c, d).FontSize(11).TextColor(t.TextMuted).SingleLine().Shrink(0)
		}
	})
}

func (a *App) runnerRow(c *ui.Context, selected bool) {
	t := c.Theme()
	row := ui.Row(c.Key("runner")).FillWidth().Padding(t.Space(1.5), t.Space(2)).Gap(t.Space(2)).AlignItems(ui.Center).Radius(t.Radius)
	switch {
	case selected:
		row.Background(t.Accent.Alpha(.18))
	case row.Hovered():
		row.Background(t.Text.Alpha(.04))
	}
	row.Label("Runner output").Cursor(ui.CursorPointer).OnClick(func() { a.selJob = act.RunnerJobKey })
	row.Children(func() {
		components.Icon(c, lucide.Terminal()).Size(t.Space(4.5), t.Space(4.5)).TextColor(t.TextMuted)
		components.Text(c, "Runner output").SingleLine().Grow(1)
		components.Badge(c, strconv.Itoa(len(a.run.Runner)), components.BadgeOptions{Tone: components.BadgeNeutral})
	})
}

// logPane shows the selected job: header with name/status, then the
// GitHub-style step tree or the raw log.
func (a *App) logPane(c *ui.Context, j *act.Job) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(t.Space(2), t.Space(3)).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			statusIcon(c, j.Status, 5)
			ui.Row(c).Grow(1).MinWidth(0).Gap(t.Space(2)).AlignItems(ui.Center).Clip().Children(func() {
				components.Text(c, j.Name).Bold().SingleLine()
				if d := j.Duration(); d != "" {
					components.Text(c, d).FontSize(11).TextColor(t.TextMuted).Font("JetBrains Mono").SingleLine().Shrink(0)
				}
			})
			ui.Segmented(c, &a.logMode, "Steps", "Raw log").Shrink(0).Label("Log view mode")
		})
		ui.Divider(c)
		if a.logMode == 1 {
			a.rawLogView(c, j)
		} else {
			a.stepSummary(c, j)
			axis := jobTimeAxis(j, time.Now())
			timelineColumns(c, axis)
			a.stepsView(c, j, axis)
		}
	})
}

// runnerPane shows untagged act output (startup, errors, docker host info).
func (a *App) runnerPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(t.Space(2), t.Space(3)).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			components.Icon(c, lucide.Terminal()).Size(t.Space(5), t.Space(5)).TextColor(t.TextMuted)
			components.Text(c, "Runner output").Bold().SingleLine()
		})
		ui.Divider(c)
		a.linesView(c, a.run.Runner)
	})
}

// stepsView keeps each bounded output panel as one virtual row.
func (a *App) stepsView(c *ui.Context, j *act.Job, axis timeAxis) {
	rows := flatRows(j, a.logUI.filter)
	a.logUI.list.Key = func(i int) any { return rows[i].key }
	components.List(c, &a.logUI.list, len(rows), func(i int) {
		r := rows[i]
		switch {
		case r.output:
			a.stepOutput(c, r)
		case r.step != nil:
			a.stepHeaderRow(c, r, axis)
		default:
			a.logLineRow(c, r)
		}
	}).Fill()
}

// rawLogView appends new verbatim lines into a persistent buffer so the read
// only view keeps its scroll position.
func (a *App) rawLogView(c *ui.Context, j *act.Job) {
	rl := a.rawLogFor(j)
	if len(j.Raw) > rl.n {
		var b strings.Builder
		for _, l := range j.Raw[rl.n:] {
			b.WriteString(l)
			b.WriteByte('\n')
		}
		rl.buf.Replace(rl.buf.Len(), rl.buf.Len(), b.String())
		rl.n = len(j.Raw)
	}
	components.TextBoxBuffer(c, rl.buf, components.TextBoxOptions{Multiline: true, ReadOnly: true}).
		Fill().Font("JetBrains Mono").FontSize(12).Label("Raw act output")
}

// linesView renders a plain list of lines (runner output).
func (a *App) linesView(c *ui.Context, lines []string) {
	t := c.Theme()
	components.List(c, &a.logUI.runner, len(lines), func(i int) {
		ui.Row(c).FillWidth().Padding(0, t.Space(3), 0, 0).Children(func() {
			ui.Text(c, lines[i]).Font("JetBrains Mono").FontSize(12).Selectable()
		})
	}).Fill()
}

// logLineRow is one output line beneath a step header (or job level).
func (a *App) logLineRow(c *ui.Context, r rowRef) {
	t := c.Theme()
	indent := float32(12)
	if r.step == nil {
		indent = t.Space(2.5)
	}
	row := ui.Row(c).FillWidth().Padding(0, t.Space(2.5), 0, indent)
	color := t.Text
	errorColor, warningColor := t.Danger, t.Warning
	if r.step != nil {
		row.Background(ui.Hex("#07090C"))
		color = ui.Hex("#C6CFDA")
		errorColor, warningColor = ui.Hex("#F27379"), ui.Hex("#E8A93F")
	}
	switch {
	case strings.HasPrefix(r.line, "❌"), strings.Contains(r.line, "::error"), strings.HasPrefix(strings.ToLower(r.line), "error:"):
		color = errorColor
	case strings.HasPrefix(r.line, "⚠"), strings.Contains(r.line, "::warning"), strings.HasPrefix(strings.ToLower(r.line), "warning:"), strings.HasPrefix(r.line, "tip:"):
		color = warningColor
	case r.step == nil:
		color = t.TextMuted
	}
	row.Children(func() {
		ui.Text(c, r.line).Font("JetBrains Mono").FontSize(12).TextColor(color).Selectable()
	})
}

// rowRef is a header, a bounded output panel, or a job diagnostic.
type rowRef struct {
	step   *act.Step
	line   string
	key    any
	output bool
	number string
	depth  int
}

type (
	orphanKey struct{ idx int }
	outputKey struct{ step *act.Step }
)

func flatRows(j *act.Job, filter stepFilter) []rowRef {
	rows := make([]rowRef, 0, len(j.Items))
	var add func(*act.Step, string, int)
	add = func(s *act.Step, number string, depth int) {
		if !filter.matches(s) {
			return
		}
		row := rowRef{step: s, key: s, number: number, depth: depth}
		rows = append(rows, row)
		if s.Open {
			for i, child := range s.Children {
				add(child, fmt.Sprintf("%s.%d", number, i+1), depth+1)
			}
			if s.Open && len(s.Lines) > 0 {
				row.output = true
				row.key = outputKey{s}
				rows = append(rows, row)
			}
		}
	}
	number := 0
	for i, it := range j.Items {
		if it.Step == nil {
			if filter == filterAll {
				rows = append(rows, rowRef{line: it.Line, key: orphanKey{i}})
			}
			continue
		}
		number++
		add(it.Step, strconv.Itoa(number), 0)
	}
	return rows
}

// statusIcon draws the GitHub-style status glyph: check, cross, skip,
// spinner or dashed circle.
func statusIcon(c *ui.Context, st act.Status, size float32) {
	t := c.Theme()
	s := t.Space(size)
	switch st {
	case act.StatusSuccess:
		components.Icon(c, lucide.CircleCheck()).Size(s, s).TextColor(t.Success)
	case act.StatusFailure:
		components.Icon(c, lucide.CircleX()).Size(s, s).TextColor(t.Danger)
	case act.StatusRunning:
		components.Spinner(c, components.SpinnerSpokes).Size(s, s).TextColor(t.Accent)
	case act.StatusSkipped:
		components.Icon(c, lucide.CircleMinus()).Size(s, s).TextColor(t.TextMuted)
	case act.StatusCancelled:
		components.Icon(c, lucide.CircleSlash()).Size(s, s).TextColor(t.Warning)
	case act.StatusPending:
		fallthrough
	default:
		components.Icon(c, lucide.CircleDashed()).Size(s, s).TextColor(t.TextMuted)
	}
}

// field renders a labeled form field.
func field(c *ui.Context, label string, fn func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(1.5)).Children(func() {
		components.Text(c, label).FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted).SingleLine()
		fn()
	})
}

// styleConfigInput keeps code-like settings consistent across text and tag inputs.
func styleConfigInput(c *ui.Context, input ui.Element) {
	input.FillWidth().Font("JetBrains Mono").FontSize(c.Theme().Rem(12.0 / 13)).Background(inputBackground(c))
}

// abbrevHome shortens a path under the user's home directory for display.
func abbrevHome(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}
