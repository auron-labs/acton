package app

import (
	"acton/internal/act"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/plain"
)

// view builds the window content: the project tabs above the selected
// project, or Home.
func (a *App) view(c *ui.Context) {
	if a.runningCount() > 0 {
		// Tick elapsed/duration labels while a run is live.
		c.After(time.Second)
	}
	ui.Column(c).Fill().Background(panelBackground(c)).Padding(plain.HeaderHeight(c), 0, 0, 0).Children(func() {
		ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
			ui.Divider(c)
			if len(a.sessions) > 0 {
				a.tabBar(c)
				ui.Divider(c)
			}
			if a.current == nil {
				a.homeView(c)
				return
			}
			a.current.view(c)
		})
	})
}

// headerLeading is the shell header's left slot: the project picker and the
// selected project's path and .actrc badge.
func (a *App) headerLeading(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).MinWidth(0).Children(func() {
		folder := components.Button(c, "", components.ButtonSecondary).Label("Choose project folder")
		folder.Children(func() {
			ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).MinWidth(0).Children(func() {
				components.Icon(c, lucide.FolderOpen()).Size(t.Space(4), t.Space(4)).Shrink(0)
				components.Text(c, "Choose project…").SingleLine()
			})
		})
		if folder.Clicked() {
			a.pickFolder()
		}
		s := a.current
		if s == nil {
			return
		}
		components.Text(c, abbrevHome(s.settings.Dir)).TextColor(t.TextMuted).SingleLine().MaxWidth(400).Ellipsis("…")
		if n := len(s.settings.ActrcFiles); n > 0 {
			badge := components.Badge(c, fmt.Sprintf(".actrc ×%d", n), components.BadgeOptions{Tone: components.BadgeNeutral, Icon: lucide.FileText()})
			components.Tooltip(c, badge, "Defaults loaded from:\n"+strings.Join(s.settings.ActrcFiles, "\n"))
		}
	})
}

// headerActions is the shell header's right slot: the selected project's run
// status and Run/Stop button.
func (a *App) headerActions(c *ui.Context) {
	s := a.current
	if s == nil {
		return
	}
	t := c.Theme()
	ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		switch st := s.overallStatus(); {
		case s.running:
			components.Text(c, act.FormatDuration(time.Since(s.started))).TextColor(t.TextMuted).Font("JetBrains Mono").SingleLine()
			stop := components.Button(c, "", components.ButtonSecondary).Label("Stop run")
			stop.Children(func() {
				ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					components.Icon(c, lucide.Square()).Size(t.Space(3.5), t.Space(3.5)).TextColor(t.Danger)
					components.Text(c, "Stop").SingleLine()
				})
			})
			if stop.Clicked() {
				s.stopRun()
			}
		default:
			if st.Done() && len(s.run.Jobs)+len(s.run.Runner) > 0 {
				statusIcon(c, st, 4.5)
				components.Text(c, act.FormatDuration(s.elapsed)).TextColor(t.TextMuted).Font("JetBrains Mono").SingleLine()
			}
			if len(s.workflows) == 0 {
				return
			}
			run := components.Button(c, "", components.ButtonPrimary).Label("Run workflow")
			run.Children(func() {
				ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
					components.Icon(c, lucide.Play()).Size(t.Space(3.5), t.Space(3.5))
					components.Text(c, "Run").SingleLine()
				})
			})
			if run.Clicked() {
				s.startRun()
			}
		}
	})
}

// tabLabels names each tab by its folder, adding the parent folder when two
// open projects share a name.
func (a *App) tabLabels() []string {
	labels := make([]string, len(a.sessions))
	for i, s := range a.sessions {
		labels[i] = s.name()
		for _, other := range a.sessions {
			if other != s && other.name() == labels[i] {
				labels[i] = filepath.Join(filepath.Base(filepath.Dir(s.settings.Dir)), labels[i])
				break
			}
		}
	}
	return labels
}

