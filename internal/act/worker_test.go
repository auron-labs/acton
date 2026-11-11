package act

import (
	"acton/internal/dockertest"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func startWorker(t *testing.T, dir string, args []string, flush func()) (*Runner, <-chan struct{}) {
	t.Helper()
	r := &Runner{}
	complete := make(chan struct{})
	if err := r.Start(dir, args, flush, func() { close(complete) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Stop(); r.Wait() })
	return r, complete
}

func finishWorker(t *testing.T, r *Runner, complete <-chan struct{}) ([]string, error) {
	t.Helper()
	select {
	case <-complete:
	case <-time.After(30 * time.Second):
		t.Fatal("worker run did not finish")
	}
	lines, done, err := r.Drain()
	if !done {
		t.Fatal("completion callback preceded done status")
	}
	return lines, err
}

func TestWorkerStreamsRun(t *testing.T) {
	dockertest.Require(t)
	dir := workflowProject(t, testWorkflow)
	host, hostSet := os.LookupEnv("DOCKER_HOST")
	r, complete := startWorker(t, dir, []string{"push", "-n", "-P", "ubuntu-latest=unused", "--no-cache-server"}, func() {})
	lines, err := finishWorker(t, r, complete)
	if err != nil {
		t.Fatal(err)
	}
	var run Run
	for _, line := range lines {
		run.Feed(line)
	}
	if len(run.Jobs) != 1 || run.Jobs[0].Status != StatusSuccess {
		t.Fatalf("unexpected jobs: %+v\n%s", run.Jobs, strings.Join(lines, "\n"))
	}
	if got, set := os.LookupEnv("DOCKER_HOST"); got != host || set != hostSet {
		t.Fatal("worker changed the parent's environment")
	}
}

func TestWorkerReportsErrors(t *testing.T) {
	if err := (&Runner{}).Start(t.TempDir(), []string{"--no-such-flag"}, func() {}, func() {}); err == nil {
		t.Fatal("invalid flags accepted")
	}
	dir := workflowProject(t, testWorkflow)
	r, complete := startWorker(t, dir, []string{"pull_request"}, func() {})
	_, err := finishWorker(t, r, complete)
	if err == nil || errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "no jobs match") {
		t.Fatalf("workflow error lost: %v", err)
	}
}

func TestWorkerStop(t *testing.T) {
	dockertest.Require(t)
	dir := workflowProject(t, strings.Replace(testWorkflow, `printf 'hello\n'`, `printf 'waiting\n'; exec sleep 60`, 1))
	waiting := make(chan struct{}, 1)
	var r *Runner
	started := make(chan struct{})
	r, complete := startWorker(t, dir, []string{"push", "-P", "ubuntu-latest=-self-hosted", "--no-cache-server", "--action-cache-path", t.TempDir()}, func() {
		<-started
		lines, _, _ := r.Drain()
		for _, line := range lines {
			if strings.Contains(line, `"raw_output":true`) && strings.Contains(line, "waiting") {
				select {
				case waiting <- struct{}{}:
				default:
				}
			}
		}
	})
	close(started)
	select {
	case <-waiting:
	case <-complete:
		t.Fatal("run finished before cancellation")
	case <-time.After(30 * time.Second):
		t.Fatal("workflow did not start")
	}
	r.Stop()
	r.Stop()
	if _, err := finishWorker(t, r, complete); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
}

func TestWorkersRunConcurrently(t *testing.T) {
	dockertest.Require(t)
	slow := strings.Replace(testWorkflow, `printf 'hello\n'`, `printf 'started\n'; sleep 2`, 1)
	args := func() []string {
		return []string{"push", "-P", "ubuntu-latest=-self-hosted", "--no-cache-server", "--action-cache-path", t.TempDir()}
	}
	first, firstDone := startWorker(t, workflowProject(t, slow), args(), func() {})
	second, secondDone := startWorker(t, workflowProject(t, slow), args(), func() {})
	for name, run := range map[string]struct {
		r    *Runner
		done <-chan struct{}
	}{"first": {first, firstDone}, "second": {second, secondDone}} {
		if _, err := finishWorker(t, run.r, run.done); err != nil {
			t.Fatalf("%s run: %v", name, err)
		}
	}
}
