// Acton wraps nektos/act in a desktop window: project picker and settings
// on the left, GitHub-Actions-style job output on the right.
//
// Usage: acton [--run] [--mcp[=127.0.0.1:7337]] [project-dir]
//
// Each run executes in a worker process: the same executable started with
// act.WorkerArg.
package main

import (
	"acton/internal/act"
	"acton/internal/app"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	act.MaybeRunWorker()
	dir, runNow := "", false
	mcpAddr := ""
	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--run" || arg == "-run":
			runNow = true
		case arg == "--mcp":
			mcpAddr = "127.0.0.1:7337"
		case strings.HasPrefix(arg, "--mcp="):
			mcpAddr = strings.TrimPrefix(arg, "--mcp=")
		case strings.HasPrefix(arg, "-"):
			//nolint:gosec // G706: %q escapes control characters in the flag.
			log.Printf("acton: ignoring unknown flag %q", arg)
		default:
			if abs, err := filepath.Abs(arg); err == nil {
				dir = abs
			}
		}
	}
	if err := app.Run(dir, runNow, mcpAddr); err != nil {
		log.Fatal(err)
	}
}
