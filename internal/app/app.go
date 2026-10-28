// Package app is the Acton application: the window's state, the views that
// render it, and the act runs it launches.
package app

import (
	"acton/internal/act"
	"acton/internal/settings"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/kballard/go-shellquote"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/shells/plain"
	"gopkg.in/yaml.v3"
)

// App is the application's state. Views are pure functions of it; act output
// arrives through runner → pump on the main thread.
type App struct {
	win *mygo.Window

	settings     settings.Settings
	eventSel     string // event dropdown value; "custom" reveals a text field
	eventOther   string
	workflows    []string
	workflowJobs []string
	workflowMeta string
	workflowErr  string
	configOpen   [4]bool
	envTab       int
	showDefaults bool
	fileErr      string

	run     act.Run
	runner  *act.Runner
	running bool
	runErr  string // start or exit error
	elapsed time.Duration
	started time.Time

	flushScheduled atomic.Bool
	done           chan struct{}

	selJob  string // "" auto-selects the first job
	logMode int    // 0 steps, 1 raw log

	cfgW    float32 // config pane width
	jobW    float32 // jobs pane width
	logUI   logUIState
	rawLogs map[*act.Job]*rawLog
}

// logUIState holds view-scoped widget state for the log pane.
type logUIState struct {
	list   components.ListState
	runner components.ListState
}

// rawLog is the view-side raw log buffer of one job, appended lazily so the
// read-only view keeps its scroll position.
type rawLog struct {
	buf *ui.TextBuffer
	n   int
}

// rawLogFor returns the job's raw log buffer, creating it on first use.
func (a *App) rawLogFor(j *act.Job) *rawLog {
	if a.rawLogs == nil {
		a.rawLogs = map[*act.Job]*rawLog{}
	}
	rl := a.rawLogs[j]
	if rl == nil {
		rl = &rawLog{buf: ui.NewTextBuffer("")}
		a.rawLogs[j] = rl
	}
	return rl
}

// Run starts the desktop app: it builds the window's shell once mygo is
// ready, wires the lifecycle, and blocks in the event loop until done. dir,
// when set, preselects the project; runNow starts act immediately.
func Run(dir string, runNow bool) error {
	if err := registerFonts(); err != nil {
		return err
	}
	a := newApp()
	defer a.shutdown()
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	go func() {
		select {
		case <-ctx.Done():
			mygo.App.Quit()
		case <-a.done:
		}
	}()
	if dir != "" {
		a.setDir(dir)
	}
	mygo.App.WhenReady(func() {
		shell := plain.NewShell(plain.ShellOptions{
			Title:               "Acton",
			Width:               1120,
			Height:              720,
			MinWidth:            720,
			MinHeight:           480,
			StateKey:            "main",
			Theme:               designTheme,
			HeaderLeading:       a.headerLeading,
			HeaderActions:       a.headerActions,
			Content:             a.view,
			ContentBehindHeader: true,
		})
		a.win = shell.Window()
		a.win.OnClosed(func() {
			a.stopRun()
			mygo.App.Quit()
		})
		if runNow {
			a.startRun()
		}
	})
	if err := mygo.App.Run(); err != nil {
		return fmt.Errorf("mygo run: %w", err)
	}
	return nil
}

func newApp() *App {
	a := &App{
		settings: *settings.Default(""),
		eventSel: "push",
		cfgW:     320,
		jobW:     240,
		done:     make(chan struct{}),
	}
	a.logUI.list.FollowEnd = true
	a.logUI.runner.FollowEnd = true
	a.logUI.runner.Key = func(i int) any { return i }
	return a
}

// pickFolder opens the native directory picker off the UI thread, then
// applies the choice (including .actrc defaults) on it.
func (a *App) pickFolder() {
	if a.win == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    a.win,
			Title:     "Choose a project folder",
			Directory: true,
		})
		a.win.Update(func() {
			if err != nil || len(paths) == 0 {
				return
			}
			a.setDir(paths[0])
		})
	}()
}

