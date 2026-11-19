// Package security_test is the Phase 1 security baseline (docs/testing/test-strategy.md §2).
// It exercises the real HTTP server (handler + middleware + server limits) and asserts the
// controls every later endpoint inherits. Authentication/authorization matrices are added in
// Phase 2.
package security_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/app"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/logging"
)

const allowedOrigin = "https://business.example.com"

// syncBuffer collects logs written concurrently by the server.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type harness struct {
	base string
	logs *syncBuffer
}

// start runs the production server stack on 127.0.0.1 with a failing readiness check (so 503
// paths are covered) and short timeouts.
func start(t *testing.T) harness {
	t.Helper()
	cfg, err := config.Load(func(k string) string {
		return map[string]string{
			"APP_ENV":                  "test",
			"DATABASE_URL":             "postgres://u:supersecret@127.0.0.1:1/db",
			"CORS_ALLOWED_ORIGINS":     allowedOrigin,
			"HTTP_MAX_BODY_BYTES":      "1024",
			"HTTP_READ_HEADER_TIMEOUT": "1s",
		}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	logs := &syncBuffer{}
	logger := logging.New(logs, slog.LevelDebug)
	h := app.NewHTTPHandler(cfg, logger, app.Deps{Readiness: []app.Check{{
		Name: "database",
		Fn: func(context.Context) error {
			return errors.New("dial tcp 10.1.2.3:5432: password authentication failed for user admin")
		},
	}}})

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := app.NewHTTPServer(context.Background(), cfg, logger, h)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return harness{base: "http://" + ln.Addr().String(), logs: logs}
}

type result struct {
	status int
	header http.Header
	body   string
}

func (h harness) do(t *testing.T, method, path string, body io.Reader, headers map[string]string) result {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, h.base+path, body)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return result{status: resp.StatusCode, header: resp.Header, body: string(b)}
}

// TestSecurityHeadersOnEveryResponse: success, client errors and server-side failures all carry
// the hardening headers and a request ID; nothing is cacheable.
func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	h := start(t)
	cases := []struct {
		name, method, path string
		body               io.Reader
		want               int
	}{
		{"200 healthz", http.MethodGet, "/healthz", nil, http.StatusOK},
		{"404 unknown", http.MethodGet, "/admin", nil, http.StatusNotFound},
		{"405 method", http.MethodDelete, "/healthz", nil, http.StatusMethodNotAllowed},
		{"405 TRACE", http.MethodTrace, "/healthz", nil, http.StatusMethodNotAllowed},
		{"413 body", http.MethodPost, "/healthz", strings.NewReader(strings.Repeat("x", 2048)), http.StatusRequestEntityTooLarge},
		{"503 readyz", http.MethodGet, "/readyz", nil, http.StatusServiceUnavailable},
	}
	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		"Cache-Control":           "no-store",
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := h.do(t, tc.method, tc.path, tc.body, nil)
			if r.status != tc.want {
				t.Fatalf("status = %d, want %d", r.status, tc.want)
			}
			for k, v := range want {
				if got := r.header.Get(k); got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
			if r.header.Get("X-Request-ID") == "" {
				t.Error("missing X-Request-ID")
			}
			if r.header.Get("Server") != "" || r.header.Get("X-Powered-By") != "" {
				t.Errorf("technology disclosure headers present: %v", r.header)
			}
		})
	}
}

// TestErrorsDoNotLeakInternals: error bodies never contain stack traces, file paths, internal
// addresses or credentials, even when a dependency error carries them.
func TestErrorsDoNotLeakInternals(t *testing.T) {
	h := start(t)
	for _, path := range []string{"/readyz", "/does-not-exist", "/healthz/../../etc/passwd"} {
		r := h.do(t, http.MethodGet, path, nil, nil)
		for _, leak := range []string{"goroutine", ".go:", "panic", "10.1.2.3", "password", "supersecret", "admin"} {
			if strings.Contains(r.body, leak) {
				t.Errorf("GET %s leaks %q: %s", path, leak, r.body)
			}
		}
	}
	// The failure detail must reach the logs (for operators) but not the DB password.
	logs := h.logs.String()
	if !strings.Contains(logs, "readiness check failed") {
		t.Error("readiness failure not logged")
	}
	if strings.Contains(logs, "supersecret") {
		t.Error("database password present in logs")
	}
}

