package main

import (
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeAppWindow struct {
	visible     bool
	minimised   bool
	shows       int
	hides       int
	unminimises int
	focuses     int
	shown       chan struct{}
}

func (f *fakeAppWindow) Show() application.Window {
	f.shows++
	f.visible = true
	if f.shown != nil {
		select {
		case f.shown <- struct{}{}:
		default:
		}
	}
	return nil
}

func (f *fakeAppWindow) Hide() application.Window {
	f.hides++
	f.visible = false
	return nil
}

func (f *fakeAppWindow) UnMinimise()       { f.unminimises++; f.minimised = false }
func (f *fakeAppWindow) Focus()            { f.focuses++ }
func (f *fakeAppWindow) IsVisible() bool   { return f.visible }
func (f *fakeAppWindow) IsMinimised() bool { return f.minimised }

func TestCloseHidesSavesAndCancels(t *testing.T) {
	window := &fakeAppWindow{visible: true}
	flushes := 0
	lifecycle := &windowLifecycle{window: window, flush: func() { flushes++ }}

	event := application.NewWindowEvent()
	lifecycle.handleClosing(event)

	if !event.IsCancelled() {
		t.Fatal("close was not cancelled")
	}
	if window.hides != 1 || window.visible {
		t.Fatalf("window was not hidden: hides=%d visible=%v", window.hides, window.visible)
	}
	if flushes != 1 {
		t.Fatalf("flush calls = %d, want 1", flushes)
	}
}

func TestCloseDuringQuitDoesNotHide(t *testing.T) {
	window := &fakeAppWindow{visible: true}
	flushes := 0
	lifecycle := &windowLifecycle{window: window, flush: func() { flushes++ }}
	lifecycle.beginQuit()

	event := application.NewWindowEvent()
	lifecycle.handleClosing(event)

	if event.IsCancelled() {
		t.Fatal("close was cancelled during quit")
	}
	if window.hides != 0 {
		t.Fatalf("window hidden during quit: %d", window.hides)
	}
	if flushes != 1 {
		t.Fatalf("flush calls = %d, want 1", flushes)
	}
}

func TestReopenRestoresHiddenOrMinimisedWindow(t *testing.T) {
	tests := []struct {
		name      string
		visible   bool
		minimised bool
		wantShow  bool
	}{
		{name: "hidden", wantShow: true},
		{name: "minimised", visible: true, minimised: true, wantShow: true},
		{name: "visible", visible: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			window := &fakeAppWindow{visible: tc.visible, minimised: tc.minimised}
			lifecycle := &windowLifecycle{window: window, flush: func() {}}
			lifecycle.handleReopen()

			if tc.wantShow {
				if window.shows != 1 || window.unminimises != 1 || window.focuses != 1 {
					t.Fatalf("shows=%d unminimises=%d focuses=%d, want 1/1/1",
						window.shows, window.unminimises, window.focuses)
				}
				return
			}
			if window.shows != 0 || window.focuses != 0 {
				t.Fatalf("visible window was touched: shows=%d focuses=%d", window.shows, window.focuses)
			}
		})
	}
}

func TestShowGateFallbackShowsWindow(t *testing.T) {
	window := &fakeAppWindow{shown: make(chan struct{}, 2)}
	gate := &showGate{window: window}
	gate.scheduleFallback(5 * time.Millisecond)

	select {
	case <-window.shown:
	case <-time.After(time.Second):
		t.Fatal("fallback did not show the window")
	}
}

func TestShowGateShowsOnceWhenReadyArrives(t *testing.T) {
	window := &fakeAppWindow{shown: make(chan struct{}, 2)}
	gate := &showGate{window: window}
	gate.scheduleFallback(20 * time.Millisecond)
	gate.ensure()
	gate.stopFallback()

	select {
	case <-window.shown:
	case <-time.After(time.Second):
		t.Fatal("ready did not show the window")
	}
	select {
	case <-window.shown:
		t.Fatal("fallback showed an already visible window")
	case <-time.After(60 * time.Millisecond):
	}
}
