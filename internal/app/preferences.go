package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type preferences struct {
	SaveRuns      bool `json:"save_runs"`
	Notifications bool `json:"notifications"`
}

func preferencesFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "acton", "preferences.json")
}

func loadPreferences(path string) (preferences, error) {
	var p preferences
	if path == "" {
		return p, errors.New("user config directory unavailable")
	}
	//nolint:gosec // G304: path is Acton's own preferences file.
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return preferences{}, fmt.Errorf("read preferences: %w", err)
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return preferences{}, fmt.Errorf("decode preferences: %w", err)
	}
	return p, nil
}

func savePreferences(path string, p preferences) error {
	if path == "" {
		return errors.New("user config directory unavailable")
	}
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".preferences-*")
	if err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_, werr := f.Write(b)
	if err = errors.Join(werr, f.Close()); err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("replace preferences: %w", err)
	}
	return nil
}

func (a *App) persistPreferences() {
	if err := savePreferences(a.preferencesPath, a.preferences); err != nil {
		a.preferenceErr = "Cannot save preferences: " + err.Error()
	} else {
		a.preferenceErr = ""
	}
}

func (a *App) runPreferencesView(c *ui.Context) {
	configSection(c, "Run preferences", "History, notifications", lucide.Settings(), &a.preferencesOpen, func() {
		previous := a.preferences.SaveRuns
		components.Switch(c.Key("save-runs"), &a.preferences.SaveRuns, "Save run output as JSONL", components.SwitchOptions{Compact: true})
		if previous != a.preferences.SaveRuns {
			a.persistPreferences()
		}
		previous = a.preferences.Notifications
		components.Switch(c.Key("notifications"), &a.preferences.Notifications, "Notify when runs finish", components.SwitchOptions{Compact: true})
		if previous != a.preferences.Notifications {
			if a.preferences.Notifications {
				a.preferences.Notifications = false
				a.enableNotifications()
			} else {
				a.persistPreferences()
			}
		}
		if a.notificationPending {
			components.Text(c, "Waiting for notification permission…").TextColor(c.Theme().TextMuted)
		}
	})
	if a.preferenceErr != "" {
		components.Text(c, a.preferenceErr).Padding(c.Theme().Space(3)).TextColor(c.Theme().Danger)
	}
}
