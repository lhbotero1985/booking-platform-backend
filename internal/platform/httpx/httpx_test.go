package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) Problem {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != ContentTypeProblem {
		t.Fatalf("Content-Type = %q, want %q", ct, ContentTypeProblem)
	}
	var p Problem
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return p
}

func TestWriteProblemFillsRequestIDAndInstance(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteProblem(w, r, NewProblem(http.StatusConflict, "SLOT_UNAVAILABLE", "taken"))
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/appointments", nil)
	req.Header.Set(HeaderRequestID, "client-req-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	p := decodeProblem(t, rec)
	if rec.Code != http.StatusConflict || p.Status != http.StatusConflict || p.Title != "Conflict" ||
		p.Code != "SLOT_UNAVAILABLE" || p.Detail != "taken" || p.Type != "about:blank" ||
		p.Instance != "/api/v1/appointments" || p.RequestID != "client-req-1" {
		t.Errorf("unexpected problem: %+v (status %d)", p, rec.Code)
	}
}

func TestRequestID(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
	}))

	tests := []struct {
		name, header string
		keep         bool
	}{
		{"valid client id is kept", "abc-123_x.y", true},
		{"missing id is generated", "", false},
		{"unsafe characters are replaced", "abc\nlog-injection", false},
		{"too long is replaced", strings.Repeat("a", 65), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set(HeaderRequestID, tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Header().Get(HeaderRequestID) != seen || seen == "" {
				t.Fatalf("header %q, context %q", rec.Header().Get(HeaderRequestID), seen)
			}
			if tt.keep && seen != tt.header {
				t.Errorf("id = %q, want %q", seen, tt.header)
			}
			if !tt.keep && (seen == tt.header || len(seen) != 32) {
				t.Errorf("id = %q, want generated 32-char hex", seen)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	SecurityHeaders(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	for k, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestRecoverReturnsProblemWithoutInternals(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("db password leaked in panic message")
	}), RequestID, Recover(logger))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "password") || strings.Contains(body, "goroutine") {
		t.Errorf("response leaks internals: %s", body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != ContentTypeProblem {
		t.Errorf("Content-Type = %q", ct)
	}
	var got Problem
	if err := json.Unmarshal([]byte(body), &got); err != nil || got.Code != CodeInternal || got.RequestID == "" {
		t.Errorf("problem = %+v, err %v", got, err)
	}
	if !strings.Contains(logBuf.String(), "panic recovered") || !strings.Contains(logBuf.String(), got.RequestID) {
		t.Errorf("panic not logged with request id: %s", logBuf.String())
	}
}

func TestRecoverRepanicsOnAbortHandler(t *testing.T) {
	h := Recover(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if err, _ := recover().(error); !errors.Is(err, http.ErrAbortHandler) {
			t.Errorf("recovered %v, want http.ErrAbortHandler", err)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
}

func TestAccessLog(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	}), RequestID, AccessLog(logger))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x?token=secret-value", nil))

	var entry map[string]any
	if err := json.Unmarshal(logBuf.Bytes(), &entry); err != nil {
		t.Fatalf("log not JSON: %v (%s)", err, logBuf.String())
	}
	if entry["status"] != float64(http.StatusTeapot) || entry["path"] != "/x" || entry["bytes"] != float64(2) {
		t.Errorf("unexpected log entry: %v", entry)
	}
	if strings.Contains(logBuf.String(), "secret-value") {
		t.Errorf("query string leaked to log: %s", logBuf.String())
	}
}

func TestLimitBody(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	h := LimitBody(32)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p payload
		if DecodeJSON(w, r, &p) {
			w.WriteHeader(http.StatusNoContent)
		}
	}))

	t.Run("declared length too large", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, jsonPost(t, strings.NewReader(`{"name":"`+strings.Repeat("a", 64)+`"}`)))
		if rec.Code != http.StatusRequestEntityTooLarge || decodeProblem(t, rec).Code != CodeBodyTooLarge {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("streamed body too large", func(t *testing.T) {
		req := jsonPost(t, strings.NewReader(`{"name":"`+strings.Repeat("a", 64)+`"}`))
		req.ContentLength = -1 // unknown length (chunked)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("small body passes", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, jsonPost(t, strings.NewReader(`{"name":"ok"}`)))
		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d", rec.Code)
		}
	})
}

func TestDecodeJSONRejectsInvalidInput(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	for name, body := range map[string]string{
		"malformed":     `{"name":`,
		"unknown field": `{"name":"a","isAdmin":true}`,
		"trailing data": `{"name":"a"}{"name":"b"}`,
		"wrong type":    `{"name":123}`,
		"empty body":    ``,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			var p payload
			if DecodeJSON(rec, jsonPost(t, strings.NewReader(body)), &p) {
				t.Fatal("DecodeJSON() = true, want false")
			}
			if rec.Code != http.StatusBadRequest || decodeProblem(t, rec).Code != CodeInvalidRequest {
				t.Errorf("status = %d", rec.Code)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := CORS([]string{"https://business.example.com"})(next)

	t.Run("allowed origin", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://business.example.com")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "https://business.example.com" ||
			rec.Header().Get("Access-Control-Allow-Credentials") != "true" || rec.Code != http.StatusOK {
			t.Errorf("headers = %v, status %d", rec.Header(), rec.Code)
		}
	})
	t.Run("allowed preflight", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "https://business.example.com")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Methods") == "" {
			t.Errorf("status = %d, headers %v", rec.Code, rec.Header())
		}
	})
	t.Run("denied origin gets no CORS headers", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("ACAO set for denied origin: %v", rec.Header())
		}
	})
	t.Run("denied preflight is 403", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		req.Header.Set("Access-Control-Request-Method", http.MethodDelete)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || decodeProblem(t, rec).Code != CodeForbiddenOrigin {
			t.Errorf("status = %d", rec.Code)
		}
	})
}

func TestRouterProblems(t *testing.T) {
	rt := NewRouter()
	rt.HandleFunc("GET /things/{id}", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"id": r.PathValue("id")})
	})

	t.Run("match", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/things/42", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"42"`) {
			t.Errorf("status = %d body %s", rec.Code, rec.Body)
		}
	})
	t.Run("not found", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/nope", nil))
		if rec.Code != http.StatusNotFound || decodeProblem(t, rec).Code != CodeNotFound {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/things/42", nil))
		if rec.Code != http.StatusMethodNotAllowed || decodeProblem(t, rec).Code != CodeMethodNotAllowed {
			t.Errorf("status = %d", rec.Code)
		}
		if allow := rec.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
			t.Errorf("Allow = %q", allow)
		}
	})
}

func jsonPost(t *testing.T, body io.Reader) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", body)
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	return r
}

func TestDecodeJSONRequiresJSONContentType(t *testing.T) {
	for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x", "application/jsonp", "garbage;;"} {
		rec := httptest.NewRecorder()
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(`{"name":"a"}`))
		if ct != "" {
			r.Header.Set("Content-Type", ct)
		}
		var p struct {
			Name string `json:"name"`
		}
		if DecodeJSON(rec, r, &p) {
			t.Errorf("Content-Type %q accepted", ct)
			continue
		}
		if rec.Code != http.StatusUnsupportedMediaType || decodeProblem(t, rec).Code != CodeUnsupportedMedia {
			t.Errorf("Content-Type %q: status %d", ct, rec.Code)
		}
	}
}
