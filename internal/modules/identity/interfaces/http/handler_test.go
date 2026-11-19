package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	httpapi "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/interfaces/http"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/logging"
)

type fixture struct {
	h      http.Handler
	svc    *application.Service
	store  *apptest.Store
	mailer *apptest.Mailer
	logs   *bytes.Buffer
}

func newFixture() fixture {
	f := fixture{store: apptest.NewStore(), mailer: &apptest.Mailer{}, logs: &bytes.Buffer{}}
	logger := logging.New(f.logs, slog.LevelDebug)
	f.svc = application.NewService(application.Deps{
		Store: f.store, Hasher: &apptest.Hasher{}, Mailer: f.mailer,
		Clock: clock.NewFixed(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)), Random: &apptest.FixedRandom{},
		Config: application.Config{PublicAppURL: "https://app.example.test"}, Logger: logger,
	})
	rt := httpx.NewRouter()
	httpapi.New(f.svc, logger).Routes(rt)
	f.h = httpx.Chain(rt, httpx.RequestID, httpx.Recover(logger), httpx.LimitBody(1<<20))
	return f
}

type resp struct {
	status int
	ct     string
	body   string
}

func (f fixture) post(t *testing.T, path, body string) resp {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	return resp{rec.Code, rec.Header().Get("Content-Type"), rec.Body.String()}
}

const regBody = `{"email":"ana@example.com","password":"una clave suficientemente larga","fullName":"Ana Pérez"}`

var tokenInLink = regexp.MustCompile(`token=([^\s]+)`)

func (f fixture) issueToken(t *testing.T) string {
	t.Helper()
	if err := f.svc.SendVerificationEmail(context.Background(), f.store.Enqueued[len(f.store.Enqueued)-1]); err != nil {
		t.Fatal(err)
	}
	m := tokenInLink.FindStringSubmatch(f.mailer.Last().Body)
	raw, _ := url.QueryUnescape(m[1])
	return raw
}

func TestRegisterAccepted(t *testing.T) {
	f := newFixture()
	r := f.post(t, "/api/v1/auth/register", regBody)
	if r.status != http.StatusAccepted || r.ct != "application/json" || strings.TrimSpace(r.body) != `{"status":"accepted"}` {
		t.Fatalf("register: %d %s %s", r.status, r.ct, r.body)
	}
}

func TestRegisterResponseIdenticalForExistingEmail(t *testing.T) {
	f := newFixture()
	first := f.post(t, "/api/v1/auth/register", regBody)
	second := f.post(t, "/api/v1/auth/register", strings.Replace(regBody, "ana@", "ANA@", 1))
	if first != second {
		t.Fatalf("responses differ (enumeration): %+v vs %+v", first, second)
	}
}

func TestRegisterValidationProblem(t *testing.T) {
	f := newFixture()
	r := f.post(t, "/api/v1/auth/register", `{"email":"nope","password":"short","fullName":""}`)
	if r.status != http.StatusBadRequest || r.ct != httpx.ContentTypeProblem {
		t.Fatalf("status %d ct %s", r.status, r.ct)
	}
	var p httpx.Problem
	if err := json.Unmarshal([]byte(r.body), &p); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range p.Errors {
		got[e.Field] = e.Message
	}
	if p.Code != httpx.CodeInvalidRequest || got["email"] != "INVALID_EMAIL" || got["password"] != "PASSWORD_TOO_SHORT" || got["fullName"] != "REQUIRED" {
		t.Fatalf("problem = %+v", p)
	}
}

func TestRegisterRejectsUnknownFieldsAndNonJSON(t *testing.T) {
	f := newFixture()
	if r := f.post(t, "/api/v1/auth/register", `{"email":"a@b.co","password":"x","fullName":"A","isPlatformAdmin":true}`); r.status != http.StatusBadRequest {
		t.Errorf("mass-assignment field accepted: %d", r.status)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/register", strings.NewReader("email=a@b.co"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("form post: %d", rec.Code)
	}
}

func TestVerifyEmailEndpoint(t *testing.T) {
	f := newFixture()
	f.post(t, "/api/v1/auth/register", regBody)
	raw := f.issueToken(t)

	if r := f.post(t, "/api/v1/auth/verify-email", `{"token":"`+raw+`"}`); r.status != http.StatusOK || strings.TrimSpace(r.body) != `{"status":"verified"}` {
		t.Fatalf("verify: %d %s", r.status, r.body)
	}
	for _, body := range []string{`{"token":"` + raw + `"}`, `{"token":"garbage"}`, `{"token":""}`} {
		r := f.post(t, "/api/v1/auth/verify-email", body)
		if r.status != http.StatusBadRequest || !strings.Contains(r.body, `"code":"INVALID_TOKEN"`) {
			t.Errorf("%s: %d %s", body, r.status, r.body)
		}
	}
}

func TestResendEndpointAlwaysAccepted(t *testing.T) {
	f := newFixture()
	known := f.post(t, "/api/v1/auth/verify-email/resend", `{"email":"nobody@example.com"}`)
	f.post(t, "/api/v1/auth/register", regBody)
	pending := f.post(t, "/api/v1/auth/verify-email/resend", `{"email":"ana@example.com"}`)
	if known != pending || known.status != http.StatusAccepted {
		t.Fatalf("responses differ: %+v vs %+v", known, pending)
	}
}

func TestInternalErrorsAreGeneric(t *testing.T) {
	f := newFixture()
	f.store.FailNext = errors.New("pq: connection to 10.0.0.9 refused password=s3cret")
	r := f.post(t, "/api/v1/auth/register", regBody)
	if r.status != http.StatusInternalServerError || strings.Contains(r.body, "10.0.0.9") || strings.Contains(r.body, "s3cret") {
		t.Fatalf("500 leaks internals: %d %s", r.status, r.body)
	}
}

func TestSecretsNeverLogged(t *testing.T) {
	f := newFixture()
	f.post(t, "/api/v1/auth/register", regBody)
	raw := f.issueToken(t)
	f.post(t, "/api/v1/auth/verify-email", `{"token":"`+raw+`"}`)
	logs := f.logs.String()
	for _, secret := range []string{"una clave suficientemente larga", raw, raw[6:]} {
		if strings.Contains(logs, secret) {
			t.Fatalf("secret in logs: %s", logs)
		}
	}
}
