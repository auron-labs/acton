package app

import (
	"actgui/internal/act"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// view builds the window content: the resizable configuration | run split.
// The shell header carries the project picker and run controls.
func (a *App) view(c *ui.Context) {
	if a.running {
		// Tick elapsed/duration labels while a run is live.
		c.After(time.Second)
	}
	ui.Column(c).Fill().Children(func() {
		ui.Divider(c)
		components.Split(c, &a.cfgW, func() { a.configView(c) }, func() { a.runView(c) },
			components.SplitOptions{FirstMin: 230, SecondMin: 320, Default: 300}).Fill()
	})
}

// headerLeading is the shell header's left slot: the project picker and the
// .actrc badge.
func (a *App) headerLeading(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).MinWidth(0).Children(func() {
		folder := components.Button(c, "", components.ButtonGhost).Label("Choose project folder")
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
			components.Text(c, time.Since(a.started).Round(time.Second).String()).TextColor(t.TextMuted).Font("monospace").SingleLine()
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
				components.Text(c, a.elapsed.Round(time.Second).String()).TextColor(t.TextMuted).Font("monospace").SingleLine()
			}
			run := components.Button(c, "", components.ButtonPrimary).Label("Run workflow").Disabled(a.settings.Dir == "")
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

// configView is the left pane: the run configuration form in a scroll view.
func (a *App) configView(c *ui.Context) {
	t := c.Theme()
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Padding(t.Space(3)).Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space(4)).Children(func() {
			sectionTitle(c, "Target")
			field(c, "Workflow", func() {
				opts := make([]components.NativeSelectOption[string], 0, len(a.workflows)+1)
				opts = append(opts, components.NativeSelectOption[string]{Value: "", Label: "All workflows"})
				for _, w := range a.workflows {
					opts = append(opts, components.NativeSelectOption[string]{Value: w, Label: filepath.Base(w)})
				}
				components.NativeSelect(c, &a.settings.Workflow, opts).Trigger.FillWidth().Label("Workflow")
			})
			field(c, "Event", func() {
				ui.Row(c).Gap(t.Space(2)).Children(func() {
					events := []string{"push", "pull_request", "workflow_dispatch", "schedule", "release", "issues", "create", "delete", "fork"}
					opts := make([]components.NativeSelectOption[string], 0, len(events)+1)
					for _, e := range events {
						opts = append(opts, components.NativeSelectOption[string]{Value: e, Label: e})
					}
					opts = append(opts, components.NativeSelectOption[string]{Value: "custom", Label: "Custom…"})
					sel := components.NativeSelect(c, &a.eventSel, opts)
					sel.Trigger.Shrink(0).Label("Event")
					if sel.Changed() {
						if a.eventSel == "custom" {
							a.eventOther = a.settings.Event
						} else {
							a.settings.Event = a.eventSel
						}
					}
					if a.eventSel == "custom" {
						tb := components.TextBox(c, &a.eventOther, components.TextBoxOptions{}).
							Placeholder("event name").Label("Custom event name").Grow(1)
						if tb.Changed() {
							a.settings.Event = a.eventOther
						}
					}
				})
			})
			field(c, "Job", func() {
				components.TextBox(c, &a.settings.Job, components.TextBoxOptions{}).
					Placeholder("All jobs").Label("Job ID filter").FillWidth()
			})

			sectionTitle(c, "Images")
			field(c, "Platforms", func() {
				components.TagInput(c, &a.settings.Platforms).Placeholder("runner=image").Label("Platform to image map")
			})
			field(c, "Container architecture", func() {
				components.TextBox(c, &a.settings.Arch, components.TextBoxOptions{}).
					Placeholder("linux/amd64").Label("Container architecture").FillWidth()
			})

			sectionTitle(c, "Variables")
			field(c, "Secrets", func() {
				components.TagInput(c, &a.settings.Secrets).Placeholder("NAME=VALUE").Label("Secrets")
			})
			field(c, "Environment", func() {
				components.TagInput(c, &a.settings.Env).Placeholder("NAME=VALUE").Label("Environment variables")
			})
			field(c, "Vars", func() {
				components.TagInput(c, &a.settings.Vars).Placeholder("NAME=VALUE").Label("Variables")
			})
			field(c, "Action inputs", func() {
				components.TagInput(c, &a.settings.Inputs).Placeholder("name=value").Label("workflow_dispatch inputs")
			})

			sectionTitle(c, "Options")
			ui.Column(c).Gap(t.Space(2.5)).Children(func() {
				components.Switch(c, &a.settings.DryRun, "Dry run (plan only)")
				components.Switch(c, &a.settings.Verbose, "Verbose output")
				components.Switch(c, &a.settings.Pull, "Always pull images")
				components.Switch(c, &a.settings.Reuse, "Reuse containers")
				components.Switch(c, &a.settings.Bind, "Bind workdir (no copy)")
				components.Switch(c, &a.settings.Quiet, "Quiet (no progress)")
			})
			field(c, "Event payload file", func() {
				components.TextBox(c, &a.settings.EventPayload, components.TextBoxOptions{}).
					Placeholder("path/to/event.json").Label("Event payload JSON file").FillWidth()
			})
			field(c, "Extra act args", func() {
				components.TextBox(c, &a.settings.ExtraArgs, components.TextBoxOptions{}).
					Placeholder("--matrix os:linux …").Label("Additional act flags").FillWidth()
			})

			if len(a.settings.ActrcFiles) > 0 {
				ui.Column(c).Gap(t.Space(1)).Children(func() {
					components.Text(c, "Defaults from .actrc").FontSize(11).Bold().TextColor(t.TextMuted)
					for _, f := range a.settings.ActrcFiles {
						components.Text(c, abbrevHome(f)).FontSize(11).Font("monospace").TextColor(t.TextMuted).MaxLines(2).Ellipsis("…")
					}
				})
			}
		})
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
		components.Icon(c, lucide.SquareTerminal()).Size(t.Space(12), t.Space(12)).TextColor(t.TextMuted)
		if a.settings.Dir == "" {
			components.Text(c, "Pick a project folder to run its workflows with act").TextColor(t.TextMuted)
			return
		}
		if len(a.workflows) == 0 {
			components.Text(c, "No .github/workflows found in this folder — act may fail").TextColor(t.Warning).FontSize(12)
		}
		if a.running {
			components.Spinner(c, components.SpinnerSpokes)
			components.Text(c, "Starting act…").TextColor(t.TextMuted)
		} else {
			components.Text(c, "Nothing has run yet").TextColor(t.TextMuted)
			ui.Row(c).MaxWidth(560).Padding(t.Space(1.5), t.Space(3)).Radius(t.Radius).
				Background(t.Surface.Alpha(.6)).Border(1, t.Border).Children(func() {
				components.Text(c, a.cmdline()).Font("monospace").FontSize(12).TextColor(t.TextMuted).Selectable()
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
		row.Background(t.Text.Alpha(.09))
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
		row.Background(t.Text.Alpha(.09))
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
					components.Text(c, d).FontSize(11).TextColor(t.TextMuted).Font("monospace").SingleLine().Shrink(0)
				}
			})
			ui.Segmented(c, &a.logMode, "Steps", "Raw log").Shrink(0).Label("Log view mode")
		})
		ui.Divider(c)
		if a.logMode == 1 {
			a.rawLogView(c, j)
		} else {
			a.stepsView(c, j)
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

// stepsView renders the job's flat row list (step headers + open lines) in a
// virtualized, end-following list, like GitHub's job log.
func (a *App) stepsView(c *ui.Context, j *act.Job) {
	rows := flatRows(j)
	a.logUI.list.Key = func(i int) any { return rows[i].key }
	components.List(c, &a.logUI.list, len(rows), func(i int) {
		r := rows[i]
		if r.step != nil && r.line == "" {
			a.stepHeaderRow(c, r.step)
			return
		}
		a.logLineRow(c, r)
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
		Fill().Font("monospace").FontSize(12).Label("Raw act output")
}

// linesView renders a plain list of lines (runner output).
func (a *App) linesView(c *ui.Context, lines []string) {
	t := c.Theme()
	components.List(c, &a.logUI.runner, len(lines), func(i int) {
		ui.Row(c).FillWidth().Padding(0, t.Space(3), 0, 0).Children(func() {
			ui.Text(c, lines[i]).Font("monospace").FontSize(12).Selectable()
		})
	}).Fill()
}

// stepHeaderRow is one collapsible step header in the log list.
func (a *App) stepHeaderRow(c *ui.Context, s *act.Step) {
	t := c.Theme()
	row := ui.Row(c).FillWidth().Padding(t.Space(1.5), t.Space(2.5)).Gap(t.Space(2)).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(t.Text.Alpha(.04))
	}
	row.Label("Step " + s.Name).Cursor(ui.CursorPointer).OnClick(func() { s.Open = !s.Open })
	row.Children(func() {
		statusIcon(c, s.Status, 4)
		ui.Row(c).Grow(1).MinWidth(0).Gap(t.Space(2)).AlignItems(ui.Baseline).Children(func() {
			components.Text(c, s.Name).SingleLine()
			if d := s.Duration(); d != "" {
				components.Text(c, d).FontSize(11).TextColor(t.TextMuted).Font("monospace").SingleLine().Shrink(0)
			}
		})
		chev := lucide.ChevronRight()
		if s.Open {
			chev = lucide.ChevronDown()
		}
		components.Icon(c, chev).Size(t.Space(3.5), t.Space(3.5)).TextColor(t.TextMuted).Shrink(0)
	})
}

// logLineRow is one output line beneath a step header (or job level).
func (a *App) logLineRow(c *ui.Context, r rowRef) {
	t := c.Theme()
	indent := t.Space(8)
	if r.step == nil {
		indent = t.Space(2.5)
	}
	row := ui.Row(c).FillWidth().Padding(0, t.Space(2.5), 0, indent)
	color := t.Text
	switch {
	case strings.HasPrefix(r.line, "❌"), strings.Contains(r.line, "::error"), strings.HasPrefix(r.line, "Error:"):
		color = t.Danger
	case strings.HasPrefix(r.line, "⚠"), strings.Contains(r.line, "::warning"):
		color = t.Warning
	case r.step == nil:
		color = t.TextMuted
	}
	row.Children(func() {
		ui.Text(c, r.line).Font("monospace").FontSize(12).TextColor(color).Selectable()
	})
}

// rowRef is one row of the flat steps list: a step header (step set, no
// line) or an output line (line set; step nil for job-level lines).
type rowRef struct {
	step *act.Step
	line string
	key  any
}

type lineKey struct {
	step *act.Step
	idx  int
}
type orphanKey struct{ idx int }

// flatRows expands a job's items into the visible row list: each step's
// header followed by its lines while open.
func flatRows(j *act.Job) []rowRef {
	rows := make([]rowRef, 0, len(j.Items))
	for i, it := range j.Items {
		if it.Step == nil {
			rows = append(rows, rowRef{line: it.Line, key: orphanKey{i}})
			continue
		}
		rows = append(rows, rowRef{step: it.Step, key: it.Step})
		if it.Step.Open {
			for li, l := range it.Step.Lines {
				rows = append(rows, rowRef{step: it.Step, line: l, key: lineKey{it.Step, li}})
			}
		}
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

// sectionTitle separates groups in the config form.
func sectionTitle(c *ui.Context, text string) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		components.Text(c, text).FontSize(11).Bold().TextColor(t.TextMuted).SingleLine()
		ui.Box(c).Grow(1).Height(1).Background(t.Border)
	})
}

// field renders a labeled form field.
func field(c *ui.Context, label string, fn func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
		components.Text(c, label).FontSize(11).TextColor(t.TextMuted).SingleLine()
		fn()
	})
}

// abbrevHome shortens a path under the user's home directory for display.
func abbrevHome(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}
