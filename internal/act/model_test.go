package act

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func feed(r *Run, lines ...string) {
	for _, l := range lines {
		r.Feed(l)
	}
}

func TestFeedParsesJobsAndSteps(t *testing.T) {
	var r Run
	feed(&r,
		`{"level":"info","msg":"Using docker host"}`,
		`{"job":"CI/build","jobID":"build","step":"Set up job","stepid":["--setup-job"],"msg":"⭐ Run Set up job"}`,
		`{"job":"CI/build","jobID":"build","step":"Set up job","stepid":["--setup-job"],"msg":"🚀 Start image=catthehacker/ubuntu:act-latest"}`,
		`{"job":"CI/build","jobID":"build","step":"Set up job","stepid":["--setup-job"],"msg":"🐳 docker pull image=catthehacker/ubuntu:act-latest"}`,
		`{"job":"CI/build","jobID":"build","step":"Set up job","stepid":["--setup-job"],"stepResult":"success","msg":"✅ Success - Set up job"}`,
		`{"job":"CI/build","jobID":"build","step":"actions/checkout@v4","stepID":["checkout"],"stage":"Main","msg":"⭐ Run Main actions/checkout@v4"}`,
		`{"job":"CI/build","jobID":"build","step":"actions/checkout@v4","stepID":["checkout"],"stage":"Main","raw_output":true,"msg":"Checking out the ref"}`,
		`{"job":"CI/build","jobID":"build","step":"actions/checkout@v4","stepID":["checkout"],"stage":"Main","stepResult":"success","executionTime":1500000000,"msg":"result text need not be parsed"}`,
		`{"job":"CI/build","jobID":"build","step":"go test","stepID":["test"],"stage":"Main","msg":"⭐ Run Main go test"}`,
		`{"job":"CI/build","jobID":"build","step":"go test","stepID":["test"],"stage":"Main","raw_output":true,"msg":"ok\t pkg\t0.3s"}`,
		`{"job":"CI/build","jobID":"build","step":"go test","stepID":["test"],"stage":"Main","stepResult":"failure","executionTime":2000000000,"msg":"failure"}`,
		`{"job":"CI/build","jobID":"build","jobResult":"failure","msg":"job result"}`,
		`{"job":"CI/lint","jobID":"lint","step":"Set up job","stepid":["--setup-job"],"stepResult":"success","msg":"setup done"}`,
		`{"job":"CI/lint","jobID":"lint","jobResult":"success","msg":"done"}`,
	)
	if len(r.Jobs) != 2 || len(r.Runner) != 1 || r.Runner[0] != "Using docker host" {
		t.Fatalf("jobs=%d runner=%q", len(r.Jobs), r.Runner)
	}
	build := r.Jobs[0]
	if build.Name != "build" || build.Status != StatusFailure || len(build.Steps) != 3 {
		t.Fatalf("build: %+v", build)
	}
	setup, checkout, test := build.Steps[0], build.Steps[1], build.Steps[2]
	if setup.Name != "Set up job" || setup.Status != StatusSuccess || len(setup.Lines) != 2 {
		t.Fatalf("setup: %+v", setup)
	}
	if checkout.Stage != "Main" || checkout.Name != "actions/checkout" || checkout.Dur != 1500*time.Millisecond || checkout.Open {
		t.Fatalf("checkout: %+v", checkout)
	}
	if test.Status != StatusFailure || len(test.Lines) != 1 || !test.Open || test.Dur != 2*time.Second {
		t.Fatalf("test: %+v", test)
	}
	if lint := r.Jobs[1]; lint.Status != StatusSuccess || lint.Name != "lint" {
		t.Fatalf("lint: %+v", lint)
	}
	if !strings.Contains(build.Raw[0], `"stepid"`) {
		t.Fatalf("raw record not preserved: %q", build.Raw[0])
	}
}