// setDir points the app at a project: reload .actrc defaults and workflows.
func (a *App) setDir(dir string) {
	keep := a.settings.ExtraArgs
	a.settings = *settings.Default(dir)
	a.settings.ExtraArgs = keep
	a.settings.LoadActrc()
	a.workflows = settings.ScanWorkflows(dir)
	a.clearRun()
	if a.settings.Workflow != "" && !contains(a.workflows, a.settings.Workflow) {
		a.workflows = append(a.workflows, a.settings.Workflow)
	}
	a.eventSel = a.settings.Event
	a.eventOther = a.settings.Event
	if !contains(workflowEvents, a.eventSel) {
		a.eventSel = "custom"
	}
	a.fileErr = ""
	a.loadWorkflow()
}

// loadWorkflow refreshes the selector metadata only when the target changes.
func (a *App) loadWorkflow() {
	a.workflowJobs, a.workflowMeta, a.workflowErr = nil, "", ""
	if a.settings.Workflow == "" {
		a.workflowMeta = fmt.Sprintf("%d workflows", len(a.workflows))
		return
	}
	path := a.settings.Workflow
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.settings.Dir, path)
	}
	//nolint:gosec // G304: the native project picker and workflow menu select this local file.
	data, err := os.ReadFile(path)
	var workflow struct {
		Jobs map[string]yaml.Node `yaml:"jobs"`
		On   yaml.Node            `yaml:"on"`
	}
	if err == nil {
		err = yaml.Unmarshal(data, &workflow)
	}
	if err != nil {
		a.workflowErr = "Cannot read workflow: " + err.Error()
		return
	}
	for id := range workflow.Jobs {
		a.workflowJobs = append(a.workflowJobs, id)
	}
	slices.Sort(a.workflowJobs)
	events := []string{}
	switch workflow.On.Kind {
	case yaml.ScalarNode:
		events = append(events, workflow.On.Value)
	case yaml.SequenceNode:
		for _, event := range workflow.On.Content {
			events = append(events, event.Value)
		}
	case yaml.MappingNode:
		for i := 0; i < len(workflow.On.Content); i += 2 {
			events = append(events, workflow.On.Content[i].Value)
		}
	case yaml.DocumentNode, yaml.AliasNode:
	}
	a.workflowMeta = fmt.Sprintf("%d jobs", len(a.workflowJobs))
	if len(events) > 0 {
		a.workflowMeta += " · on " + strings.Join(events, ", ")
	}
}

// pickConfigFile binds the chosen path to act's existing file flags.
func (a *App) pickConfigFile(kind string) {
	if a.win == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "Choose " + kind + " file"})
		a.win.Update(func() {
			if err != nil {
				a.fileErr = "Cannot choose file: " + err.Error()
				return
			}
			if len(paths) == 0 {
				return
			}
			a.fileErr = ""
			switch kind {
			case "Secrets":
				a.settings.SecretFile = paths[0]
			case "Env":
				a.settings.EnvFile = paths[0]
			case "Vars":
				a.settings.VarFile = paths[0]
			case "Event payload":
				a.settings.EventPayload = paths[0]
			}
		})
	}()
}

// clearRun drops run state for a fresh directory or a manual clear.
func (a *App) clearRun() {
	a.run.Reset()
	a.runErr = ""
	a.selJob = ""
	a.logMode = 0
	a.elapsed = 0
	a.rawLogs = nil
}

// startRun launches act with the current settings.
func (a *App) startRun() {
	if a.running || a.settings.Dir == "" || a.win == nil {
		return
	}
	args, err := a.settings.Args()
	if err != nil {
		a.runErr = err.Error()
		return
	}
	a.clearRun()
	r := &act.Runner{}
	err = r.Start(a.settings.Dir, args, a.scheduleFlush, a.scheduleFlush)
	if err != nil {
		a.runErr = fmt.Sprintf("failed to start act: %v", err)
		return
	}
	a.runner = r
	a.running = true
	a.started = time.Now()
}

