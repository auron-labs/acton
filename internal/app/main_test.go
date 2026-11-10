package app

import (
	"acton/internal/act"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	act.MaybeRunWorker()
	os.Exit(m.Run())
}
