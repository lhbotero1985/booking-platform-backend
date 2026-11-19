package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identityhttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/interfaces/http"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
)

type identityEnv struct {
	srv    *httptest.Server
	svc    *application.Service
	store  *apptest.Store
	mailer *apptest.Mailer
}

func newIdentityServer(t *testing.T) identityEnv {
	t.Helper()
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"APP_ENV": "test", "DATABASE_URL": "postgres://u:p@localhost:5432/test"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := identityEnv{store: apptest.NewStore(), mailer: &apptest.Mailer{}}
	e.svc = application.NewService(application.Deps{
		Store: e.store, Hasher: &apptest.Hasher{}, Mailer: e.mailer,
		Clock: clock.NewFixed(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)), Random: &apptest.FixedRandom{},
		Config: application.Config{PublicAppURL: "https://app.example.test"}, Logger: logger,
	})
	e.srv = httptest.NewServer(NewHTTPHandler(cfg, logger, Deps{Modules: []RouteRegistrar{identityhttp.New(e.svc, logger)}}))
	t.Cleanup(e.srv.Close)
	return e
}

// call sends a JSON request, validates the request itself against the contract (when wantReqValid)
// and the response against the contract, and returns the response body.
func (c contract) call(t *testing.T, srv *httptest.Server, path, body, contentType string, wantStatus int, wantReqValid bool) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)

	// Request validation needs its own readable body copy.
	vreq, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+path, strings.NewReader(body))
	vreq.Header.Set("Content-Type", contentType)
	if ok, errs := c.v.ValidateHttpRequest(vreq); ok != wantReqValid {
		t.Errorf("POST %s request valid=%v, want %v: %v", path, ok, wantReqValid, errs)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s: status %d, want %d (%s)", path, resp.StatusCode, wantStatus, b)
	}
	resp.Body = io.NopCloser(bytes.NewReader(b))
	if ok, errs := c.v.ValidateHttpResponse(req, resp); !ok {
		for _, e := range errs {
			t.Errorf("POST %s %d does not conform: %s (%s)", path, wantStatus, e.Message, e.Reason)
		}
	}
	return string(b)
}

func TestContractIdentityEndpoints(t *testing.T) {
	c := loadContract(t)
	e := newIdentityServer(t)
	const js = "application/json"
	reg := `{"email":"ana@example.com","password":"una clave suficientemente larga","fullName":"Ana Pérez"}`

	c.call(t, e.srv, "/api/v1/auth/register", reg, js, http.StatusAccepted, true)
	c.call(t, e.srv, "/api/v1/auth/register", `{"email":"bad","password":"short","fullName":"A"}`, js, http.StatusBadRequest, false)
	c.call(t, e.srv, "/api/v1/auth/register", "email=a", "application/x-www-form-urlencoded", http.StatusUnsupportedMediaType, false)
	c.call(t, e.srv, "/api/v1/auth/verify-email/resend", `{"email":"ana@example.com"}`, js, http.StatusAccepted, true)

	// Issue a real token through the worker use case and verify it.
	if err := e.svc.SendVerificationEmail(context.Background(), e.store.Enqueued[0]); err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`token=([^\s]+)`).FindStringSubmatch(e.mailer.Last().Body)
	raw, _ := url.QueryUnescape(m[1])
	c.call(t, e.srv, "/api/v1/auth/verify-email", `{"token":"`+raw+`"}`, js, http.StatusOK, true)
	c.call(t, e.srv, "/api/v1/auth/verify-email", `{"token":"`+raw+`"}`, js, http.StatusBadRequest, true)
}
