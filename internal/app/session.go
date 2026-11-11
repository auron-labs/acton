package app

import (
	"acton/internal/act"
	"acton/internal/settings"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/kballard/go-shellquote"
	"github.com/oneo-me/onevra/source/components"
	"gopkg.in/yaml.v3"
)

// Session is one project tab: its settings, workflow list, and act run. Views
// are pure functions of it; act output arrives through runner → pump on the
// main thread.
type Session struct {
	app *App

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
	savedArgs    string // resolved settings as last loaded from or saved to .actrc

	mappingOpen   bool   // runner-label prompt shown before a run
	jobOnly       string // job id to run alone, instead of settings.Job, until the run starts or is cancelled
	mappingPrompt []mappingSuggestion

	run     act.Run
	runner  *act.Runner
	running bool
	runErr  string // start or exit error
	elapsed time.Duration
	started time.Time

	flushScheduled atomic.Bool

	selJob  string // "" auto-selects the first job
	logMode int    // 0 steps, 1 raw log

	cfgW    float32 // config pane width
	jobW    float32 // jobs pane width
	logUI   logUIState
	rawLogs map[*act.Job]*rawLog

	runID        int
	runDir       string
	runWorkflows []*workflowSource // workflow files of the current run
	workflowTab  int
	runFiles     *runFiles
}

// logUIState holds view-scoped widget state for the log pane.
type logUIState struct {
	list   components.ListState
	runner components.ListState
	filter stepFilter
	output map[*act.Step]*components.ListState
	tabs   map[*act.Step]*int
}

// rawLog keeps a job's raw-output scroll position across frames.
type rawLog struct {
	list components.ListState
}

// rawLogFor returns the job's persistent raw-output view state.
func (s *Session) rawLogFor(j *act.Job) *rawLog {
	if s.rawLogs == nil {
		s.rawLogs = map[*act.Job]*rawLog{}
	}
	rl := s.rawLogs[j]
	if rl == nil {
		rl = &rawLog{}
		s.rawLogs[j] = rl
	}
	return rl
}

func newSession(app *App) *Session {
	s := &Session{
		app:      app,
		settings: *settings.Default(""),
		eventSel: "push",
		cfgW:     320,
		jobW:     240,
	}
	s.logUI.list.FollowEnd = true
	s.logUI.runner.FollowEnd = true
	s.logUI.runner.Key = func(i int) any { return i }
	s.markSettingsSaved()
	return s
}

// name is the project's folder name, as shown on its tab.
func (s *Session) name() string {
	if s.settings.Dir == "" {
		return "Project"
	}
	return filepath.Base(s.settings.Dir)
}

// setDir points the session at a project: reload .actrc defaults and workflows.
func (s *Session) setDir(dir string) {
	keep := s.settings.ExtraArgs
	s.settings = *settings.Default(dir)
	s.settings.ExtraArgs = keep
	s.settings.LoadActrc()
	s.markSettingsSaved()
	s.clearRun()
	s.eventSel = s.settings.Event
	s.eventOther = s.settings.Event
	if !contains(workflowEvents, s.eventSel) {
		s.eventSel = "custom"
	}
	s.fileErr = ""
	s.rescanWorkflows()
	s.app.rememberProject(dir)
}

// workflowFile summarizes a workflow file's jobs and trigger events.
type workflowFile struct {
	jobs   []string
	events []string
}

// readWorkflow parses the jobs and events of a workflow path relative to dir.
func readWorkflow(dir, path string) (workflowFile, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	//nolint:gosec // G304: the native project picker and workflow menu select this local file.
	data, err := os.ReadFile(path)
	var workflow struct {
		Jobs map[string]yaml.Node `yaml:"jobs"`
		On   yaml.Node            `yaml:"on"`
	}
	if err != nil {
		//nolint:wrapcheck // The error names the file; callers add the user-facing context.
		return workflowFile{}, err
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return workflowFile{}, fmt.Errorf("invalid YAML: %w", err)
	}
	var file workflowFile
	for id := range workflow.Jobs {
		file.jobs = append(file.jobs, id)
	}
	slices.Sort(file.jobs)
	switch workflow.On.Kind {
	case yaml.ScalarNode:
		file.events = append(file.events, workflow.On.Value)
	case yaml.SequenceNode:
		for _, event := range workflow.On.Content {
			file.events = append(file.events, event.Value)
		}
	case yaml.MappingNode:
		for i := 0; i < len(workflow.On.Content); i += 2 {
			file.events = append(file.events, workflow.On.Content[i].Value)
		}
	case yaml.DocumentNode, yaml.AliasNode:
	}
	return file, nil
}

