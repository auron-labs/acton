// Package act runs the act CLI and models its output: a Runner for the
// process, and a Run/Job/Step tree built by feeding it output lines.
package act

import (
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

// Step is one step of a job's log, holding the output lines act printed
// between its "⭐ Run" and "✅/❌" markers.
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

// Duration returns the label shown next to a step name: the duration act
// printed, or elapsed time while it runs.
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

// Item is one entry of a job's ordered item list: either a step or a
// job-level output line that arrived outside any step.
type Item struct {
	Step *Step
	Line string
}

// Job is one act job stream, identified by its "[workflow/job]" log tag.
type Job struct {
	Key     string
	Name    string // display name: part after the last "/"
	Status  Status
	Items   []Item
	Steps   []*Step
	Started time.Time
	Ended   time.Time
	Raw     []string // every tagged line, verbatim

	current *Step
}

// Duration returns the label shown next to a job name: elapsed time between
// its first and last output (now while it runs).
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

func (j *Job) addLine(text string) {
	if j.current != nil && j.current.Status == StatusRunning {
		j.current.Lines = append(j.current.Lines, text)
		return
	}
	j.Items = append(j.Items, Item{Line: text})
}

func (j *Job) startStep(name string) {
	stage, disp := "", name
	for _, p := range []string{"Pre ", "Main ", "Post "} {
		if strings.HasPrefix(name, p) {
			stage = strings.TrimSuffix(p, " ")
			break
		}
	}
	if stage == "Main" {
		disp = name[len("Main "):]
	}
	s := &Step{Name: disp, Stage: stage, Status: StatusRunning, Started: time.Now(), Open: true}
	j.Steps = append(j.Steps, s)
	j.Items = append(j.Items, Item{Step: s})
	j.current = s
}

// endStep closes the running step. text is the "✅/❌ … - <stage> <name>
// [<duration>]" message.
func (j *Job) endStep(st Status, text string) {
	dur := ""
	if i := strings.LastIndex(text, " ["); i >= 0 && strings.HasSuffix(strings.TrimSpace(text), "]") {
		dur = strings.TrimSuffix(strings.TrimSpace(text[i+2:]), "]")
		if d, err := time.ParseDuration(dur); err == nil {
			dur = d.Round(time.Millisecond).String()
		}
	}
	if j.current == nil {
		// An end marker without an open step: keep it visible.
		s := &Step{Name: stepResultName(text), Status: st, Started: time.Now(), Ended: time.Now(), Dur: dur}
		j.Steps = append(j.Steps, s)
		j.Items = append(j.Items, Item{Step: s})
		return
	}
	j.current.Status = st
	j.current.Ended = time.Now()
	j.current.Dur = dur
	j.current.Open = st == StatusFailure
	j.current = nil
}

func (j *Job) skippedStep(name string) {
	s := &Step{Name: name, Status: StatusSkipped, Started: time.Now(), Ended: time.Now()}
	j.Steps = append(j.Steps, s)
	j.Items = append(j.Items, Item{Step: s})
}

// stepResultName extracts "<stage> <name>" from a step result message like
// "✅  Success - Main go test [2.3s]" or "❌  Failure - Set up job".
func stepResultName(text string) string {
	if i := strings.Index(text, " - "); i >= 0 {
		text = text[i+3:]
	}
	if i := strings.LastIndex(text, " ["); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}

// Run is the parsed model of one act invocation. Feed consumes raw output
// lines on the UI thread; the view renders jobs and steps from it.
type Run struct {
	Jobs   []*Job
	Runner []string // lines without a [job] tag, in arrival order
	byKey  map[string]*Job
}

// RunnerJobKey selects the pseudo entry for untagged runner output.
const RunnerJobKey = "\x00runner"

// Reset drops all parsed output for a fresh run.
func (r *Run) Reset() {
	r.Jobs = nil
	r.Runner = nil
	r.byKey = nil
}

func (r *Run) job(key string) *Job {
	if r.byKey == nil {
		r.byKey = map[string]*Job{}
	}
	if j, ok := r.byKey[key]; ok {
		return j
	}
	name := key
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	j := &Job{Key: key, Name: name, Status: StatusRunning, Started: time.Now()}
	r.byKey[key] = j
	r.Jobs = append(r.Jobs, j)
	return j
}

var (
	tagRe  = regexp.MustCompile(`^\[([^\]]+)\]\s?(.*)$`)
	ansiRe = regexp.MustCompile("\x1b(?:\\[[0-9;]*[a-zA-Z]|\\][^\x07]*\x07|[()][0-9A-B]|[=>])")
)

// Feed parses one act output line into the run model. Lines are the raw
// process output with ANSI escapes still present; they are stripped here.
func (r *Run) Feed(line string) {
	line = ansiRe.ReplaceAllString(line, "")
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return
	}

	body := strings.TrimPrefix(line, "*DRYRUN* ")
	m := tagRe.FindStringSubmatch(body)
	if m == nil {
		r.Runner = append(r.Runner, strings.TrimSpace(line))
		return
	}

	tag := strings.TrimSpace(m[1])
	msg := m[2]
	if tag == "" {
		r.Runner = append(r.Runner, strings.TrimSpace(line))
		return
	}
	j := r.job(tag)
	j.Raw = append(j.Raw, strings.TrimRight(line, " "))

	t := strings.TrimLeft(msg, " ")
	switch {
	case strings.HasPrefix(t, "⭐"):
		name := strings.TrimSpace(strings.TrimPrefix(t, "⭐"))
		name = strings.TrimSpace(strings.TrimPrefix(name, "Run"))
		j.startStep(name)
	case strings.HasPrefix(t, "✅"):
		j.endStep(StatusSuccess, t)
	case strings.HasPrefix(t, "❌"):
		j.endStep(StatusFailure, t)
	case strings.HasPrefix(t, "⏭"):
		if j.current != nil {
			j.endStep(StatusSkipped, t)
		} else {
			j.addLine(msg)
		}
	case strings.HasPrefix(t, "🏁"):
		switch {
		case strings.Contains(t, "succeed"):
			j.Status = StatusSuccess
		case strings.Contains(t, "fail"):
			j.Status = StatusFailure
		case strings.Contains(t, "cancel"):
			j.Status = StatusCancelled
		case strings.Contains(t, "skip"):
			j.Status = StatusSkipped
		}
		j.Ended = time.Now()
		if j.current != nil {
			j.current.Status = StatusSkipped
			j.current.Ended = j.Ended
			j.current = nil
		}
	case strings.HasPrefix(t, "🚧") && strings.Contains(t, "Skipping"):
		j.Status = StatusSkipped
		j.Ended = time.Now()
		j.addLine(msg)
	case strings.HasPrefix(t, "| "):
		j.addLine(strings.TrimPrefix(t, "| "))
	case strings.HasPrefix(t, "[DEBUG] "):
		rest := strings.TrimPrefix(t, "[DEBUG] ")
		if name, ok := skippedStepName(rest); ok {
			j.skippedStep(name)
		} else {
			j.addLine(msg)
		}
	default:
		j.addLine(msg)
	}
}

// skippedStepName reads "[DEBUG] Skipping step '<name>' due to …" lines act
// prints in verbose mode for steps whose if: condition is false.
func skippedStepName(msg string) (string, bool) {
	const p = "Skipping step '"
	if !strings.HasPrefix(msg, p) {
		return "", false
	}
	rest := msg[len(p):]
	i := strings.Index(rest, "'")
	if i <= 0 {
		return "", false
	}
	return rest[:i], true
}

// Finish marks still-running steps of all jobs as skipped; used when
// the process ends without 🏁 markers (e.g. killed).
func (r *Run) Finish(st Status) {
	for _, j := range r.Jobs {
		if j.Status == StatusRunning {
			j.Status = st
			j.Ended = time.Now()
		}
		if j.current != nil {
			j.current.Status = st
			j.current.Ended = time.Now()
			j.current = nil
		}
	}
}