func TestFeedRawOutputIsNeverLifecycle(t *testing.T) {
	var r Run
	messages := []string{"⭐ Run Main fake", "✅ Success - fake", "❌ Failure - fake", "🏁 Job failed", "⏭ skipped", "[other/job] hi", "", "  indented\nsecond line", "{\"jobResult\":\"failure\"}"}
	for _, msg := range messages {
		b, err := json.Marshal(map[string]any{
			"job": "CI/build", "step": "script", "stepID": []string{"script"}, "stage": "Main", "raw_output": true,
			"msg": msg, "stepResult": "failure", "jobResult": "failure",
		})
		if err != nil {
			t.Fatal(err)
		}
		r.Feed(string(b))
	}
	j := r.Jobs[0]
	if j.Status != StatusRunning || len(j.Steps) != 1 || j.Steps[0].Status != StatusRunning {
		t.Fatalf("user output changed lifecycle: %+v", j)
	}
	if got := j.Steps[0].Lines; strings.Join(got, "\x00") != strings.Join(messages, "\x00") {
		t.Fatalf("lines=%q, want %q", got, messages)
	}
}

func TestFeedStepIdentityAndStages(t *testing.T) {
	var r Run
	feed(&r,
		`{"job":"CI/build","step":"same name","stepID":["a"],"stage":"Main","msg":"⭐ Run Main same name"}`,
		`{"job":"CI/build","step":"same name","stepID":["a","child"],"stage":"Main","msg":"⭐ Run Main child"}`,
		`{"job":"CI/build","step":"same name","stepID":["a","child"],"stage":"Main","stepResult":"success","msg":"child done"}`,
		`{"job":"CI/build","step":"same name","stepID":["a"],"stage":"Main","raw_output":true,"msg":"parent output"}`,
		`{"job":"CI/build","step":"same name","stepID":["a"],"stage":"Main","stepResult":"success","msg":"parent done"}`,
		`{"job":"CI/build","step":"same name","stepID":["b"],"stage":"Main","stepResult":"skipped","msg":"skip without parsing name"}`,
		`{"job":"CI/build","step":"same name","stepID":["a"],"stage":"Post","msg":"⭐ Run Post same name"}`,
		`{"job":"CI/build","step":"same name","stepID":["a"],"stage":"Post","stepResult":"success","msg":"post done"}`,
	)
	j := r.Jobs[0]
	if len(j.Steps) != 3 {
		t.Fatalf("steps=%+v", j.Steps)
	}
	if s := j.Steps[0]; len(s.Lines) != 1 || s.Lines[0] != "parent output" || s.Status != StatusSuccess {
		t.Fatalf("parent: %+v", s)
	}
	if j.Steps[0].Children[0].Status != StatusSuccess || j.Steps[1].Status != StatusSkipped || j.Steps[2].Name != "Post same name" {
		t.Fatalf("incorrect composite/stage results: %+v", j.Steps)
	}
}

func TestFeedJobIdentity(t *testing.T) {
	var r Run
	feed(&r,
		`{"job":"CI/build","jobID":"build","matrix":{"os":"linux","go":"1.27"},"msg":"first"}`,
		`{"job":"CI/build","jobID":"build","matrix":{"go":"1.27","os":"linux"},"msg":"same variant"}`,
		`{"job":"CI/build","jobID":"build","matrix":{"os":"macos","go":"1.27"},"msg":"other variant"}`,
		`{"job":"Release/build","jobID":"build","matrix":{"os":"linux","go":"1.27"},"msg":"other workflow"}`,
		`{"job":"CI/build","jobID":"other","matrix":{"os":"linux","go":"1.27"},"msg":"other ID"}`,
	)
	if len(r.Jobs) != 4 || len(r.Jobs[0].Items) != 2 {
		t.Fatalf("jobs=%+v", r.Jobs)
	}
}

func TestFeedDiagnostics(t *testing.T) {
	var r Run
	feed(&r,
		"\x1b[0mplain text\x1b[0m",
		`[CI/build] ⭐ Run Main not a structured event`,
		`{"msg":`,
		`{"something":"else"}`,
		`null`,
		`{"msg":123}`,
		`{"level":"warning","msg":"\u001b[31mwarning\u001b[0m"}`,
		"  ",
	)
	if len(r.Jobs) != 0 || len(r.Runner) != 7 || r.Runner[0] != "plain text" || r.Runner[6] != "warning" {
		t.Fatalf("jobs=%+v runner=%q", r.Jobs, r.Runner)
	}
}

