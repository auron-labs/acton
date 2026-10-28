// Acton wraps nektos/act in a desktop window: project picker and settings
// on the left, GitHub-Actions-style job output on the right.
//
// Usage: acton [--run] [project-dir]
package main

import (
	"acton/internal/app"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dir, runNow := "", false
	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--run" || arg == "-run":
			runNow = true
		case strings.HasPrefix(arg, "-"):
			//nolint:gosec // G706: %q escapes control characters in the flag.
			log.Printf("acton: ignoring unknown flag %q", arg)
		default:
			if abs, err := filepath.Abs(arg); err == nil {
				dir = abs
			}
		}
	}
	if err := app.Run(dir, runNow); err != nil {
		log.Fatal(err)
	}
}
