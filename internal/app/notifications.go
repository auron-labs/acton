package app

import (
	"acton/internal/act"
	"errors"
	"fmt"

	"github.com/egoist/mygo"
)

func nativeNotification(title, body string) error {
	if !mygo.NotificationsSupported() {
		return errors.New("notifications are not supported on this platform")
	}
	if err := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body}).Show(); err != nil {
		return fmt.Errorf("show notification: %w", err)
	}
	return nil
}

func (a *App) enableNotifications() {
	if a.notificationPending {
		return
	}
	if a.win == nil {
		a.preferenceErr = "Notifications require a desktop window"
		return
	}
	a.notificationPending = true
	go func() {
		err := nativeNotification("Acton notifications enabled", "You will be notified when a run finishes.")
		a.win.Update(func() {
			a.notificationPending = false
			if err != nil {
				a.preferenceErr = "Cannot enable notifications: " + err.Error()
				return
			}
			a.preferences.Notifications = true
			a.persistPreferences()
		})
	}()
}

func (a *App) notifyRun(st act.Status) {
	if a.shuttingDown {
		return
	}
	if a.notifyTest != nil {
		a.notifyTest(st)
		return
	}
	if !a.preferences.Notifications || a.win == nil {
		return
	}
	go func() {
		err := nativeNotification("Acton run "+st.String(), "Workflow run "+st.String()+".")
		if err != nil {
			a.win.Update(func() { a.preferenceErr = "Cannot deliver notification: " + err.Error() })
		}
	}()
}