func TestFeedTimestampsAndFinish(t *testing.T) {
	var r Run
	feed(&r,
		`{"job":"CI/build","step":"test","stepID":["test"],"stage":"Main","time":"2026-10-28T12:00:00Z","msg":"⭐ Run Main test"}`,
		`{"job":"CI/build","step":"test","stepID":["test"],"stage":"Main","time":"2026-10-28T12:00:02Z","stepResult":"failure","executionTime":1234567890,"msg":"done"}`,
		`{"job":"CI/build","jobResult":"success","time":"2026-10-28T12:00:03Z","msg":"continued on error"}`,
		`{"job":"CI/lint","step":"lint","stepID":["lint"],"stage":"Main","msg":"⭐ Run Main lint"}`,
	)
	r.Finish(StatusCancelled)
	j := r.Jobs[0]
	if j.Duration() != "3.0s" || j.Status != StatusSuccess || j.Steps[0].Dur != 1234567890 || j.Steps[0].Status != StatusFailure {
		t.Fatalf("finished job: %+v step: %+v", j, j.Steps[0])
	}
	if s := j.Steps[0]; !s.Started.Equal(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)) || s.Ended.Sub(s.Started) != 2*time.Second {
		t.Fatalf("timestamps: %+v", s)
	}
	if j := r.Jobs[1]; j.Status != StatusCancelled || j.Steps[0].Status != StatusCancelled || j.Steps[0].Ended.IsZero() {
		t.Fatalf("unfinished job: %+v", j)
	}
	r.Reset()
	if len(r.Jobs) != 0 || len(r.Runner) != 0 || r.byKey != nil {
		t.Fatalf("reset: %+v", r)
	}
}

func TestFeedOrphanLinesAndDryRun(t *testing.T) {
	var r Run
	feed(&r,
		`{"job":"CI/build","dryrun":true,"step":"Set up job","stepid":["--setup-job"],"msg":"⭐ Run Set up job"}`,
		`{"job":"CI/build","dryrun":true,"step":"Set up job","stepid":["--setup-job"],"stepResult":"success","msg":"done"}`,
		`{"job":"CI/build","dryrun":true,"msg":"unable to get git ref: reference not found"}`,
		`{"job":"CI/build","dryrun":true,"step":"test","stepID":["test"],"stage":"Main","msg":"⭐ Run Main test"}`,
	)
	j := r.Jobs[0]
	if len(j.Items) != 3 || j.Items[1].Line == "" || len(j.Steps) != 2 {
		t.Fatalf("items=%+v", j.Items)
	}
}

func TestFeedJobSkipped(t *testing.T) {
	for _, line := range []string{
		`{"job":"CI/build","jobResult":"skipped","msg":"condition false"}`,
		`{"job":"CI/build","msg":"🚧  Skipping unsupported platform -- Try running with -P"}`,
	} {
		var r Run
		r.Feed(line)
		r.Finish(StatusSuccess)
		if j := r.Jobs[0]; j.Status != StatusSkipped || j.Ended.IsZero() {
			t.Fatalf("skipped job: %+v", j)
		}
	}
}

func TestGitHubAnnotations(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"::error::failed", "Error: failed"},
		{"  ❗  ::error file=src/main.go,line=12,col=4,title=Compile%2C fail::bad%0Anext%25line", "Error: src/main.go:12:4: Compile, fail: bad\nnext%line"},
		{"  🚧  ::warning file=a%2Cb.go,line=3::literal%250A", "Warning: a,b.go:3: literal%0A"},
		{"::notice title=Info::ready", "Notice: Info: ready"},
		{"::error file=x", "::error file=x"},
		{"echo ::error::example", "echo ::error::example"},
		{"::add-mask::private", "::add-mask::private"},
	} {
		if got := annotationMessage(tc.input); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.input, got, tc.want)
		}
	}
	var run Run
	feed(&run,
		`{"job":"CI/build","step":"compile","stepID":["compile"],"msg":"⭐ Run Main compile"}`,
		`{"job":"CI/build","step":"compile","stepID":["compile"],"command":"error","arg":"unmasked-secret","msg":"  ❗  ::error file=src/main.go,line=12::***"}`,
		`{"job":"CI/build","step":"compile","stepID":["compile"],"command":"ignored","msg":"::error::literal while commands stopped"}`,
		`{"job":"CI/build","step":"compile","stepID":["compile"],"stepResult":"success","msg":"done"}`,
	)
	step := run.Jobs[0].Steps[0]
	if len(step.Lines) != 2 || step.Lines[0] != "Error: src/main.go:12: ***" || step.Lines[1] != "::error::literal while commands stopped" || step.Status != StatusSuccess {
		t.Fatalf("annotations lost masking, command handling, or exit status: %+v", step)
	}
	if !strings.Contains(run.Jobs[0].Raw[1], "::error") {
		t.Fatal("raw log was changed")
	}
}

