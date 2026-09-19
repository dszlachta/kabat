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

func showConfirmDialog(app *application.App, svc *walletservice.WalletService, data any) {
	tx, ok := data.(map[string]any)
	if !ok || tx == nil {
		tx = map[string]any{}
	}

	dialog := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:         "Confirm transaction",
		Width:         520,
		Height:        480,
		AlwaysOnTop:   true,
		DisableResize: true,
		Frameless:     true,
		URL:           "/confirm.html",
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

	finish := func(approved bool) {
		if !resolved.CompareAndSwap(false, true) {
			return
		}
		if approved {
			reportPending(svc.ConfirmPending())
		} else {
			reportPending(svc.CancelPending())
		}
		cleanup()
		go dialog.Close()
	}

	offYes = app.Event.On("confirm-yes", func(*application.CustomEvent) {
		finish(true)
	})
	offNo = app.Event.On("confirm-no", func(*application.CustomEvent) {
		finish(false)
	})

	dialog.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		// Cancelled is a listener hook result, not a failure.
		_ = dialog.EmitEvent("pendingTransaction", tx)
	})
	dialog.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		if resolved.CompareAndSwap(false, true) {
			reportPending(svc.CancelPending())
		}
		cleanup()
	})

	dialog.Show()
	dialog.Focus()
}

func reportPending(err error) {
	if err == nil || errors.Is(err, walletservice.ErrNoPendingTransaction) {
		// No pending tx: the other path already resolved it.
		return
	}
	report.Error(err)
}
