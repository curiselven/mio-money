package main

import (
	"embed"
	"flag"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

//go:embed assets
var assets embed.FS

func main() {
	dbPath := flag.String("db", store.DefaultPath(), "SQLite database path")
	maxChange := flag.Float64("max-change", 20, "anomaly threshold percentage")
	flag.Parse()

	if *maxChange < 0 {
		log.Fatal("--max-change must be >= 0")
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer st.Close()

	svc := domain.NewService(st, *maxChange/100)
	pending := NewPendingService(svc)
	overview := NewOverviewService(svc)
	app := application.New(application.Options{
		Name:     "Mio",
		Services: []application.Service{application.NewService(pending), application.NewService(overview)},
		Assets: application.AssetOptions{
			Handler:        application.AssetFileServerFS(assets),
			DisableLogging: true,
		},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Mio · 总览",
		URL:    "/",
		Width:  900,
		Height: 760,
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