func eventJSON(t *testing.T, e logEvent) string {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCompositeSteps(t *testing.T) {
	for _, tc := range []struct {
		name, stage string
		failed      bool
	}{
		{"success", "Main", false}, {"child failure", "Main", true}, {"post children", "Post", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var run Run
			start := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
			send := func(ids []string, second int, msg, result string) {
				run.Feed(eventJSON(t, logEvent{Job: "CI/build", Step: "setup", StepID: ids, Stage: tc.stage, Time: start.Add(time.Duration(second) * time.Second).Format(time.RFC3339Nano), Message: &msg, StepResult: result}))
			}
			// A child can arrive before its parent start record.
			names := []string{"Restore cache", "Install tools", "composite-3"}
			for i, name := range names {
				id := []string{"3", "composite-" + string(rune('1'+i))}
				if i < 2 {
					send(id, i*3, "⭐ Run "+tc.stage+" "+name, "")
				} else {
					send(id, i*3, "output without a start marker", "")
				}
				result := "success"
				if tc.failed && i == 1 {
					result = "failure"
				}
				send(id, i*3+2, "done", result)
				parent := run.Jobs[0].Steps[0]
				want := StatusRunning
				if tc.failed && i >= 1 {
					want = StatusFailure
				}
				if parent.Status != want || !parent.Open {
					t.Fatalf("during run: %+v", parent)
				}
			}
			send([]string{"3"}, 7, "parent done", "success")
			j := run.Jobs[0]
			if len(j.Steps) != 1 || len(j.Items) != 1 {
				t.Fatalf("children leaked into top level: %+v", j)
			}
			parent := j.Steps[0]
			want := StatusSuccess
			if tc.failed {
				want = StatusFailure
			}
			if parent.Status != want || parent.Open != tc.failed || !parent.Started.Equal(start) || !parent.Ended.Equal(start.Add(8*time.Second)) {
				t.Fatalf("aggregate: %+v", parent)
			}
			if len(parent.Children) != 3 {
				t.Fatalf("children: %+v", parent.Children)
			}
			for i, child := range parent.Children {
				if child.Name != names[i] || child.Parent != parent || child.Stage != tc.stage {
					t.Fatalf("child %d: %+v", i, child)
				}
			}
		})
	}
}

func TestCloneAttachment(t *testing.T) {
	for _, tc := range []struct {
		name, source, url, wantSource string
		nested                        bool
	}{
		{"remote sha", "actions/cache@55c83451234567890", "https://github.com/actions/cache", "actions/cache@55c8345", false},
		{"nested action", "actions/cache@v4", "https://github.com/actions/cache.git", "actions/cache@v4", true},
		{"subdirectory", "actions/cache/restore@v4", "https://github.com/actions/cache", "actions/cache/restore@v4", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r Run
			clone := "  ☁  git clone '" + tc.url + "' # ref=v4"
			r.Feed(eventJSON(t, logEvent{Job: "CI/build", Message: &clone}))
			other := "⭐ Run Main other/action@v1"
			r.Feed(eventJSON(t, logEvent{Job: "CI/build", Step: "other/action@v1", StepID: []string{"1"}, Stage: "Main", Message: &other}))
			ids := []string{"2"}
			if tc.nested {
				ids = append(ids, "composite-1")
			}
			msg := "⭐ Run Main " + tc.source
			r.Feed(eventJSON(t, logEvent{Job: "CI/build", Step: "setup", StepID: ids, Stage: "Main", Message: &msg}))
			j := r.Jobs[0]
			step := j.Steps[1]
			if tc.nested {
				step = step.Children[0]
			}
			if !step.Fetched || step.Source != tc.wantSource || len(step.Lines) != 1 || step.Lines[0] != clone || j.Steps[0].Fetched {
				t.Fatalf("wrong attachment: %+v", step)
			}
			if len(j.Items) != 2 || !strings.Contains(j.Raw[0], "git clone") {
				t.Fatalf("clone row/raw: %+v", j)
			}
			// One clone only marks the next use, not subsequent uses of that repo.
			ids = []string{"4"}
			r.Feed(eventJSON(t, logEvent{Job: "CI/build", Step: tc.source, StepID: ids, Stage: "Main", Message: &msg}))
			if j.Steps[2].Fetched {
				t.Fatal("clone reused")
			}
		})
	}
}

