// Package act embeds the act runner and models its JSON output as a Run/Job/Step tree.
package act

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

// Status is the state of a job or step, matching GitHub Actions naming.
type Status int

// Status values in lifecycle order.
const (
	StatusPending Status = iota
	StatusRunning
	StatusSuccess
	StatusFailure
	StatusSkipped
	StatusCancelled
)

func (s Status) String() string {
	switch s {
	case StatusRunning:
		return "Running"
	case StatusSuccess:
		return "Succeeded"
	case StatusFailure:
		return "Failed"
	case StatusSkipped:
		return "Skipped"
	case StatusCancelled:
		return "Cancelled"
	case StatusPending:
		return "Pending"
	}
	return "Pending"
}

// Done reports whether the status is a finished state.
func (s Status) Done() bool {
	return s == StatusSuccess || s == StatusFailure || s == StatusSkipped || s == StatusCancelled
}

// Step is one workflow step or runner setup/cleanup step.
type Step struct {
	Name        string
	Stage       string
	Source      string
	Fetched     bool
	Parent      *Step `json:"-"`
	Children    []*Step
	Status      Status
	Lines       []string
	Started     time.Time
	Ended       time.Time
	Dur         time.Duration
	Open        bool
	UserToggled bool

	ownStatus Status
	ownEnded  time.Time
}

// Elapsed keeps timing numeric for filters and the waterfall.
func (s *Step) Elapsed(now time.Time) time.Duration {
	if len(s.Children) == 0 && s.Dur > 0 {
		return s.Dur
	}
	if s.Started.IsZero() {
		return 0
	}
	end := s.Ended
	if end.IsZero() {
		end = now
	}
	return max(0, end.Sub(s.Started))
}

