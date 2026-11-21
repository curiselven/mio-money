package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const windowShowFallback = 3 * time.Second

type appWindow interface {
	Show() application.Window
	Hide() application.Window
	UnMinimise()
	Focus()
	IsVisible() bool
	IsMinimised() bool
}

// windowLifecycle keeps the single window alive so Dock and Launchpad can reopen it.
type windowLifecycle struct {
	window   appWindow
	flush    func()
	quitting atomic.Bool
}

func (life *windowLifecycle) handleClosing(event *application.WindowEvent) {
	if life.quitting.Load() {
		return
	}
	life.flush()
	life.window.Hide()
	event.Cancel()
}

func (life *windowLifecycle) handleReopen() {
	if life.window.IsVisible() && !life.window.IsMinimised() {
		return
	}
	life.window.Show()
	life.window.UnMinimise()
	life.window.Focus()
}

func (life *windowLifecycle) beginQuit() {
	life.quitting.Store(true)
	life.flush()
}

// showGate shows the initially hidden window once, from the ready event or the
// fallback timer, whichever comes first.
type showGate struct {
	window appWindow
	once   sync.Once
	mu     sync.Mutex
	timer  *time.Timer
}

func (gate *showGate) ensure() {
	gate.once.Do(func() { gate.window.Show() })
}

func (gate *showGate) scheduleFallback(timeout time.Duration) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	gate.timer = time.AfterFunc(timeout, gate.ensure)
}

func (gate *showGate) stopFallback() {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.timer != nil {
		gate.timer.Stop()
		gate.timer = nil
	}
}