func TestMessageNewlines(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"line\n", "line"}, {"line\r\n", "line"}, {"  line\nnext\r\n\n", "  line\nnext"}, {"\n", ""}, {"line  ", "line  "},
	} {
		t.Run(tc.input, func(t *testing.T) {
			var r Run
			record := eventJSON(t, logEvent{Job: "CI/build", Step: "script", StepID: []string{"1"}, RawOutput: true, Message: &tc.input})
			r.Feed(record)
			j := r.Jobs[0]
			if j.Steps[0].Lines[0] != tc.want || j.Raw[0] != record {
				t.Fatalf("lines=%q raw=%q", j.Steps[0].Lines, j.Raw)
			}
		})
	}
}

func TestDurationFormatting(t *testing.T) {
	for _, tc := range []struct {
		duration time.Duration
		want     string
	}{
		{0, "0.0s"}, {183 * time.Millisecond, "0.2s"}, {4300 * time.Millisecond, "4.3s"}, {14146 * time.Millisecond, "14.1s"}, {59 * time.Second, "59.0s"}, {time.Minute, "1m 0s"}, {167 * time.Second, "2m 47s"}, {time.Hour, "60m 0s"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			start := time.Unix(0, 0)
			step := &Step{Started: start, Ended: start.Add(tc.duration), Dur: tc.duration}
			job := &Job{Started: start, Ended: start.Add(tc.duration)}
			if FormatDuration(tc.duration) != tc.want || step.Duration() != tc.want || job.Duration() != tc.want {
				t.Fatalf("formatter=%s step=%s job=%s", FormatDuration(tc.duration), step.Duration(), job.Duration())
			}
		})
	}
}

func TestStepAutoExpansion(t *testing.T) {
	for _, tc := range []struct {
		name          string
		toggled, open bool
		result        string
		want          bool
	}{
		{"auto closes", false, true, "success", false},
		{"auto failure", false, true, "failure", true},
		{"user opened", true, true, "success", true},
		{"user closed", true, false, "success", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r Run
			feed(&r, `{"job":"CI/build","step":"test","stepID":["1"],"msg":"⭐ Run Main test"}`)
			step := r.Jobs[0].Steps[0]
			step.UserToggled, step.Open = tc.toggled, tc.open
			msg := "done"
			r.Feed(eventJSON(t, logEvent{Job: "CI/build", Step: "test", StepID: []string{"1"}, StepResult: tc.result, Message: &msg}))
			if step.Open != tc.want {
				t.Fatalf("open=%v", step.Open)
			}
		})
	}
}

func TestStepSources(t *testing.T) {
	for _, tc := range []struct{ label, uses, wantName, wantSource string }{
		{"actions/checkout@d3c42e1234567890", "", "actions/checkout", "actions/checkout@d3c42e1"},
		{"./.github/actions/setup", "", "setup", "./.github/actions/setup"},
		{"Restore cache", "actions/cache@55c83451234567890", "Restore cache", "actions/cache@55c8345"},
		{"Script with no type metadata", "", "Script with no type metadata", ""},
	} {
		t.Run(tc.label, func(t *testing.T) {
			var r Run
			msg := "⭐ Run Main " + tc.label
			r.Feed(eventJSON(t, logEvent{Job: "build", Step: tc.label, StepID: []string{"1"}, Stage: "Main", Uses: tc.uses, Message: &msg}))
			s := r.Jobs[0].Steps[0]
			if s.Name != tc.wantName || s.Source != tc.wantSource {
				t.Fatalf("name=%q source=%q", s.Name, s.Source)
			}
		})
	}
}