// loadWorkflow refreshes the selector metadata only when the target changes.
func (s *Session) loadWorkflow() {
	s.workflowJobs, s.workflowMeta, s.workflowErr = nil, "", ""
	selected := s.settings.Workflows
	if len(selected) == 0 {
		s.workflowMeta = fmt.Sprintf("%d workflows", len(s.workflows))
		return
	}
	var events []string
	for _, path := range selected {
		file, err := readWorkflow(s.settings.Dir, path)
		if err != nil {
			s.workflowJobs, s.workflowMeta = nil, ""
			s.workflowErr = "Cannot read workflow: " + err.Error()
			return
		}
		for _, job := range file.jobs {
			if !slices.Contains(s.workflowJobs, job) {
				s.workflowJobs = append(s.workflowJobs, job)
			}
		}
		events = file.events
	}
	slices.Sort(s.workflowJobs)
	if len(selected) > 1 {
		s.workflowMeta = fmt.Sprintf("%d workflows · %d jobs", len(selected), len(s.workflowJobs))
		return
	}
	s.workflowMeta = fmt.Sprintf("%d jobs", len(s.workflowJobs))
	if len(events) > 0 {
		s.workflowMeta += " · on " + strings.Join(events, ", ")
	}
}

// rescanWorkflows looks for workflow files in the current project again,
// keeping the configured settings; explicit workflow selections stay
// selectable just like in setDir.
func (s *Session) rescanWorkflows() {
	s.workflows = settings.ScanWorkflows(s.settings.Dir)
	for _, w := range s.settings.Workflows {
		if !contains(s.workflows, w) {
			s.workflows = append(s.workflows, w)
		}
	}
	s.loadWorkflow()
}

// pickConfigFile binds the chosen path to act's existing file flags.
func (s *Session) pickConfigFile(kind string) {
	win := s.app.win
	if win == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "Choose " + kind + " file"})
		win.Update(func() {
			if err != nil {
				s.fileErr = "Cannot choose file: " + err.Error()
				return
			}
			if len(paths) == 0 {
				return
			}
			s.fileErr = ""
			switch kind {
			case "Secrets":
				s.settings.SecretFile = paths[0]
			case "Env":
				s.settings.EnvFile = paths[0]
			case "Vars":
				s.settings.VarFile = paths[0]
			case "Event payload":
				s.settings.EventPayload = paths[0]
			}
		})
	}()
}

// clearRun drops run state for a fresh directory or a manual clear.
func (s *Session) clearRun() {
	if s.runID != 0 {
		s.app.history = append(s.app.history, s.snapshot())
		s.runID = 0
	}
	s.run.Reset()
	s.runErr = ""
	s.selJob = ""
	s.logMode = 0
	s.elapsed = 0
	s.rawLogs = nil
	s.runWorkflows, s.workflowTab = nil, 0
	s.logUI.filter = filterAll
	s.logUI.output = nil
	s.logUI.tabs = nil
}

// settingsKey is the resolved act command line, used to detect unsaved edits.
func settingsKey(st *settings.Settings) (string, error) {
	args, err := st.Args()
	if err != nil {
		return "", err //nolint:wrapcheck // Callers only test whether the settings resolve.
	}
	return strings.Join(args, "\x00"), nil
}

func (s *Session) markSettingsSaved() { s.savedArgs, _ = settingsKey(&s.settings) }

// settingsUnsaved reports valid settings that differ from the saved .actrc;
// invalid ones already show their own error and cannot be saved.
func (s *Session) settingsUnsaved() bool {
	key, err := settingsKey(&s.settings)
	return s.settings.Dir != "" && err == nil && key != s.savedArgs
}

// mappingSuggestion is an unmapped runner label with an editable image.
type mappingSuggestion struct{ Label, Image string }

// unmappedLabels lists the runner labels of the jobs about to run that have no image.
func (s *Session) unmappedLabels() []string {
	st := s.runSettings()
	return act.UnmappedLabels(st.Dir, st.Workflows, st.Event, st.Job, st.Platforms)
}

// runSettings is the settings the next run uses: the configured ones, narrowed
// to a single job when one was requested from the job list.
func (s *Session) runSettings() settings.Settings {
	st := s.settings
	if s.jobOnly != "" {
		st.Job = s.jobOnly
	}
	return st
}

// unmappedLabelsError is the non-interactive report: labels plus the -P values to add.
func unmappedLabelsError(labels []string) error {
	hints := make([]string, len(labels))
	for i, l := range labels {
		hints[i] = "-P " + l + "=" + settings.SuggestImage(l)
	}
	return fmt.Errorf("no image mapped for runner label(s) %s; add platform mappings such as %s and retry",
		strings.Join(labels, ", "), strings.Join(hints, " "))
}

// requestRun is the UI entry point: runner labels without an image open the
// mapping prompt instead of letting act silently skip those jobs.
func (s *Session) requestRun() { s.requestJobRun("") }

// requestJobRun is requestRun for a single job; "" runs the configured selection.
func (s *Session) requestJobRun(jobID string) {
	if !s.runnable() {
		return
	}
	s.jobOnly = jobID
	labels := s.unmappedLabels()
	if len(labels) == 0 {
		s.startRun()
		return
	}
	s.mappingPrompt = s.mappingPrompt[:0]
	for _, l := range labels {
		s.mappingPrompt = append(s.mappingPrompt, mappingSuggestion{l, settings.SuggestImage(l)})
	}
	s.mappingOpen = true
}

