package act

import "testing"

func feed(r *Run, lines ...string) {
	for _, l := range lines {
		r.Feed(l)
	}
}

func TestFeedParsesJobsAndSteps(t *testing.T) {
	var r Run
	feed(&r,
		`time="x" level=info msg="Using docker host"`,
		`[CI/build] ⭐  Run Set up job`,
		`[CI/build]   🚀  Start image=catthehacker/ubuntu:act-latest`,
		`[CI/build]   🐳  docker pull image=catthehacker/ubuntu:act-latest`,
		`[CI/build]   ✅  Success - Set up job`,
		`[CI/build] ⭐  Run Main actions/checkout@v4`,
		`[CI/build]   | Checking out the ref`,
		`[CI/build]   ✅  Success - Main actions/checkout@v4 [1.5s]`,
		`[CI/build] ⭐  Run Main go test`,
		`[CI/build]   | ok  	pkg	0.3s`,
		`[CI/build]   ❌  Failure - Main go test [2.0s]`,
		`[CI/build] 🏁  Job failed`,
		`[CI/lint ] ⭐  Run Set up job`,
		`[CI/lint ]   ✅  Success - Set up job`,
		`[CI/lint ] 🏁  Job succeeded`,
	)

	if len(r.Jobs) != 2 {
		t.Fatalf("jobs=%d, want 2", len(r.Jobs))
	}
	if len(r.Runner) != 1 {
		t.Fatalf("runner lines=%d, want 1", len(r.Runner))
	}

	build := r.Jobs[0]
	if build.Name != "build" || build.Status != StatusFailure {
		t.Fatalf("build: name=%q status=%v", build.Name, build.Status)
	}
	if len(build.Steps) != 3 {
		t.Fatalf("build steps=%d, want 3", len(build.Steps))
	}
	setup, checkout, test := build.Steps[0], build.Steps[1], build.Steps[2]
	if setup.Name != "Set up job" || setup.Status != StatusSuccess {
		t.Fatalf("setup: name=%q status=%v", setup.Name, setup.Status)
	}
	if len(setup.Lines) != 2 {
		t.Fatalf("setup lines=%d, want 2", len(setup.Lines))
	}
	if checkout.Stage != "Main" || checkout.Name != "actions/checkout@v4" || checkout.Dur != "1.5s" {
		t.Fatalf("checkout: %+v", checkout)
	}
	if test.Status != StatusFailure || len(test.Lines) != 1 {
		t.Fatalf("test: %+v", test)
	}
	if lint := r.Jobs[1]; lint.Status != StatusSuccess || lint.Name != "lint" {
		t.Fatalf("lint: name=%q status=%v", lint.Name, lint.Status)
	}
}

func TestFeedDryRunAndPadding(t *testing.T) {
	var r Run
	feed(&r,
		`*DRYRUN* [CI/lint ] ⭐  Run Set up job`,
		`*DRYRUN* [CI/lint ]   ✅  Success - Set up job`,
	)
	if len(r.Jobs) != 1 || r.Jobs[0].Key != "CI/lint" {
		t.Fatalf("jobs=%+v", r.Jobs)
	}
}

func TestFeedSkippedDebugStep(t *testing.T) {
	var r Run
	feed(&r,
		`[CI/build] [DEBUG] Skipping step 'deploy' due to 'if'`,
	)
	j := r.Jobs[0]
	if len(j.Steps) != 1 || j.Steps[0].Status != StatusSkipped || j.Steps[0].Name != "deploy" {
		t.Fatalf("steps=%+v", j.Steps)
	}
}

func TestFeedAnsiAndRunnerLines(t *testing.T) {
	var r Run
	r.Feed("\x1b[0mplain text\x1b[0m")
	if len(r.Runner) != 1 || r.Runner[0] != "plain text" {
		t.Fatalf("runner=%q", r.Runner)
	}
}

func TestOrphanLinesKeepOrder(t *testing.T) {
	var r Run
	feed(&r,
		`[CI/build] ⭐  Run Set up job`,
		`[CI/build]   ✅  Success - Set up job`,
		`[CI/build] unable to get git ref: reference not found`,
		`[CI/build] ⭐  Run Main step`,
	)
	j := r.Jobs[0]
	if len(j.Items) != 3 || j.Items[1].Line == "" {
		t.Fatalf("items=%+v", j.Items)
	}
}
