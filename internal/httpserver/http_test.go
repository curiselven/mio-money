package httpserver_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/httpserver"
	mioserver "github.com/curiselven/mio-money/internal/mcp"
	"github.com/curiselven/mio-money/internal/store"
)

const testToken = "0123456789abcdef01234567"

func TestAuthenticationAndSourceChecks(t *testing.T) {
	handler, err := httpserver.NewHandler(nil, httpserver.Config{
		Token:        testToken,
		AllowedCIDRs: []string{"127.0.0.1/32", "::1/128"},
		SessionIdle:  time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		remoteAddr string
		auth       string
		want       int
	}{
		{name: "no token", remoteAddr: "127.0.0.1:1234", want: http.StatusUnauthorized},
		{name: "wrong token", remoteAddr: "127.0.0.1:1234", auth: "Bearer wrong", want: http.StatusUnauthorized},
		{name: "ipv6 loopback", remoteAddr: "[::1]:1234", want: http.StatusUnauthorized},
		{name: "disallowed IP", remoteAddr: "203.0.113.5:1234", auth: "Bearer " + testToken, want: http.StatusForbidden},
		{name: "source checked before token", remoteAddr: "203.0.113.5:1234", want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestStreamableHTTPClient(t *testing.T) {
	ctx := context.Background()
	httpServer, client := newTestServer(t, time.Minute)
	session, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx,
		&sdk.StreamableClientTransport{
			Endpoint:             httpServer.URL,
			HTTPClient:           client,
			DisableStandaloneSSE: true,
		}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 10 {
		t.Fatalf("got %d tools, want 10", len(tools.Tools))
	}
	if _, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "list_accounts", Arguments: map[string]any{}}); err != nil {
		t.Fatalf("list_accounts: %v", err)
	}
}

func TestSessionExpiresWhenIdle(t *testing.T) {
	ctx := context.Background()
	httpServer, client := newTestServer(t, 50*time.Millisecond)
	session, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx,
		&sdk.StreamableClientTransport{
			Endpoint:             httpServer.URL,
			HTTPClient:           client,
			DisableStandaloneSSE: true,
		}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	sessionID := session.ID()
	if sessionID == "" {
		t.Fatal("session id is empty")
	}

	time.Sleep(300 * time.Millisecond)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpServer.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", sessionID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestConfigValidation(t *testing.T) {
	valid := httpserver.Config{
		Token:        testToken,
		AllowedCIDRs: []string{"127.0.0.1/32"},
		SessionIdle:  time.Minute,
	}
	for _, token := range []string{"", "short", strings.Repeat("a", httpserver.MinTokenLength-1)} {
		cfg := valid
		cfg.Token = token
		if err := cfg.Validate(); err == nil {
			t.Fatalf("token of length %d should be rejected", len(token))
		}
	}
	cfg := valid
	cfg.AllowedCIDRs = []string{"not-a-cidr"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid CIDR should be rejected")
	}
	for _, idle := range []time.Duration{0, -time.Second} {
		cfg := valid
		cfg.SessionIdle = idle
		if err := cfg.Validate(); err == nil {
			t.Fatalf("session idle %v should be rejected", idle)
		}
	}
}

func newTestServer(t *testing.T, sessionIdle time.Duration) (*httptest.Server, *http.Client) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "mio.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	handler, err := httpserver.NewHandler(
		mioserver.NewServer(domain.NewService(st, 0.2)),
		httpserver.Config{
			Token:        testToken,
			AllowedCIDRs: []string{"127.0.0.1/32", "::1/128"},
			SessionIdle:  sessionIdle,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	listener := newPipeListener()
	httpServer := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	httpServer.Start()
	t.Cleanup(httpServer.Close)

	client := &http.Client{Transport: bearerTransport{
		token: testToken,
		base:  &http.Transport{DialContext: listener.dial},
	}}
	return httpServer, client
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr("127.0.0.1:80") }

func (l *pipeListener) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case l.conns <- remoteAddrConn{Conn: server}:
		return client, nil
	case <-ctx.Done():
		client.Close()
		server.Close()
		return nil, ctx.Err()
	case <-l.done:
		client.Close()
		server.Close()
		return nil, net.ErrClosed
	}
}

type remoteAddrConn struct {
	net.Conn
}

func (c remoteAddrConn) RemoteAddr() net.Addr { return pipeAddr("127.0.0.1:12345") }

type pipeAddr string

func (a pipeAddr) Network() string { return "tcp" }
func (a pipeAddr) String() string  { return string(a) }
