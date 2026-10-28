package act

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// actBin is the act executable; tests swap it for a stub.
var actBin = "act"

// Runner runs one act process, buffers its combined output, and lets the
// UI drain it on the main thread. The process streams both stdout (the
// [job] progress log) and stderr (act's own log lines) through one pipe so
// ordering is preserved.
type Runner struct {
	mu      sync.Mutex
	pending []string

	cmd   *exec.Cmd
	timer *time.Timer

	done    bool
	exitErr error
}

// Start launches act in dir with args. onFlush is called from a worker each
// time output is buffered (at most ~15/s); it must schedule a UI update.
// onDone is called from a worker after the process exits and all output has
// been buffered; it must also schedule a UI update.
func (r *Runner) Start(dir string, args []string, onFlush, onDone func()) error {
	ctx, cancel := context.WithCancel(context.Background())
	//nolint:gosec // G204: launching act with the configured args is the app.
	cmd := exec.CommandContext(ctx, actBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLICOLOR=0", "CLICOLOR_FORCE=0")
	// Kill the whole process group on cancel so docker exec children die too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGINT)
	}
	// If a child keeps our pipe open after act exits, don't wait forever.
	cmd.WaitDelay = 5 * time.Second

	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		return fmt.Errorf("pipe: %w", err)
	}
	cmd.Stdout, cmd.Stderr = pw, pw

	if err := cmd.Start(); err != nil {
		cancel()
		_ = pw.Close()
		_ = pr.Close()
		return fmt.Errorf("start %s: %w", actBin, err)
	}
	r.cmd = cmd

	// Read lines into pending; close the write end when the process exits.
	readDone := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
		for sc.Scan() {
			r.mu.Lock()
			r.pending = append(r.pending, sc.Text())
			r.mu.Unlock()
			onFlush()
		}
		_ = pr.Close()
		readDone <- sc.Err()
	}()

	go func() {
		err := cmd.Wait()
		_ = pw.Close()
		cancel()
		err = errors.Join(err, <-readDone)
		r.mu.Lock()
		r.done = true
		r.exitErr = err
		r.mu.Unlock()
		onDone()
	}()
	return nil
}

// Stop interrupts the process group, then kills it after a grace period.
func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done || r.cmd == nil || r.cmd.Process == nil {
		return
	}
	pgid := r.cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGINT)
	r.timer = time.AfterFunc(4*time.Second, func() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	})
}

// Drain returns buffered lines (and clears them) plus done/err status.
func (r *Runner) Drain() (lines []string, done bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lines = r.pending
	r.pending = nil
	return lines, r.done, r.exitErr
}