// FormatDuration is shared by finished and running steps and jobs.
func FormatDuration(d time.Duration) string {
	d = max(0, d)
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	seconds := int64(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
}

// Duration formats a step's reported or elapsed duration for display.
func (s *Step) Duration() string {
	if s.Started.IsZero() && s.Dur == 0 {
		return ""
	}
	return FormatDuration(s.Elapsed(time.Now()))
}

// Keep the parent's own result when aggregating child failures and time bounds.
func (s *Step) refresh() {
	s.Status, s.Ended = s.ownStatus, s.ownEnded
	running := s.ownStatus == StatusRunning
	for _, child := range s.Children {
		if s.Started.IsZero() || child.Started.Before(s.Started) {
			s.Started = child.Started
		}
		if child.Ended.After(s.Ended) {
			s.Ended = child.Ended
		}
		running = running || child.Status == StatusRunning || child.Ended.IsZero()
		if child.Status == StatusFailure {
			s.Status = StatusFailure
		}
	}
	if running {
		s.Ended = time.Time{}
		if s.Status != StatusFailure {
			s.Status = StatusRunning
		}
	}
	if !s.UserToggled {
		s.Open = s.Status == StatusRunning || s.Status == StatusFailure
	}
	if s.Parent != nil {
		s.Parent.refresh()
	}
}

// Item is either a step or a job-level output line, in arrival order.
type Item struct {
	Step *Step
	Line string
}

// Job is one act job stream, including its matrix variant.
type Job struct {
	Key     string
	Name    string
	Status  Status
	Items   []Item
	Steps   []*Step
	Started time.Time
	Ended   time.Time
	Raw     []string // original JSON records, verbatim

	byStep        map[string]*Step
	pendingClones []actionClone
}

// Duration returns elapsed time between the job's first and last output.
func (j *Job) Duration() string {
	if j.Started.IsZero() {
		return ""
	}
	end := j.Ended
	if end.IsZero() {
		end = time.Now()
	}
	return FormatDuration(end.Sub(j.Started))
}

// logEvent is act's logrus JSON envelope. StepID is a path for composite
// actions; setup/cleanup use the spelling "stepid", also matched by Go's decoder.
type logEvent struct {
	Message       *string        `json:"msg"`
	Time          string         `json:"time"`
	Job           string         `json:"job"`
	JobID         string         `json:"jobID"`
	Matrix        map[string]any `json:"matrix"`
	Step          string         `json:"step"`
	Source        string         `json:"source"`
	Uses          string         `json:"uses"`
	StepID        []string       `json:"stepID"`
	Stage         string         `json:"stage"`
	RawOutput     bool           `json:"raw_output"`
	Command       string         `json:"command"`
	StepResult    string         `json:"stepResult"`
	JobResult     string         `json:"jobResult"`
	ExecutionTime *time.Duration `json:"executionTime"`
}

func (e logEvent) timestamp() time.Time {
	if t, err := time.Parse(time.RFC3339Nano, e.Time); err == nil {
		return t
	}
	return time.Now()
}

func (e logEvent) jobKey() string {
	// JSON encoding gives matrix maps stable key ordering and avoids separator
	// collisions in names and IDs. JobID alone is not unique across workflows.
	b, _ := json.Marshal([]any{e.Job, e.JobID, e.Matrix})
	return string(b)
}

func (e logEvent) stepKey() string {
	var identity any = e.StepID
	if len(e.StepID) == 0 {
		identity = e.Step
	}
	b, _ := json.Marshal([]any{e.Stage, identity})
	return string(b)
}

func (j *Job) step(e logEvent, at time.Time) *Step {
	key := e.stepKey()
	if s := j.byStep[key]; s != nil {
		return s
	}
	name := e.Step
	if len(e.StepID) > 1 || name == "" && len(e.StepID) > 0 {
		name = e.StepID[len(e.StepID)-1]
	}
	if len(e.StepID) <= 1 && (e.Stage == "Pre" || e.Stage == "Post") {
		name = e.Stage + " " + name
	}
	s := &Step{Name: name, Stage: e.Stage, Status: StatusRunning, ownStatus: StatusRunning, Started: at, Open: true}
	if j.byStep == nil {
		j.byStep = make(map[string]*Step)
	}
	if len(e.StepID) > 1 {
		parentEvent := e
		parentEvent.StepID = e.StepID[:len(e.StepID)-1]
		s.Parent = j.step(parentEvent, at)
		s.Parent.Children = append(s.Parent.Children, s)
	} else {
		j.Steps = append(j.Steps, s)
		j.Items = append(j.Items, Item{Step: s})
	}
	j.byStep[key] = s
	return s
}

var (
	actionSourceRe = regexp.MustCompile(`^(?:[[:alnum:]_.-]+/[[:alnum:]_./-]+@[^[:space:]]+|\./[^[:space:]]+)$`)
	shaRe          = regexp.MustCompile(`^[a-fA-F0-9]{8,40}$`)
	cloneRe        = regexp.MustCompile(`^☁\s+git clone '([^']+)' # ref=(\S+)`)
)

type actionClone struct{ repo, line string }

func shortSource(source string) string {
	repo, ref, ok := strings.Cut(source, "@")
	if ok && shaRe.MatchString(ref) {
		return repo + "@" + ref[:7]
	}
	return source
}

func actionRepo(source string) string {
	repo, _, _ := strings.Cut(source, "@")
	parts := strings.Split(repo, "/")
	if len(parts) < 2 {
		return ""
	}
	return strings.ToLower(strings.Join(parts[:2], "/"))
}

func (j *Job) describeStep(s *Step, e logEvent, name string) {
	if name != "" {
		displayName := name
		if actionSourceRe.MatchString(name) {
			displayName, _, _ = strings.Cut(name, "@")
			if strings.HasPrefix(displayName, "./") {
				displayName = path.Base(displayName)
			}
		}
		s.Name = displayName
		if s.Parent == nil && (e.Stage == "Pre" || e.Stage == "Post") {
			s.Name = e.Stage + " " + displayName
		}
	}
	source := e.Source
	if source == "" {
		source = e.Uses
	}
	if source == "" && actionSourceRe.MatchString(name) {
		source = name
	}
	if source == "" && s.Parent == nil && actionSourceRe.MatchString(e.Step) {
		source = e.Step
	}
	if source != "" {
		s.Source = shortSource(source)
	}
	for i, clone := range j.pendingClones {
		if actionRepo(s.Source) != clone.repo {
			continue
		}
		s.Fetched = true
		s.Lines = append(s.Lines, clone.line)
		j.pendingClones = append(j.pendingClones[:i], j.pendingClones[i+1:]...)
		break
	}
}

func resultStatus(result string) (Status, bool) {
	switch result {
	case "success":
		return StatusSuccess, true
	case "failure":
		return StatusFailure, true
	case "skipped":
		return StatusSkipped, true
	case "cancelled":
		return StatusCancelled, true
	default:
		return StatusPending, false
	}
}

// Run consumes act JSON records on the UI thread.
type Run struct {
	Jobs   []*Job
	Runner []string // decoded runner messages and non-JSON diagnostics
	byKey  map[string]*Job
}

// RunnerJobKey selects the pseudo entry for runner output.
const RunnerJobKey = "\x00runner"

// Reset drops all parsed output for a fresh run.
func (r *Run) Reset() {
	r.Jobs = nil
	r.Runner = nil
	r.byKey = nil
}

func (r *Run) job(e logEvent, at time.Time) *Job {
	if r.byKey == nil {
		r.byKey = map[string]*Job{}
	}
	key := e.jobKey()
	if j := r.byKey[key]; j != nil {
		return j
	}
	name := e.Job
	if name == "" {
		name = e.JobID
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	j := &Job{Key: key, Name: name, Status: StatusRunning, Started: at}
	r.byKey[key] = j
	r.Jobs = append(r.Jobs, j)
	return j
}

var ansiRe = regexp.MustCompile("\x1b(?:\\[[0-9;]*[a-zA-Z]|\\][^\x07]*\x07|[()][0-9A-B]|[=>])")

// Feed decodes one JSON record. Non-JSON diagnostics remain visible in the
// runner pane; they are never interpreted as job or step lifecycle events.
func (r *Run) Feed(line string) {
	line = strings.TrimRight(line, "\r\n")
	var e logEvent
	if err := json.Unmarshal([]byte(line), &e); err != nil || e.Message == nil {
		text := strings.TrimSpace(ansiRe.ReplaceAllString(line, ""))
		if text != "" {
			r.Runner = append(r.Runner, text)
		}
		return
	}
	msg := ansiRe.ReplaceAllString(*e.Message, "")
	if e.Command != "ignored" {
		msg = annotationMessage(msg)
	}
	if e.Job == "" && e.JobID == "" {
		r.Runner = append(r.Runner, msg)
		return
	}
	at := e.timestamp()
	j := r.job(e, at)
	j.Raw = append(j.Raw, line)

	// Workflow output is data, even when it contains act's lifecycle markers.
	if e.RawOutput {
		r.addMessage(j, e, at, msg, true)
		return
	}
	if status, ok := resultStatus(e.JobResult); ok {
		j.Status, j.Ended = status, at
		j.finishSteps(StatusSkipped, at)
		return
	}
	if status, ok := resultStatus(e.StepResult); ok && (e.Step != "" || len(e.StepID) > 0) {
		s := j.step(e, at)
		s.ownStatus, s.ownEnded = status, at
		if e.ExecutionTime != nil {
			s.Dur = *e.ExecutionTime
		}
		j.describeStep(s, e, "")
		s.refresh()
		return
	}
	// act has no structured start-event field. Restrict recognition to records
	// with step metadata, never user output; composite metadata names the parent.
	if (e.Step != "" || len(e.StepID) > 0) && strings.HasPrefix(strings.TrimSpace(msg), "⭐ Run ") {
		s := j.step(e, at)
		name := strings.TrimPrefix(strings.TrimSpace(msg), "⭐ Run ")
		for _, stage := range []string{"Main ", "Pre ", "Post "} {
			if strings.HasPrefix(name, stage) {
				name = strings.TrimPrefix(name, stage)
				break
			}
		}
		j.describeStep(s, e, name)
		s.refresh()
		return
	}
	// Unsupported platforms do not have a jobResult in act 0.2.89.
	if strings.HasPrefix(strings.TrimSpace(msg), "🚧  Skipping unsupported platform") {
		j.Status, j.Ended = StatusSkipped, at
	}
	r.addMessage(j, e, at, msg, false)
}

var (
	annotationRe       = regexp.MustCompile(`(?is)^\s*(?:[❗🚧💬]\s*)?::(error|warning|notice)(?: ([^\r\n]*?))?::(.*)$`)
	annotationData     = strings.NewReplacer("%0D", "\r", "%0A", "\n", "%25", "%")
	annotationProperty = strings.NewReplacer("%0D", "\r", "%0A", "\n", "%3A", ":", "%2C", ",", "%25", "%")
)

// Parse only the masked message; act's raw annotation fields can contain secrets.
func annotationMessage(msg string) string {
	m := annotationRe.FindStringSubmatch(msg)
	if m == nil {
		return msg
	}
	properties := make(map[string]string)
	for _, pair := range strings.Split(m[2], ",") {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			properties[strings.ToLower(key)] = annotationProperty.Replace(value)
		}
	}
	location := properties["file"]
	if line := properties["line"]; line != "" {
		location += ":" + line
		if col := properties["col"]; col != "" {
			location += ":" + col
		}
	}
	parts := []string{strings.ToUpper(m[1][:1]) + strings.ToLower(m[1][1:])}
	if location != "" {
		parts = append(parts, location)
	}
	if title := properties["title"]; title != "" {
		parts = append(parts, title)
	}
	return strings.Join(append(parts, annotationData.Replace(m[3])), ": ")
}

func (r *Run) addMessage(j *Job, e logEvent, at time.Time, msg string, raw bool) {
	msg = strings.TrimRight(msg, "\r\n")
	if !raw {
		if match := cloneRe.FindStringSubmatch(strings.TrimSpace(msg)); match != nil {
			u, err := url.Parse(match[1])
			if err == nil {
				repo := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
				j.pendingClones = append(j.pendingClones, actionClone{repo: strings.ToLower(repo), line: msg})
				return
			}
		}
	}

	if e.Step != "" || len(e.StepID) > 0 {
		s := j.byStep[e.stepKey()]
		if s == nil && (raw || len(e.StepID) > 1) {
			s = j.step(e, at)
		}
		if s != nil {
			if !raw && s.Source == "" && strings.HasPrefix(msg, "Wrote command ") {
				s.Source = "run"
			}
			s.Lines = append(s.Lines, msg)
			j.describeStep(s, e, "")
			s.refresh()
			return
		}
	}
	j.Items = append(j.Items, Item{Line: msg})
}

func (j *Job) finishSteps(st Status, at time.Time) {
	var finish func(*Step)
	finish = func(s *Step) {
		for _, child := range s.Children {
			finish(child)
		}
		if s.ownStatus == StatusRunning {
			s.ownStatus, s.ownEnded = st, at
		}
		s.refresh()
	}
	for _, s := range j.Steps {
		finish(s)
	}
}

// Finish closes unfinished jobs and steps when the process exits.
func (r *Run) Finish(st Status) {
	at := time.Now()
	for _, j := range r.Jobs {
		if j.Status == StatusRunning {
			j.Status, j.Ended = st, at
		}
		j.finishSteps(st, at)
	}
}
