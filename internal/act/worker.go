package act

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

// WorkerArg switches the Acton executable into its headless worker mode, which
// executes one act plan. act mutates process-wide state (environment, logger),
// so every run gets its own process and runs for different projects can overlap.
const WorkerArg = "--act-worker"

// exitPrefix starts the worker's final stdout record. Log records are written
// by logrus with "level" or "msg" keys first, so they never match.
const exitPrefix = `{"acton_exit":`

type workerRequest struct {
	Dir  string   `json:"dir"`
	Args []string `json:"args"`
}

type workerExit struct {
	Error     string `json:"error,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// MaybeRunWorker runs the worker and exits when the process was started with
// WorkerArg. Call it first in main and in TestMain of packages that start runs.
func MaybeRunWorker() {
	if len(os.Args) == 2 && os.Args[1] == WorkerArg {
		os.Exit(runWorker(os.Stdin, os.Stdout))
	}
}

// runWorker reads one request from in and streams log records to out. The run
// is cancelled when in closes, which also covers the parent dying.
func runWorker(in io.Reader, out *os.File) int {
	// Interrupts reach the whole terminal process group; the parent owns shutdown.
	signal.Ignore(os.Interrupt, syscall.SIGTERM)
	// Anything else written to stdout must not corrupt the record stream.
	os.Stdout = os.Stderr
	decoder := json.NewDecoder(in)
	var request workerRequest
	if err := decoder.Decode(&request); err != nil {
		fmt.Fprintln(os.Stderr, "act worker: read request:", err)
		return 1
	}
	var (
		writeMu sync.Mutex
		e       engine
	)
	complete := make(chan struct{})
	writer := bufio.NewWriter(out)
	flush := func() {
		writeMu.Lock()
		defer writeMu.Unlock()
		lines, _, _ := e.Drain()
		for _, line := range lines {
			_, _ = writer.WriteString(line + "\n")
		}
		_ = writer.Flush()
	}
	if err := e.Start(request.Dir, request.Args, flush, func() { close(complete) }); err != nil {
		writeExit(writer, err)
		return 1
	}
	go func() {
		_, _ = io.Copy(io.Discard, io.MultiReader(decoder.Buffered(), in))
		e.Stop()
	}()
	<-complete
	flush()
	_, _, err := e.Drain()
	writeMu.Lock()
	writeExit(writer, err)
	writeMu.Unlock()
	if err != nil {
		return 1
	}
	return 0
}

func writeExit(w *bufio.Writer, err error) {
	var result workerExit
	if err != nil {
		result = workerExit{Error: err.Error(), Cancelled: errors.Is(err, context.Canceled)}
	}
	data, _ := json.Marshal(result)
	_, _ = w.WriteString(exitPrefix + string(data) + "}\n")
	_ = w.Flush()
}

// Runner executes one act plan in a worker process and buffers its JSON logs
// for the UI.
type Runner struct {
	mu       sync.Mutex
	pending  []string
	stdin    io.Closer
	stopped  bool
	finished chan struct{}
	done     bool
	exitErr  error
}

// Start validates flags, then starts the worker. Callbacks run on worker
// goroutines and must schedule UI updates; onDone runs after all output was
// delivered to onFlush.
func (r *Runner) Start(dir string, args []string, onFlush, onDone func()) error {
	if _, err := parseOptions(dir, args); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished != nil {
		return errors.New("runner has already been started")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate Acton executable: %w", err)
	}
	//nolint:gosec // G204: the worker is this executable, with a fixed argument.
	cmd := exec.CommandContext(context.Background(), exe, WorkerArg)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("start act worker: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("start act worker: %w", err)
	}
	var stderr tailBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start act worker: %w", err)
	}
	// Settings can include secrets, so they travel over the private pipe.
	if err := json.NewEncoder(stdin).Encode(workerRequest{Dir: dir, Args: args}); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return fmt.Errorf("start act worker: %w", err)
	}
	r.stdin = stdin
	r.finished = make(chan struct{})
	go func() {
		exit := r.read(stdout, onFlush)
		waitErr := cmd.Wait()
		r.mu.Lock()
		switch {
		case exit == nil:
			msg := "act worker stopped unexpectedly"
			if waitErr != nil {
				msg += ": " + waitErr.Error()
			}
			if tail := strings.TrimSpace(stderr.String()); tail != "" {
				msg += ": " + tail
			}
			r.exitErr = errors.New(msg)
		case exit.Cancelled:
			r.exitErr = context.Canceled
		case exit.Error != "":
			r.exitErr = errors.New(exit.Error)
		}
		r.done = true
		close(r.finished)
		r.mu.Unlock()
		onDone()
	}()
	return nil
}

// read buffers the worker's log records and returns its final exit record,
// or nil when the stream ended without one.
func (r *Runner) read(stdout io.Reader, onFlush func()) *workerExit {
	// Records have no size limit, so avoid bufio.Scanner.
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if line = strings.TrimSuffix(line, "\n"); strings.HasPrefix(line, exitPrefix) {
			var exit struct {
				Exit workerExit `json:"acton_exit"`
			}
			if json.Unmarshal([]byte(line), &exit) == nil {
				return &exit.Exit
			}
		} else if line != "" {
			r.mu.Lock()
			r.pending = append(r.pending, line)
			r.mu.Unlock()
			onFlush()
		}
		if err != nil {
			return nil
		}
	}
}

// Stop asks the worker to cancel jobs, which allows post steps and container
// cleanup, and to force cancellation after a grace period.
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done || r.stdin == nil || r.stopped {
		return
	}
	r.stopped = true
	_ = r.stdin.Close()
}

// Wait waits for execution and cleanup to finish before the app exits.
func (r *Runner) Wait() {
	r.mu.Lock()
	finished := r.finished
	r.mu.Unlock()
	if finished != nil {
		<-finished
	}
}

// Drain returns buffered records and the execution status atomically.
func (r *Runner) Drain() (lines []string, done bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lines, r.pending = r.pending, nil
	return lines, r.done, r.exitErr
}

// tailBuffer keeps the end of a worker's stderr for crash reports.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if extra := len(b.buf) - 4096; extra > 0 {
		b.buf = bytes.Clone(b.buf[extra:])
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
