// Package act embeds the act runner and models its JSON output as a Run/Job/Step tree.
package act

import (
	"encoding/json"
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
	Name    string // display name; "Pre "/"Post " prefix kept, "Main" dropped
	Stage   string // "Pre", "Main", "Post", or "" for pseudo steps
	Status  Status
	Lines   []string
	Started time.Time
	Ended   time.Time
	Dur     string // duration reported by act, e.g. "1.234s"
	Open    bool
}

// Duration returns the reported duration, or elapsed time while a step runs.
func (s *Step) Duration() string {
	if s.Dur != "" {
		return s.Dur
	}
	if s.Started.IsZero() {
		return ""
	}
	end := s.Ended
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(s.Started).Round(time.Second).String()
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

	byStep map[string]*Step
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
	return end.Sub(j.Started).Round(time.Second).String()
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
	if name == "" && len(e.StepID) > 0 {
		name = e.StepID[len(e.StepID)-1]
	}
	if e.Stage == "Pre" || e.Stage == "Post" {
		name = e.Stage + " " + name
	}
	s := &Step{Name: name, Stage: e.Stage, Status: StatusRunning, Started: at, Open: true}
	if j.byStep == nil {
		j.byStep = make(map[string]*Step)
	}
	j.byStep[key] = s
	j.Steps = append(j.Steps, s)
	j.Items = append(j.Items, Item{Step: s})
	return s
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
		s.Status, s.Ended, s.Open = status, at, status == StatusFailure
		if e.ExecutionTime != nil {
			s.Dur = e.ExecutionTime.Round(time.Millisecond).String()
		}
		return
	}
	// act has no structured start-event field. Restrict recognition to records
	// with step metadata, never user output, and use the metadata for the name.
	if (e.Step != "" || len(e.StepID) > 0) && strings.HasPrefix(strings.TrimSpace(msg), "⭐ Run ") {
		j.step(e, at)
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
	if e.Step != "" || len(e.StepID) > 0 {
		s := j.byStep[e.stepKey()]
		if s == nil && raw {
			s = j.step(e, at)
		}
		if s != nil {
			s.Lines = append(s.Lines, msg)
			return
		}
	}
	j.Items = append(j.Items, Item{Line: msg})
}

func (j *Job) finishSteps(st Status, at time.Time) {
	for _, s := range j.Steps {
		if s.Status == StatusRunning {
			s.Status, s.Ended = st, at
		}
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
