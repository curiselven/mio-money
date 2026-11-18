package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestEmbeddedAssets(t *testing.T) {
	handler := application.AssetFileServerFS(assets)
	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/", want: "Mio Money"},
		{path: "/app.js", want: "PendingService"},
		{path: "/style.css", want: ".card"},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s: status=%d body=%q", tc.path, rec.Code, rec.Body.String())
		}
	}
}
