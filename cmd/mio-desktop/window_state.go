package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	defaultWindowWidth  = 980
	defaultWindowHeight = 1010
	minWindowWidth      = 640
	minWindowHeight     = 480
)

type windowState struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func windowStatePath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "window-state.json")
}

func (s windowState) usable() bool {
	return s.Width >= minWindowWidth && s.Height >= minWindowHeight
}

func loadWindowState(path string) (windowState, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return windowState{}, false
	}
	var state windowState
	if err := json.Unmarshal(data, &state); err != nil || !state.usable() {
		return windowState{}, false
	}
	return state, true
}

func saveWindowState(path string, state windowState) error {
	if !state.usable() {
		return fmt.Errorf("invalid window state %dx%d", state.Width, state.Height)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func restoreWindowState(saved windowState, savedOK bool, screens []*application.Screen) windowState {
	if savedOK && saved.usable() {
		if screen := bestScreen(saved, screens); screen != nil {
			return fitToWorkArea(saved, workArea(screen))
		}
	}
	return defaultWindowState(screens)
}

func defaultWindowState(screens []*application.Screen) windowState {
	state := windowState{Width: defaultWindowWidth, Height: defaultWindowHeight}
	screen := primaryScreen(screens)
	if screen == nil {
		return state
	}
	work := workArea(screen)
	state.Width = min(state.Width, work.Width)
	state.Height = min(state.Height, work.Height)
	state.X = work.X + (work.Width-state.Width)/2
	state.Y = work.Y + (work.Height-state.Height)/2
	return fitToWorkArea(state, work)
}

func bestScreen(state windowState, screens []*application.Screen) *application.Screen {
	window := application.Rect{X: state.X, Y: state.Y, Width: state.Width, Height: state.Height}
	var best *application.Screen
	bestArea := 0
	for _, screen := range screens {
		work := workArea(screen)
		if work.IsEmpty() {
			continue
		}
		intersection := window.Intersect(work)
		area := intersection.Width * intersection.Height
		if area > bestArea {
			best = screen
			bestArea = area
		}
	}
	return best
}

func primaryScreen(screens []*application.Screen) *application.Screen {
	for _, screen := range screens {
		if screen.IsPrimary && !workArea(screen).IsEmpty() {
			return screen
		}
	}
	for _, screen := range screens {
		if !workArea(screen).IsEmpty() {
			return screen
		}
	}
	return nil
}

func workArea(screen *application.Screen) application.Rect {
	if !screen.WorkArea.IsEmpty() {
		return screen.WorkArea
	}
	return screen.Bounds
}

func fitToWorkArea(state windowState, work application.Rect) windowState {
	state.Width = min(state.Width, work.Width)
	state.Height = min(state.Height, work.Height)
	state.X = min(max(state.X, work.X), work.X+work.Width-state.Width)
	state.Y = min(max(state.Y, work.Y), work.Y+work.Height-state.Height)
	return state
}

type windowStateSaver struct {
	path   string
	window *application.WebviewWindow

	mu    sync.Mutex
	timer *time.Timer
	state windowState
	ready bool
}

func restoreAndTrackWindowState(app *application.App, window *application.WebviewWindow, path string) {
	saver := &windowStateSaver{path: path, window: window}
	lifecycle := &windowLifecycle{window: window, flush: saver.flush}
	window.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) {
		saver.touch()
	})
	window.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) {
		saver.touch()
	})
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		saver.flush()
	})
	if runtime.GOOS == "darwin" {
		window.RegisterHook(events.Common.WindowClosing, lifecycle.handleClosing)
		app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
			lifecycle.handleReopen()
		})
	}
	app.OnShutdown(lifecycle.beginQuit)

	var once sync.Once
	gate := &showGate{window: window}
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		gate.scheduleFallback(windowShowFallback)
	})
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		once.Do(func() {
			state, usable := currentWindowState(window)
			if screens := app.Screen.GetAll(); len(screens) > 0 {
				saved, savedOK := loadWindowState(path)
				state = restoreWindowState(saved, savedOK, screens)
				window.SetBounds(application.Rect{
					X:      state.X,
					Y:      state.Y,
					Width:  state.Width,
					Height: state.Height,
				})
				usable = true
			}
			if usable {
				saver.activate(state)
			}
			gate.stopFallback()
			gate.ensure()
		})
	})
}

func currentWindowState(window *application.WebviewWindow) (windowState, bool) {
	width, height := window.Size()
	x, y := window.Position()
	state := windowState{X: x, Y: y, Width: width, Height: height}
	return state, width > 0 && height > 0
}

func (s *windowStateSaver) activate(state windowState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.ready = true
}

func (s *windowStateSaver) touch() {
	state, ok := currentWindowState(s.window)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready {
		return
	}
	s.state = state
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(500*time.Millisecond, s.flush)
}

func (s *windowStateSaver) flush() {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	state, ready := s.state, s.ready
	s.mu.Unlock()
	if !ready {
		return
	}
	if current, ok := currentWindowState(s.window); ok && current.usable() {
		state = current
	}
	if err := saveWindowState(s.path, state); err != nil {
		log.Printf("save window state: %v", err)
	}
}
