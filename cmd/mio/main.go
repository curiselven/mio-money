package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/httpserver"
	mioserver "github.com/curiselven/mio-money/internal/mcp"
	"github.com/curiselven/mio-money/internal/store"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	dbPath := flag.String("db", store.DefaultPath(), "SQLite database path")
	maxChange := flag.Float64("max-change", 20, "reject confirm when an account total moves more than this percentage")
	httpMode := flag.Bool("http", false, "serve MCP over Streamable HTTP instead of stdio")
	addr := flag.String("addr", "127.0.0.1:7420", "HTTP listen address")
	allow := flag.String("allow", "127.0.0.1/32,::1/128", "comma-separated CIDRs allowed to connect")
	sessionIdle := flag.Duration("session-idle", 30*time.Minute, "expire idle HTTP MCP sessions after this duration")
	flag.Parse()

	if *maxChange < 0 {
		log.Fatal("--max-change must be >= 0")
	}
	httpCfg := httpserver.Config{
		Token:        os.Getenv("MIO_HTTP_TOKEN"),
		AllowedCIDRs: strings.Split(*allow, ","),
		SessionIdle:  *sessionIdle,
	}
	if *httpMode {
		if err := httpCfg.Validate(); err != nil {
			log.Fatalf("http server: %v", err)
		}
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer st.Close()

	svc := domain.NewService(st, *maxChange/100)
	server := mioserver.NewServer(svc)
	if *httpMode {
		handler, err := httpserver.NewHandler(server, httpCfg)
		if err != nil {
			log.Fatalf("http server: %v", err)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := runHTTP(ctx, *addr, handler); err != nil {
			log.Fatalf("http server: %v", err)
		}
		return
	}
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}

func runHTTP(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Printf("mio HTTP MCP listening on %s", addr)

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			srv.Close()
		}
		return nil
	}
}
