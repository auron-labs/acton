package act

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	MaybeRunWorker()
	os.Exit(m.Run())
}