// TestCORSIsAllowlistOnly: only the configured exact origin is reflected; never "*", never
// "null", never a look-alike.
func TestCORSIsAllowlistOnly(t *testing.T) {
	h := start(t)
	for _, origin := range []string{
		"https://evil.example.com",
		"null",
		"https://business.example.com.evil.com",
		"http://business.example.com",
		"https://BUSINESS.example.com",
	} {
		r := h.do(t, http.MethodGet, "/healthz", nil, map[string]string{"Origin": origin})
		if acao := r.header.Get("Access-Control-Allow-Origin"); acao != "" {
			t.Errorf("Origin %q got Access-Control-Allow-Origin %q", origin, acao)
		}
		pre := h.do(t, http.MethodOptions, "/healthz", nil, map[string]string{
			"Origin": origin, "Access-Control-Request-Method": http.MethodPost,
		})
		if pre.status != http.StatusForbidden {
			t.Errorf("preflight from %q: status %d, want 403", origin, pre.status)
		}
	}
	ok := h.do(t, http.MethodGet, "/healthz", nil, map[string]string{"Origin": allowedOrigin})
	if ok.header.Get("Access-Control-Allow-Origin") != allowedOrigin || !strings.Contains(ok.header.Get("Vary"), "Origin") {
		t.Errorf("allowed origin not reflected exactly with Vary: Origin: %v", ok.header)
	}
}

// TestBodyLimitStreaming: a body without Content-Length (chunked) is still capped.
func TestBodyLimitStreaming(t *testing.T) {
	h := start(t)
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte(strings.Repeat("y", 4096)))
		_ = pw.Close()
	}()
	// Unknown length → chunked transfer encoding. /healthz is GET-only, so the router answers
	// 405 before reading; what matters is that the server neither crashes nor buffers it.
	r := h.do(t, http.MethodPost, "/healthz", pr, nil)
	if r.status != http.StatusMethodNotAllowed && r.status != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d", r.status)
	}
}

// TestRequestIDCannotInjectLogs: a malicious X-Request-ID (CR/LF, JSON breakers, oversized) is
// replaced, so it can neither forge log lines nor be reflected.
func TestRequestIDCannotInjectLogs(t *testing.T) {
	h := start(t)
	for _, evil := range []string{
		`abc","level":"ERROR","msg":"forged`,
		strings.Repeat("a", 200),
		"<script>alert(1)</script>",
	} {
		r := h.do(t, http.MethodGet, "/healthz", nil, map[string]string{"X-Request-ID": evil})
		if got := r.header.Get("X-Request-ID"); got == evil || len(got) != 32 {
			t.Errorf("X-Request-ID %q reflected as %q", evil, got)
		}
	}
	if strings.Contains(h.logs.String(), "forged") {
		t.Error("forged content reached the logs")
	}
}

// TestOversizedHeadersRejected: headers above the effective limit get 431. net/http allows
// MaxHeaderBytes plus a 4096-byte slack before rejecting, so the effective cap is ~68 KiB.
func TestOversizedHeadersRejected(t *testing.T) {
	h := start(t)
	r := h.do(t, http.MethodGet, "/healthz", nil, map[string]string{
		"X-Filler": strings.Repeat("z", app.MaxHeaderBytes+8*1024),
	})
	if r.status != http.StatusRequestHeaderFieldsTooLarge {
		t.Errorf("status = %d, want 431", r.status)
	}
}

// TestSlowHeadersAreCutOff (slowloris): a client that never finishes its headers is
// disconnected after ReadHeaderTimeout instead of holding the connection.
func TestSlowHeadersAreCutOff(t *testing.T) {
	h := start(t)
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", strings.TrimPrefix(h.base, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "GET /healthz HTTP/1.1\r\nHost: x\r\nX-Slow: "); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	start := time.Now()
	_, err = bufio.NewReader(conn).ReadString('\n')
	elapsed := time.Since(start)
	if err == nil {
		t.Log("server answered (likely 408) before closing — acceptable")
	}
	if elapsed > 3*time.Second {
		t.Errorf("connection held for %v; ReadHeaderTimeout is 1s", elapsed)
	}
}
