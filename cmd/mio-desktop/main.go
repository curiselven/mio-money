package main

import (
	"context"
	"embed"
	"flag"
	"log"
	"time"

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
	if result, err := svc.SyncLoans(context.Background(), time.Now().Format("2006-01-02"), ""); err != nil {
		log.Printf("loan sync: %v", err)
	} else if result.Created > 0 {
		log.Printf("loan sync: wrote %d snapshots", result.Created)
	}
	pending := NewPendingService(svc)
	overview := NewOverviewService(svc)
	loans := NewLoanService(svc)
	app := application.New(application.Options{
		Name:     "Mio",
		Services: []application.Service{application.NewService(pending), application.NewService(overview), application.NewService(loans)},
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
