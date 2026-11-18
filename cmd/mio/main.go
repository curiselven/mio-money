package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/curiselven/mio-money/internal/domain"
	mioserver "github.com/curiselven/mio-money/internal/mcp"
	"github.com/curiselven/mio-money/internal/store"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	dbPath := flag.String("db", defaultDBPath(), "SQLite database path")
	maxChange := flag.Float64("max-change", 20, "reject confirm when an account total moves more than this percentage")
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
	server := mioserver.NewServer(svc)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}

func defaultDBPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "MioMoney", "mio.db")
}
