package main

import (
	"log"
	"sync"
	"time"

	"github.com/dszlachta/kabat/internal/config"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	minWindowWidth     = 800
	minWindowHeight    = 600
	windowSizeSaveWait = 400 * time.Millisecond
)

func defaultWindowSize(workW, workH int) (w, h int) {
	w = max(minWindowWidth, workW*4/9)
	h = max(minWindowHeight, workH/2)
	if workW > 0 && w > workW {
		w = workW
	}
	if workH > 0 && h > workH {
		h = workH
	}
	return w, h
}

func minWindowSize(workW, workH int) (w, h int) {
	w, h = minWindowWidth, minWindowHeight
	if workW > 0 && workW < w {
		w = workW
	}
	if workH > 0 && workH < h {
		h = workH
	}
	return w, h
}

func restoreWindowSize(workW, workH, savedW, savedH int) (w, h int) {
	w, h = defaultWindowSize(workW, workH)
	if savedW > 0 && savedH > 0 {
		w, h = savedW, savedH
	}
	minW, minH := minWindowSize(workW, workH)
	if w < minW {
		w = minW
	}
	if h < minH {
		h = minH
	}
	if workW > 0 && w > workW {
		w = workW
	}
	if workH > 0 && h > workH {
		h = workH
	}
	return w, h
}

func persistWindowSize(path string, w, h int) error {
	if path == "" || w <= 0 || h <= 0 {
		return nil
	}
	return config.Update(path, func(f *config.File) {
		f.Window = &config.Window{Width: w, Height: h}
	})
}

func loadSavedWindowSize(path string) (w, h int) {
	if path == "" {
		return 0, 0
	}
	f, err := config.Load(path)
	if err != nil || f.Window == nil {
		return 0, 0
	}
	return f.Window.Width, f.Window.Height
}

type windowSizeStore struct {
	path  string
	mu    sync.Mutex
	timer *time.Timer
}

func (s *windowSizeStore) apply(app *application.App, win *application.WebviewWindow) {
	savedW, savedH := loadSavedWindowSize(s.path)
	w, h := minWindowWidth, minWindowHeight
	minW, minH := minWindowWidth, minWindowHeight
	if screen := app.Screen.GetPrimary(); screen != nil {
		area := screen.WorkArea
		w, h = restoreWindowSize(area.Width, area.Height, savedW, savedH)
		minW, minH = minWindowSize(area.Width, area.Height)
	} else {
		w, h = restoreWindowSize(0, 0, savedW, savedH)
	}
	win.SetMinSize(minW, minH)
	win.SetSize(w, h)
	win.Center()
	win.Show()
}

func (s *windowSizeStore) schedule(win *application.WebviewWindow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(windowSizeSaveWait, func() {
		s.flush(win)
	})
}

func (s *windowSizeStore) flush(win *application.WebviewWindow) {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	if win == nil || win.IsMinimised() {
		return
	}
	w, h := win.Size()
	if err := persistWindowSize(s.path, w, h); err != nil {
		log.Print(err)
	}
}
