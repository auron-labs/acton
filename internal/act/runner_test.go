package act

import (
	"testing"
	"time"
)

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
