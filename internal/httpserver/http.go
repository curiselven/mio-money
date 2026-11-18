package httpserver

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const MinTokenLength = 24

type Config struct {
	Token        string
	AllowedCIDRs []string
	SessionIdle  time.Duration
}

func (c Config) Validate() error {
	_, err := prepare(c)
	return err
}

func prepare(cfg Config) ([]netip.Prefix, error) {
	allowed, err := parseAllowed(cfg.AllowedCIDRs)
	if err != nil {
		return nil, err
	}
	if len(cfg.Token) < MinTokenLength {
		return nil, fmt.Errorf("MIO_HTTP_TOKEN must be at least %d characters", MinTokenLength)
	}
	if cfg.SessionIdle <= 0 {
		return nil, errors.New("--session-idle must be > 0")
	}
	return allowed, nil
}

func NewHandler(server *sdk.Server, cfg Config) (http.Handler, error) {
	allowed, err := prepare(cfg)
	if err != nil {
		return nil, err
	}

	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server {
		return server
	}, &sdk.StreamableHTTPOptions{SessionTimeout: cfg.SessionIdle})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !remoteAllowed(r.RemoteAddr, allowed) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(cfg.Token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}), nil
}

func parseAllowed(cidrs []string) ([]netip.Prefix, error) {
	if len(cidrs) == 0 {
		return nil, errors.New("--allow must not be empty")
	}
	allowed := make([]netip.Prefix, 0, len(cidrs))
	for _, raw := range cidrs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", raw, err)
		}
		allowed = append(allowed, prefix)
	}
	return allowed, nil
}

func remoteAllowed(remoteAddr string, allowed []netip.Prefix) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range allowed {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
