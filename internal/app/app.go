// Package app is the Acton application: the window, its project tabs, and the
// act runs they launch.
package app

import (
	"acton/internal/act"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/egoist/mygo"
	"github.com/oneo-me/onevra/source/shells/plain"
)

// App is the window: the open project sessions plus the state they share.
type App struct {
	win *mygo.Window

	sessions []*Session
	current  *Session // the selected tab; nil shows Home

	done      chan struct{}
	mcpServer *http.Server

	history   []runSnapshot // runs cleared from their session
	nextRunID int

	preferences         preferences
	preferencesPath     string
	preferencesOpen     bool
	preferenceErr       string
	notificationPending bool
	shuttingDown        bool
	notifyTest          func(*Session, act.Status)

	recent     []string // recently opened project dirs, newest first
	recentPath string   // "" disables persistence
}

func newApp() *App {
	return &App{done: make(chan struct{})}
}

// Run starts the desktop app: it builds the window's shell once mygo is
// ready, wires the lifecycle, and blocks in the event loop until done. dir,
// when set, opens that project; runNow starts act immediately.
func Run(dir string, runNow bool, mcpAddr string) error {
	if err := registerFonts(); err != nil {
		return err
	}
	a := newApp()
	a.preferencesPath = preferencesFile()
	var prefErr error
	a.preferences, prefErr = loadPreferences(a.preferencesPath)
	if prefErr != nil {
		a.preferenceErr = "Cannot load preferences: " + prefErr.Error()
	}
	a.recentPath = recentFile()
	a.recent = loadRecent(a.recentPath)
	defer a.shutdown()
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	go func() {
		select {
		case <-ctx.Done():
			mygo.App.Quit()
		case <-a.done:
		}
	}()
	if dir != "" {
		a.openProject(dir)
	}
	var startupErr error
	mygo.App.WhenReady(func() {
		shell := plain.NewShell(plain.ShellOptions{
			Title:               "Acton",
			Width:               1120,
			Height:              720,
			MinWidth:            720,
			MinHeight:           480,
			StateKey:            "main",
			Hidden:              true,
			Theme:               designTheme,
			HeaderLeading:       a.headerLeading,
			HeaderActions:       a.headerActions,
			Content:             a.view,
			ContentBehindHeader: true,
		})
		a.win = shell.Window()
		a.win.OnReadyToShow(a.win.Show)
		a.win.OnClosed(func() {
			for _, s := range a.sessions {
				s.stopRun()
			}
			mygo.App.Quit()
		})
		if mcpAddr != "" {
			if err := a.serveMCP(mcpAddr); err != nil {
				startupErr = err
				mygo.App.Quit()
				return
			}
		}
		if runNow && a.current != nil {
			a.current.startRun()
		}
	})
	if err := mygo.App.Run(); err != nil {
		return fmt.Errorf("mygo run: %w", err)
	}
	return startupErr
}

// canonicalDir resolves symlinks so one project is never opened twice.
func canonicalDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// sessionFor returns the open session for a project directory.
func (a *App) sessionFor(dir string) *Session {
	want := canonicalDir(dir)
	for _, s := range a.sessions {
		if canonicalDir(s.settings.Dir) == want {
			return s
		}
	}
	return nil
}

// addSession opens a tab for the project without selecting it. Each project
// has one tab because its runs share Docker resources.
func (a *App) addSession(dir string) *Session {
	s := a.sessionFor(dir)
	if s == nil {
		s = newSession(a)
		s.setDir(dir)
		a.sessions = append(a.sessions, s)
	}
	return s
}

// openProject selects the project's tab, opening one when needed.
func (a *App) openProject(dir string) *Session {
	a.current = a.addSession(dir)
	return a.current
}

// closeSession removes an idle project tab.
func (a *App) closeSession(s *Session) {
	if s.running {
		return
	}
	s.clearRun()
	i := slices.Index(a.sessions, s)
	a.sessions = slices.Delete(a.sessions, i, i+1)
	if a.current == s {
		a.current = nil
		if len(a.sessions) > 0 {
			a.current = a.sessions[min(i, len(a.sessions)-1)]
		}
	}
}

func (a *App) runningCount() int {
	n := 0
	for _, s := range a.sessions {
		if s.running {
			n++
		}
	}
	return n
}

// pickFolder opens the native directory picker off the UI thread, then
// opens the chosen project on it.
func (a *App) pickFolder() {
	if a.win == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    a.win,
			Title:     "Choose a project folder",
			Directory: true,
		})
		a.win.Update(func() {
			if err != nil || len(paths) == 0 {
				return
			}
			a.openProject(paths[0])
		})
	}()
}

// shutdown waits for every run's post steps and resource cleanup before returning to main.
func (a *App) shutdown() {
	a.shuttingDown = true
	close(a.done)
	if a.mcpServer != nil {
		_ = a.mcpServer.Close()
	}
	for _, s := range a.sessions {
		if s.runner != nil {
			s.runner.Stop()
		}
	}
	for _, s := range a.sessions {
		if s.runner != nil {
			s.runner.Wait()
			s.pump()
		}
		s.closeRunFiles()
	}
}
