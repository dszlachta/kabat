package main

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/dszlachta/kabat/internal/report"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func showConnectDialog(app *application.App, svc *walletservice.WalletService, data any) {
	payload, ok := data.(map[string]any)
	if !ok || payload == nil {
		payload = map[string]any{}
	}

	dialog := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:         "Connection request",
		Width:         520,
		Height:        560,
		AlwaysOnTop:   true,
		DisableResize: true,
		Frameless:     true,
		URL:           "/connect.html",
	})

	var resolved atomic.Bool
	var offYes, offNo func()
	var cleanupOnce sync.Once

	cleanup := func() {
		cleanupOnce.Do(func() {
			if offYes != nil {
				offYes()
			}
			if offNo != nil {
				offNo()
			}
		})
	}

	finish := func(address string, approved bool) {
		if !resolved.CompareAndSwap(false, true) {
			return
		}
		if approved {
			reportConnect(svc.AllowConnect(address))
		} else {
			reportConnect(svc.CancelConnect())
		}
		cleanup()
		go dialog.Close()
	}

	offYes = app.Event.On("connect-allow", func(e *application.CustomEvent) {
		addr, _ := e.Data.(string)
		finish(addr, true)
	})
	offNo = app.Event.On("connect-deny", func(*application.CustomEvent) {
		finish("", false)
	})

	dialog.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		// Cancelled is a listener hook result, not a failure.
		_ = dialog.EmitEvent("pendingConnection", payload)
	})
	dialog.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		if resolved.CompareAndSwap(false, true) {
			reportConnect(svc.CancelConnect())
		}
		cleanup()
	})

	dialog.Show()
	dialog.Focus()
}

func reportConnect(err error) {
	if err == nil || errors.Is(err, walletservice.ErrNoPendingConnection) {
		// No pending connection: the other path already resolved it.
		return
	}
	report.Error(err)
}