// startRunChecked is the non-interactive entry point: it never prompts and
// reports unmapped runner labels as an error.
func (s *Session) startRunChecked() error {
	s.jobOnly = ""
	if labels := s.unmappedLabels(); len(labels) > 0 {
		return unmappedLabelsError(labels)
	}
	s.startRun()
	return nil
}

// acceptMappings adds the prompted mappings to the session and starts the run.
func (s *Session) acceptMappings() {
	s.mappingOpen = false
	for _, m := range s.mappingPrompt {
		if image := strings.TrimSpace(m.Image); image != "" {
			s.settings.Platforms = append(s.settings.Platforms, m.Label+"="+image)
		}
	}
	s.startRun()
}

func (s *Session) runnable() bool {
	return !s.running && s.settings.Dir != "" && len(s.workflows) > 0
}

// startRun launches act with the current settings.
func (s *Session) startRun() {
	if !s.runnable() {
		return
	}
	st := s.runSettings()
	s.jobOnly = ""
	s.clearRun()
	s.started = time.Now()
	var err error
	if s.app.preferences.SaveRuns {
		s.runFiles, err = newRunFiles(s.settings.Dir, s.started)
		if err != nil {
			s.app.preferenceErr = "Cannot save run output: " + err.Error()
		}
	}
	args, err := st.Args()
	if err != nil {
		s.runErr = err.Error()
		s.writeRunFile(`{"msg":"act failed to start"}`)
		s.closeRunFiles()
		s.app.notifyRun(s, act.StatusFailure)
		return
	}
	r := &act.Runner{}
	err = r.Start(s.settings.Dir, args, s.scheduleFlush, s.scheduleFlush)
	if err != nil {
		s.runErr = fmt.Sprintf("failed to start act: %v", err)
		s.writeRunFile(`{"msg":"act failed to start"}`)
		s.closeRunFiles()
		s.app.notifyRun(s, act.StatusFailure)
		return
	}
	s.runner = r
	s.running = true
	s.app.nextRunID++
	s.runID, s.runDir = s.app.nextRunID, s.settings.Dir
	s.runWorkflows = s.workflowSources()
}

func (s *Session) stopRun() {
	if s.running && s.runner != nil {
		s.runner.Stop()
	}
}

// scheduleFlush asks the UI thread to drain runner output; coalesced so
// floods of lines don't flood the main queue. Called from worker goroutines.
func (s *Session) scheduleFlush() {
	win := s.app.win
	if win == nil {
		return
	}
	if s.flushScheduled.CompareAndSwap(false, true) {
		win.Update(func() {
			s.flushScheduled.Store(false)
			s.pump()
		})
	}
}

// pump drains buffered act output into the run model, on the main thread.
func (s *Session) pump() {
	if s.runner == nil || !s.running {
		return
	}
	lines, done, err := s.runner.Drain()
	for _, l := range lines {
		s.feedRun(l)
	}
	if done {
		// Any lines the scanner delivered after the last drain.
		lines, _, _ = s.runner.Drain()
		for _, l := range lines {
			s.feedRun(l)
		}
		s.running = false
		s.elapsed = time.Since(s.started)
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
			for _, j := range s.run.Jobs {
				failed = failed || j.Status == act.StatusFailure
			}
			if !failed {
				s.runErr = friendlyExit(err)
			}
		}
		s.run.Finish(st)
		for _, job := range s.run.Jobs {
			if job.Status == act.StatusFailure {
				st = act.StatusFailure
				break
			}
		}
		if err != nil {
			s.writeRunFile(fmt.Sprintf(`{"msg":"act %s"}`, st.String()))
		}
		s.closeRunFiles()
		s.app.notifyRun(s, st)
	}
}

// selectedJob resolves the job the log pane shows: the user's choice, or the
// newest job once output arrives.
func (s *Session) selectedJob() *act.Job {
	if s.selJob != "" && s.selJob != act.RunnerJobKey {
		for _, j := range s.run.Jobs {
			if j.Key == s.selJob {
				return j
			}
		}
	}
	if len(s.run.Jobs) > 0 {
		return s.run.Jobs[0]
	}
	return nil
}

// overallStatus summarizes the run for the header.
func (s *Session) overallStatus() act.Status {
	if s.running {
		return act.StatusRunning
	}
	if len(s.run.Jobs) == 0 && len(s.run.Runner) == 0 {
		return act.StatusPending
	}
	st := act.StatusSuccess
	for _, j := range s.run.Jobs {
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
	if s.runErr != "" {
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
func (s *Session) cmdline() string {
	args, err := s.settings.Args()
	if err != nil {
		return "act"
	}
	redactArgs(args)
	return "act " + shellquote.Join(args...)
}

func redactArgs(args []string) {
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
}

func contains(list []string, v string) bool {
	return slices.Contains(list, v)
}
