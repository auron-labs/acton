package act

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func workflowProject(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "ci.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

const testWorkflow = `name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: check
        run: printf 'hello\n'
`

func startRunner(t *testing.T, dir string, args []string, flush func()) (*Runner, <-chan struct{}) {
	t.Helper()
	r := &Runner{}
	complete := make(chan struct{})
	if err := r.Start(dir, args, flush, func() { close(complete) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Stop(); r.Wait() })
	return r, complete
}

func finishRunner(t *testing.T, r *Runner, complete <-chan struct{}) ([]string, error) {
	t.Helper()
	select {
	case <-complete:
	case <-time.After(30 * time.Second):
		t.Fatal("embedded run did not finish")
	}
	lines, done, err := r.Drain()
	if !done {
		t.Fatal("completion callback preceded done status")
	}
	return lines, err
}

func TestRunnerDoneWaitsForOutput(t *testing.T) {
	dir := workflowProject(t, testWorkflow)
	previousHost, hostWasSet := os.LookupEnv("DOCKER_HOST")
	t.Setenv("PATH", t.TempDir()) // No act executable, shell, or gh is available.
	blocked, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	var once sync.Once
	r, complete := startRunner(t, dir, []string{"push", "-n", "-P", "ubuntu-latest=unused", "--no-cache-server"}, func() {
		once.Do(func() { close(blocked); <-release })
	})
	defer unblock()
	select {
	case <-blocked:
	case <-time.After(15 * time.Second):
		t.Fatal("no workflow output")
	}
	select {
	case <-complete:
		t.Fatal("onDone ran before logging finished")
	default:
	}
	_, done, err := r.Drain()
	if done || err != nil {
		t.Fatalf("premature completion: done=%v err=%v", done, err)
	}
	var second Runner
	if err := second.Start(dir, nil, func() {}, func() {}); err == nil {
		t.Fatal("accepted concurrent embedded execution")
	}
	unblock()
	lines, err := finishRunner(t, r, complete)
	if err != nil {
		t.Fatal(err)
	}
	if host, set := os.LookupEnv("DOCKER_HOST"); host != previousHost || set != hostWasSet {
		t.Fatal("Docker environment was not restored")
	}
	var run Run
	for _, line := range lines {
		run.Feed(line)
	}
	if len(run.Jobs) != 1 || run.Jobs[0].Status != StatusSuccess {
		t.Fatalf("unexpected jobs: %+v\n%s", run.Jobs, strings.Join(lines, "\n"))
	}
}

// shortcut: act v0.2.89 has host PTY races/output loss; upgrade when upstream fixes them.
func TestRunnerWorkflow(t *testing.T) {
	platforms := []string{"-self-hosted"}
	if image := os.Getenv("ACTON_TEST_IMAGE"); image != "" {
		platforms = append(platforms, image)
	}
	for _, platform := range platforms {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%v", platform, fail), func(t *testing.T) {
				script := `printf '%s\n' '${{ secrets.SAMPLE }}' '::add-mask::dynamic-value' 'dynamic-value' 'hello'`

				body := strings.Replace(testWorkflow, `printf 'hello\n'`, script, 1)
				if fail {
					body += "      - name: fail\n        run: exit 3\n"
				}
				dir := workflowProject(t, body)
				if err := os.WriteFile(filepath.Join(dir, ".secrets"), []byte("SAMPLE=private-value\nGITHUB_TOKEN=\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				args := []string{"push", "-P", "ubuntu-latest=" + platform, "--pull=false", "--action-cache-path", filepath.Join(t.TempDir(), "actions"), "--no-cache-server"}
				r, complete := startRunner(t, dir, args, func() {})
				lines, err := finishRunner(t, r, complete)
				if (err != nil) != fail {
					t.Fatalf("failure=%v, err=%v\n%s", fail, err, strings.Join(lines, "\n"))
				}
				output := strings.Join(lines, "\n")
				if strings.Contains(output, "private-value") || strings.Contains(output, "dynamic-value") {
					t.Fatalf("fixture secrets or add-mask values leaked into logs:\n%s", output)
				}
				var run Run
				for _, line := range lines {
					run.Feed(line)
				}
				want := StatusSuccess
				if fail {
					want = StatusFailure
				}
				if len(run.Jobs) != 1 || run.Jobs[0].Status != want || !strings.Contains(output, "hello") || !strings.Contains(output, "***") {
					t.Fatalf("unexpected workflow result: %+v\n%s", run.Jobs, output)
				}
			})
		}
	}
}

func TestRunnerStop(t *testing.T) {
	dir := workflowProject(t, strings.Replace(testWorkflow, `printf 'hello\n'`, `printf 'waiting\n'; exec sleep 60`, 1))
	waiting := make(chan struct{})
	var once sync.Once
	var r *Runner
	// The log callback runs only after Start has returned and assigned r.
	started := make(chan struct{})
	r, complete := startRunner(t, dir, []string{"push", "-P", "ubuntu-latest=-self-hosted", "--no-cache-server", "--action-cache-path", t.TempDir()}, func() {
		<-started
		lines, _, _ := r.Drain()
		for _, line := range lines {
			if strings.Contains(line, `"raw_output":true`) && strings.Contains(line, "waiting") {
				once.Do(func() { close(waiting) })
			}
		}
	})
	close(started)
	select {
	case <-waiting:
	case <-complete:
		t.Fatal("run finished before cancellation")
	case <-time.After(15 * time.Second):
		t.Fatal("workflow did not start")
	}
	r.Stop()
	r.Stop()
	_, err := finishRunner(t, r, complete)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
}

func TestRunnerErrors(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		args       []string
	}{
		{"invalid YAML", "on: [", nil},
		{"unmatched event", testWorkflow, []string{"pull_request"}},
		{"unmatched job", testWorkflow, []string{"push", "-j", "missing"}},
		{"missing event file", testWorkflow, []string{"push", "-e", "missing.json"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := workflowProject(t, tt.body)
			r, complete := startRunner(t, dir, tt.args, func() {})
			_, err := finishRunner(t, r, complete)
			if err == nil {
				t.Fatal("expected workflow error")
			}
		})
	}
}

func TestRunnerLargeOutput(t *testing.T) {
	r := &Runner{onFlush: func() {}, level: logrus.InfoLevel}
	logger := r.WithJobLogger()
	message := strings.Repeat("x", 4<<20)
	logger.Info(message)
	lines, done, err := r.Drain()
	if done || err != nil || len(lines) != 1 || !strings.Contains(lines[0], message) {
		t.Fatal("large log record was lost")
	}
}

func TestRunnerArtifactServerFailure(t *testing.T) {
	var lc net.ListenConfig
	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	dir := workflowProject(t, strings.Replace(testWorkflow, `printf 'hello\n'`, `exec sleep 60`, 1))
	r, complete := startRunner(t, dir, []string{"push", "-P", "ubuntu-latest=-self-hosted", "--no-cache-server", "--action-cache-path", t.TempDir(), "--artifact-server-path", t.TempDir(), "--artifact-server-addr", "127.0.0.1", "--artifact-server-port", port}, func() {})
	_, err = finishRunner(t, r, complete)
	if err == nil || errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "server failed") {
		t.Fatalf("server failure misreported: %v", err)
	}
}
