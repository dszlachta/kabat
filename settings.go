package main

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	settingsWindowWidth  = 480
	settingsWindowHeight = 320
)

type settingsHost struct {
	mu  sync.Mutex
	win *application.WebviewWindow
}

func (h *settingsHost) show(app *application.App) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.win != nil {
		h.win.Show()
		h.win.Focus()
		return
	}

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "settings",
		Title:           "Settings",
		Width:           settingsWindowWidth,
		Height:          settingsWindowHeight,
		MinWidth:        settingsWindowWidth,
		MinHeight:       settingsWindowHeight,
		DisableResize:   true,
		InitialPosition: application.WindowCentered,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		URL: "/settings.html",
	})
	h.win = win
	win.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		h.mu.Lock()
		h.win = nil
		h.mu.Unlock()
	})
	win.Show()
	win.Focus()
}