// tabBar holds the Home button and one tab per open project.
func (a *App) tabBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Shrink(0).Padding(t.Space(1.5), t.Space(2)).Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
		home := ui.Row(c.Key("home-tab")).Shrink(0).Padding(t.Space(1.5), t.Space(2.5)).Gap(t.Space(2)).AlignItems(ui.Center).Radius(t.Radius)
		tabBackground(home, a.current == nil, t)
		home.Label("Home").Cursor(ui.CursorPointer).OnClick(func() { a.current = nil })
		home.Children(func() {
			components.Icon(c, lucide.LayoutGrid()).Size(t.Space(4), t.Space(4)).TextColor(t.TextMuted)
			components.Text(c, "Home").SingleLine()
			if n := a.runningCount(); n > 0 {
				components.Badge(c, strconv.Itoa(n), components.BadgeOptions{Tone: components.BadgeAccent})
			}
		})
		ui.Divider(c).Vertical()
		components.ScrollView(c, components.ScrollViewOptions{Axes: components.ScrollHorizontal}).Grow(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
			// Closing a tab edits a.sessions while the bar is still being built.
			sessions := slices.Clone(a.sessions)
			for i, label := range a.tabLabels() {
				a.tab(c, sessions[i], label)
			}
		})
	})
}

func tabBackground(row ui.Element, selected bool, t *ui.Theme) {
	switch {
	case selected:
		row.Background(t.Accent.Alpha(.18))
	case row.Hovered():
		row.Background(t.Text.Alpha(.04))
	}
}

// tab is one project: select it, or close it once its run has finished.
func (a *App) tab(c *ui.Context, s *Session, label string) {
	t := c.Theme()
	ui.Row(c.Key("tab:" + s.settings.Dir)).Shrink(0).AlignItems(ui.Center).Radius(t.Radius).Children(func() {
		open := ui.Row(c.Key("open")).Padding(t.Space(1.5), t.Space(1), t.Space(1.5), t.Space(2.5)).Gap(t.Space(2)).AlignItems(ui.Center).Radius(t.Radius)
		tabBackground(open, a.current == s, t)
		open.Label("Project " + label).Cursor(ui.CursorPointer).OnClick(func() { a.current = s })
		open.Children(func() {
			s.tabIcon(c)
			components.Text(c, label).SingleLine().MaxWidth(180).Ellipsis("…")
		})
		closeTab := components.Button(c, "", components.ButtonGhost).Label("Close "+label).Disabled(s.running).Size(t.Space(5), t.Space(5)).Padding(0).Shrink(0)
		closeTab.Children(func() { components.Icon(c, lucide.X()).Size(t.Space(3), t.Space(3)).TextColor(t.TextMuted) })
		if closeTab.Clicked() {
			a.closeSession(s)
		}
	})
}

// tabIcon shows a project's run state, or a folder before its first run.
func (s *Session) tabIcon(c *ui.Context) {
	if st := s.overallStatus(); st != act.StatusPending {
		statusIcon(c, st, 4)
		return
	}
	components.Icon(c, lucide.Folder()).Size(c.Theme().Space(4), c.Theme().Space(4)).TextColor(c.Theme().TextMuted)
}

// summary describes the project's latest run for the Home list.
func (s *Session) summary() string {
	st := s.overallStatus()
	if st == act.StatusPending {
		if s.runErr != "" {
			return "Failed to start"
		}
		return "Not run yet"
	}
	finished := 0
	for _, j := range s.run.Jobs {
		if j.Status.Done() {
			finished++
		}
	}
	if len(s.run.Jobs) == 0 {
		return st.String()
	}
	return fmt.Sprintf("%s · %d of %d jobs finished", st, finished, len(s.run.Jobs))
}

// homeView lists every open project with its run state, or the project
// picker when none is open.
func (a *App) homeView(c *ui.Context) {
	t := c.Theme()
	if len(a.sessions) == 0 {
		a.firstScreen(c)
		return
	}
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(760).AlignSelf(ui.Center).Padding(t.Space(6)).Gap(t.Space(3)).Children(func() {
			ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
				components.Text(c, "Projects").FontSize(t.Rem(22.0 / 13)).Bold()
				running := "No runs in progress"
				if n := a.runningCount(); n > 0 {
					running = fmt.Sprintf("%d running", n)
				}
				components.Text(c, running).TextColor(t.TextMuted)
			})
			for _, s := range a.sessions {
				a.projectRow(c, s)
			}
			choose := components.Button(c, "", components.ButtonSecondary).Label("Choose project folder").AlignSelf(ui.Start)
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
		})
	})
}