func (a *App) stopRun() {
	if a.running && a.runner != nil {
		a.runner.Stop()
	}
}

// shutdown waits for act's post steps and resource cleanup before returning to main.
func (a *App) shutdown() {
	close(a.done)
	if a.runner != nil {
		a.runner.Stop()
		a.runner.Wait()
	}
}

// scheduleFlush asks the UI thread to drain runner output; coalesced so
// floods of lines don't flood the main queue. Called from worker goroutines.
func (a *App) scheduleFlush() {
	if a.win == nil {
		return
	}
	if a.flushScheduled.CompareAndSwap(false, true) {
		a.win.Update(func() {
			a.flushScheduled.Store(false)
			a.pump()
		})
	}
}

// pump drains buffered act output into the run model, on the main thread.
func (a *App) pump() {
	if a.runner == nil {
		return
	}
	lines, done, err := a.runner.Drain()
	for _, l := range lines {
		a.run.Feed(l)
	}
	if done {
		// Any lines the scanner delivered after the last drain.
		lines, _, _ = a.runner.Drain()
		for _, l := range lines {
			a.run.Feed(l)
		}
		a.running = false
		a.elapsed = time.Since(a.started)
		st := act.StatusFailure
		switch {
		case err == nil:
			st = act.StatusSuccess
		case friendlyExit(err) == "stopped":
			st = act.StatusCancelled
		default:
			// An exit error with a failed job already explains itself in the
			// job list; only surface it when nothing else does.
			failed := false
			for _, j := range a.run.Jobs {
				failed = failed || j.Status == act.StatusFailure
			}
			if !failed {
				a.runErr = friendlyExit(err)
			}
		}
		a.run.Finish(st)
	}
}

// selectedJob resolves the job the log pane shows: the user's choice, or the
// newest job once output arrives.
func (a *App) selectedJob() *act.Job {
	if a.selJob != "" && a.selJob != act.RunnerJobKey {
		for _, j := range a.run.Jobs {
			if j.Key == a.selJob {
				return j
			}
		}
	}
	if len(a.run.Jobs) > 0 {
		return a.run.Jobs[0]
	}
	return nil
}

// overallStatus summarizes the run for the header.
func (a *App) overallStatus() act.Status {
	if a.running {
		return act.StatusRunning
	}
	if len(a.run.Jobs) == 0 && len(a.run.Runner) == 0 {
		return act.StatusPending
	}
	st := act.StatusSuccess
	for _, j := range a.run.Jobs {
		switch j.Status {
		case act.StatusFailure:
			return act.StatusFailure
		case act.StatusRunning, act.StatusPending:
			st = act.StatusRunning
		case act.StatusCancelled:
			st = act.StatusCancelled
		case act.StatusSuccess, act.StatusSkipped:
		}
	}
	if a.runErr != "" {
		return act.StatusFailure
	}
	return st
}

func friendlyExit(err error) string {
	if errors.Is(err, context.Canceled) {
		return "stopped"
	}
	return "act failed: " + err.Error()
}

// cmdline renders a shell-quoted command with secret values redacted.
func (a *App) cmdline() string {
	args, err := a.settings.Args()
	if err != nil {
		return "act"
	}
	for i, w := range args {
		// Keep secret values out of both command previews and clipboard copies.
		switch {
		case i > 0 && (args[i-1] == "-s" || args[i-1] == "--secret"):
			name, _, _ := strings.Cut(w, "=")
			args[i] = name + "=••••••••"
		case strings.HasPrefix(w, "--secret="):
			name, _, _ := strings.Cut(strings.TrimPrefix(w, "--secret="), "=")
			args[i] = "--secret=" + name + "=••••••••"
		case strings.HasPrefix(w, "-s") && w != "-s":
			name, _, _ := strings.Cut(w[2:], "=")
			args[i] = "-s" + name + "=••••••••"
		}
	}
	return "act " + shellquote.Join(args...)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
