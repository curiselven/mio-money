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

func TestAppJSUsesNoNativeDialogs(t *testing.T) {
	body, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"confirm(", "prompt(", "alert("} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("app.js must not use %q", forbidden)
		}
	}
}

func TestEfundAssetsPresent(t *testing.T) {
	index, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "基金E账户") || !strings.Contains(string(index), `data-view="efund"`) {
		t.Fatal("efund import view is missing")
	}
	if !strings.Contains(string(index), "账户管理") || !strings.Contains(string(index), `data-view="accounts"`) {
		t.Fatal("account management view is missing")
	}
	if !strings.Contains(string(app), "EfundService.Preview") || !strings.Contains(string(app), "EfundService.ImportTarget") {
		t.Fatal("efund service calls are missing")
	}
	if !strings.Contains(string(app), "AccountService.List") {
		t.Fatal("account service calls are missing")
	}
}
