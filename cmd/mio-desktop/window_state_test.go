package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func testScreen(id string, primary bool, x, y, width, height int) *application.Screen {
	return &application.Screen{
		ID:        id,
		IsPrimary: primary,
		WorkArea: application.Rect{
			X: x, Y: y, Width: width, Height: height,
		},
	}
}

func TestWindowStatePath(t *testing.T) {
	got := windowStatePath(filepath.Join("data", "mio.db"))
	want := filepath.Join("data", "window-state.json")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestSaveLoadWindowState(t *testing.T) {
	path := windowStatePath(filepath.Join(t.TempDir(), "mio.db"))
	want := windowState{X: -1200, Y: 80, Width: 900, Height: 700}
	if err := saveWindowState(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 600", info.Mode().Perm())
	}
	got, ok := loadWindowState(path)
	if !ok || got != want {
		t.Fatalf("load = %+v, %v", got, ok)
	}
}

func TestLoadWindowStateRejectsMissingCorruptAndTiny(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-state.json")
	if _, ok := loadWindowState(path); ok {
		t.Fatal("missing file was accepted")
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadWindowState(path); ok {
		t.Fatal("corrupt file was accepted")
	}
	if err := saveWindowState(path, windowState{X: 10, Y: 10, Width: 500, Height: 400}); err == nil {
		t.Fatal("tiny window state was saved")
	}
	if err := os.WriteFile(path, []byte(`{"x":10,"y":10,"width":500,"height":400}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadWindowState(path); ok {
		t.Fatal("tiny window state was accepted")
	}
}

func TestRestoreWindowState(t *testing.T) {
	primary := testScreen("primary", true, 0, 0, 1920, 1055)
	secondary := testScreen("secondary", false, 1920, 0, 1920, 1055)
	screens := []*application.Screen{primary, secondary}

	tests := []struct {
		name    string
		saved   windowState
		savedOK bool
		screens []*application.Screen
		want    windowState
	}{
		{
			name:    "missing file centers default",
			screens: screens,
			want:    windowState{X: 470, Y: 22, Width: 980, Height: 1010},
		},
		{
			name:    "small screen shrinks default",
			screens: []*application.Screen{testScreen("small", true, 0, 0, 1280, 720)},
			want:    windowState{X: 150, Y: 0, Width: 980, Height: 720},
		},
		{
			name:    "valid saved state is restored",
			saved:   windowState{X: 2000, Y: 100, Width: 900, Height: 700},
			savedOK: true,
			screens: screens,
			want:    windowState{X: 2000, Y: 100, Width: 900, Height: 700},
		},
		{
			name:    "oversized saved state fits screen",
			saved:   windowState{X: 2500, Y: 200, Width: 2000, Height: 1400},
			savedOK: true,
			screens: screens,
			want:    windowState{X: 1920, Y: 0, Width: 1920, Height: 1055},
		},
		{
			name:    "offscreen saved state falls back centered",
			saved:   windowState{X: 5000, Y: 5000, Width: 900, Height: 700},
			savedOK: true,
			screens: screens,
			want:    windowState{X: 470, Y: 22, Width: 980, Height: 1010},
		},
		{
			name:    "tiny saved state falls back centered",
			saved:   windowState{X: 100, Y: 100, Width: 500, Height: 400},
			savedOK: true,
			screens: screens,
			want:    windowState{X: 470, Y: 22, Width: 980, Height: 1010},
		},
		{
			name: "no screens keeps default size",
			want: windowState{Width: 980, Height: 1010},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := restoreWindowState(tc.saved, tc.savedOK, tc.screens)
			if got != tc.want {
				t.Fatalf("state = %+v, want %+v", got, tc.want)
			}
		})
	}
}
