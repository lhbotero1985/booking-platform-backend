package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"APP_ENV": "test"}[k]
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	srv := httptest.NewServer(NewHTTPHandler(cfg, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	return srv
}

type response struct {
	status int
	header http.Header
	body   []byte
}

// do performs a request and returns the fully read response (the body is always closed).
func do(t *testing.T, srv *httptest.Server, method, path string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: body}
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, http.MethodGet, "/healthz")

	if resp.status != http.StatusOK {
		t.Fatalf("status = %d", resp.status)
	}
	if ct := resp.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if resp.header.Get(httpx.HeaderRequestID) == "" || resp.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("middleware headers missing: %v", resp.header)
	}
	var body healthResponse
	if err := json.Unmarshal(resp.body, &body); err != nil || body.Status != "ok" || body.Version == "" {
		t.Errorf("body = %+v, err %v", body, err)
	}
}

func TestUnknownRouteIsProblem(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, http.MethodGet, "/does-not-exist")

	if resp.status != http.StatusNotFound || resp.header.Get("Content-Type") != httpx.ContentTypeProblem {
		t.Fatalf("status = %d, Content-Type %q", resp.status, resp.header.Get("Content-Type"))
	}
	var p httpx.Problem
	if err := json.Unmarshal(resp.body, &p); err != nil || p.Code != httpx.CodeNotFound || p.RequestID == "" {
		t.Errorf("problem = %+v, err %v", p, err)
	}
}

func TestHealthzRejectsOtherMethods(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, http.MethodPost, "/healthz")
	if resp.status != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.status)
	}
}
