package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// maxRecentProjects is how many recently opened projects are remembered.
const maxRecentProjects = 5

// recentFile returns where recent projects are persisted, or "" when the
// user config directory is unavailable.
func recentFile() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cfg, "acton", "recent.json")
}

// loadRecent reads the persisted project list, newest first. A missing or
// unreadable file yields an empty list.
func loadRecent(path string) []string {
	if path == "" {
		return nil
	}
	//nolint:gosec // G304: path is Acton's own config file.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var dirs []string
	if json.Unmarshal(data, &dirs) != nil {
		return nil
	}
	return dirs
}

// pushRecent moves dir to the front of list, dropping duplicates and
// anything past the limit.
func pushRecent(list []string, dir string) []string {
	out := []string{dir}
	for _, d := range list {
		if d != dir {
			out = append(out, d)
		}
	}
	return slices.Clip(out[:min(len(out), maxRecentProjects)])
}

// saveRecent atomically writes the project list.
func saveRecent(path string, dirs []string) error {
	if path == "" {
		return nil
	}
	data, err := json.Marshal(dirs)
	if err != nil {
		return fmt.Errorf("save recent projects: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("save recent projects: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".recent-*")
	if err != nil {
		return fmt.Errorf("save recent projects: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, writeErr := tmp.Write(data)
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return fmt.Errorf("save recent projects: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace recent projects: %w", err)
	}
	return nil
}

// rememberProject records dir as the most recently opened project. Failing
// to persist only loses the history, so it is not surfaced.
func (a *App) rememberProject(dir string) {
	if dir == "" {
		return
	}
	a.recent = pushRecent(a.recent, dir)
	_ = saveRecent(a.recentPath, a.recent)
}

// recentProjects lists remembered projects that still exist, newest first.
func (a *App) recentProjects() []string {
	var out []string
	for _, d := range a.recent {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			out = append(out, d)
		}
	}
	return out
}