// projectRow is one open project on Home: click to open its tab.
func (a *App) projectRow(c *ui.Context, s *Session) {
	t := c.Theme()
	ui.Row(c.Key("project:"+s.settings.Dir)).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Radius(t.Radius).Border(1, t.Border).Children(func() {
		open := ui.Row(c.Key("open")).Grow(1).MinWidth(0).Padding(t.Space(3)).Gap(t.Space(3)).AlignItems(ui.Center).Radius(t.Radius)
		if open.Hovered() {
			open.Background(t.Text.Alpha(.04))
		}
		open.Label("Open " + s.name()).Cursor(ui.CursorPointer).OnClick(func() { a.current = s })
		open.Children(func() {
			statusIcon(c, s.overallStatus(), 5)
			ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0.5)).Children(func() {
				components.Text(c, s.name()).Bold().SingleLine()
				components.Text(c, abbrevHome(s.settings.Dir)).Font("JetBrains Mono").FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				components.Text(c, s.summary()).FontSize(t.Rem(12.0 / 13)).TextColor(t.TextMuted).SingleLine()
			})
			switch {
			case s.running:
				components.Text(c, act.FormatDuration(time.Since(s.started))).Font("JetBrains Mono").FontSize(12).TextColor(t.TextMuted).SingleLine().Shrink(0)
			case s.overallStatus().Done():
				components.Text(c, act.FormatDuration(s.elapsed)).Font("JetBrains Mono").FontSize(12).TextColor(t.TextMuted).SingleLine().Shrink(0)
			}
		})
		if !s.running {
			return
		}
		stop := components.Button(c, "", components.ButtonSecondary).Label("Stop "+s.name()).Shrink(0).Margin(0, t.Space(3), 0, 0)
		stop.Children(func() {
			ui.Row(c).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
				components.Icon(c, lucide.Square()).Size(t.Space(3.5), t.Space(3.5)).TextColor(t.Danger)
				components.Text(c, "Stop").SingleLine()
			})
		})
		if stop.Clicked() {
			s.stopRun()
		}
	})
}

// firstScreen is the centered placeholder shown before any project is open.
func (a *App) firstScreen(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(t.Space(3)).Padding(t.Space(6)).Children(func() {
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
	})
}

// recentList shows the recently opened projects that are not open yet;
// clicking one opens it. It renders nothing when there are none.
func (a *App) recentList(c *ui.Context) {
	var dirs []string
	for _, dir := range a.recentProjects() {
		if a.sessionFor(dir) == nil {
			dirs = append(dirs, dir)
		}
	}
	if len(dirs) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c).FillWidth().MaxWidth(420).Gap(t.Space(0.5)).Margin(t.Space(3), 0, 0, 0).Children(func() {
		components.Text(c, "Recent projects").FontSize(t.Rem(12.0/13)).TextColor(t.TextMuted).SingleLine().Padding(0, t.Space(2), t.Space(1), t.Space(2))
		for _, dir := range dirs {
			row := ui.Row(c.Key("recent:"+dir)).FillWidth().Padding(t.Space(1.5), t.Space(2)).Gap(t.Space(2)).AlignItems(ui.Center).Radius(t.Radius)
			if row.Hovered() {
				row.Background(t.Text.Alpha(.04))
			}
			row.Label("Open " + dir).Cursor(ui.CursorPointer).OnClick(func() { a.openProject(dir) })
			row.Children(func() {
				components.Icon(c, lucide.Folder()).Size(t.Space(4), t.Space(4)).TextColor(t.TextMuted).Shrink(0)
				ui.Row(c).Grow(1).MinWidth(0).Clip().Children(func() {
					components.Text(c, abbrevHome(dir)).SingleLine()
				})
			})
		}
	})
}
