package act

import (
	"bufio"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRunnerDoneWaitsForOutput(t *testing.T) {
	old := actBin
	actBin = "sh"
	defer func() { actBin = old }()

	const pairs = 4000
	const records = 2 * pairs
	readerBlocked := make(chan struct{})
	releaseReader := make(chan struct{})
	release := sync.OnceFunc(func() { close(releaseReader) })
	defer release()
	completed := make(chan struct{})
	var r Runner
	flushed := 0
	script := `i=0
while [ "$i" -lt 4000 ]; do
    printf '{"index":%s,"stream":"stdout"}\n' "$i"
    printf '{"index":%s,"stream":"stderr"}\n' "$i" >&2
    i=$((i + 1))
done`
	if err := r.Start(t.TempDir(), []string{"-c", script}, func() {
		flushed++
		if flushed == records-1 {
			close(readerBlocked)
			<-releaseReader
		}
	}, func() { close(completed) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Stop)

	select {
	case <-readerBlocked:
	case <-time.After(10 * time.Second):
		t.Fatal("reader did not reach the final output")
	}
	// The shell can exit while the reader is paused with one record left.
	select {
	case <-completed:
		t.Fatal("onDone called before the reader finished")
	case <-time.After(100 * time.Millisecond):
	}
	lines, done, err := r.Drain()
	if done || err != nil {
		t.Fatalf("Drain before reader finished: done=%v err=%v", done, err)
	}
	release()

	deadline := time.Now().Add(10 * time.Second)
	for {
		batch, done, err := r.Drain()
		lines = append(lines, batch...)
		if done {
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if len(lines) != records {
		t.Fatalf("Drain reported done with %d records, want %d", len(lines), records)
	}
	for i, line := range lines {
		var record struct {
			Index  int    `json:"index"`
			Stream string `json:"stream"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		wantStream := "stdout"
		if i%2 != 0 {
			wantStream = "stderr"
		}
		if record.Index != i/2 || record.Stream != wantStream {
			t.Fatalf("record %d: got %+v, want index=%d stream=%s", i, record, i/2, wantStream)
		}
	}
	select {
	case <-completed:
	case <-time.After(10 * time.Second):
		t.Fatal("onDone was not called")
	}
}

func TestRunnerScannerError(t *testing.T) {
	old := actBin
	actBin = "sh"
	defer func() { actBin = old }()

	completed := make(chan struct{})
	var r Runner
	if err := r.Start(t.TempDir(), []string{"-c", "printf '%04194304d\\n' 0"}, func() {}, func() {
		close(completed)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Stop)
	select {
	case <-completed:
	case <-time.After(10 * time.Second):
		t.Fatal("run did not finish after scanner error")
	}
	_, done, err := r.Drain()
	if !done || !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("Drain: done=%v err=%v, want scanner token-too-long error", done, err)
	}
}

// TestRunnerStop starts a stand-in process and checks Stop interrupts the
// whole process group and reports done.
func TestRunnerStop(t *testing.T) {
	old := actBin
	actBin = "sleep"
	defer func() { actBin = old }()

	var r Runner
	if err := r.Start(t.TempDir(), []string{"60"}, func() {}, func() {}); err != nil {
		t.Fatal(err)
	}
	r.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, done, err := r.Drain()
		if done {
			if err == nil {
				t.Fatal("stopped run should report a nonzero exit")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("run did not stop")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
