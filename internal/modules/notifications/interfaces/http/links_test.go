package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

type fakeLinks struct {
	view       application.LinkView
	err        error
	used       []domain.Action
	lastStart  time.Time
	lastToken  string
	lookedAt   int
	usedTokens []string
}

func (f *fakeLinks) Link(_ context.Context, raw string, a domain.Action) (application.LinkView, error) {
	f.lookedAt++
	f.lastToken = raw
	v := f.view
	v.Action = a
	return v, f.err
}

func (f *fakeLinks) UseLink(_ context.Context, raw string, a domain.Action, start time.Time) (application.LinkView, error) {
	f.used, f.lastStart = append(f.used, a), start
	f.usedTokens = append(f.usedTokens, raw)
	v := f.view
	v.Action = a
	if v.Outcome == "" {
		v.Outcome = map[domain.Action]domain.Outcome{domain.ActionConfirm: domain.OutcomeConfirmed, domain.ActionCancel: domain.OutcomeCancelled,
			domain.ActionReschedule: domain.OutcomeRescheduled}[a]
	}
	return v, f.err
}

func linksServer(t *testing.T, f *fakeLinks) *httptest.Server {
	t.Helper()
	rt := httpx.NewRouter()
	NewLinks(f, LinksConfig{AppDownloadURL: "https://booka.example/app"}, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes(rt)
	srv := httptest.NewServer(httpx.SecurityHeaders(rt))
	t.Cleanup(srv.Close)
	return srv
}

func view() application.LinkView {
	bogota, _ := time.LoadLocation("America/Bogota")
	return application.LinkView{Content: domain.Content{
		CustomerName: `Laura <script>alert(1)</script>`, BusinessName: "Barbería El Paisa", ServiceName: "Corte",
		ProfessionalName: "Andrés", Start: time.Date(2026, 12, 2, 15, 0, 0, 0, time.UTC), Location: bogota, CancelCutoffHours: 2, RescheduleCutoffHours: 1,
	}}
}

// page is a response already read and closed.
type page struct {
	StatusCode int
	Header     http.Header
}

func get(t *testing.T, u string) (page, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return page{resp.StatusCode, resp.Header}, string(b)
}

func TestLinkPages(t *testing.T) {
	f := &fakeLinks{view: view()}
	srv := linksServer(t, f)
	resp, page := get(t, srv.URL+"/citas/cancelar?token=abc%2B1")
	if resp.StatusCode != 200 || f.lastToken != "abc+1" || len(f.used) != 0 {
		t.Fatalf("first tap: %d %v", resp.StatusCode, f.used)
	}
	for _, want := range []string{"¿Cancelar tu cita?", "Hola Laura,", `action="/citas/cancelar"`, `name="token" value="abc&#43;1"`,
		"Sí, cancelar mi cita", "Miércoles 2 de diciembre de 2026", "10:00 a. m.", "Con Andrés", "Puedes cancelar hasta 2 horas antes"} {
		if !strings.Contains(page, want) {
			t.Errorf("cancel page without %q", want)
		}
	}
	if strings.Contains(page, "<script>") {
		t.Error("unescaped name")
	}
	h := resp.Header
	if !strings.Contains(h.Get("Content-Security-Policy"), "form-action 'self'") || !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'none'") ||
		h.Get("Referrer-Policy") != "no-referrer" || h.Get("Cache-Control") != "no-store" || h.Get("X-Robots-Tag") == "" {
		t.Errorf("headers %v", h)
	}
	_, page = get(t, srv.URL+"/citas/confirmar?token=x")
	if !strings.Contains(page, "Sí, confirmar mi cita") {
		t.Error("confirm page")
	}
	_, page = get(t, srv.URL+"/citas/reagendar?token=x")
	if !strings.Contains(page, "Cambia la hora de tu cita") || !strings.Contains(page, "https://booka.example/app") || strings.Contains(page, "<form") ||
		!strings.Contains(page, "hasta 1 hora antes") {
		t.Errorf("reschedule page: %s", page)
	}

	// Second tap.
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/citas/confirmar", strings.NewReader(url.Values{"token": {"abc+1"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r2.Body)
	_ = r2.Body.Close()
	if r2.StatusCode != 200 || len(f.used) != 1 || f.usedTokens[0] != "abc+1" || !strings.Contains(string(b), "¡Listo, Laura") {
		t.Errorf("second tap: %d %v %s", r2.StatusCode, f.used, b)
	}

	// No or too long token: the invalid page, the service is not asked.
	before := f.lookedAt
	for _, u := range []string{"/citas/confirmar", "/citas/confirmar?token=" + strings.Repeat("a", 101)} {
		if resp, page := get(t, srv.URL+u); resp.StatusCode != http.StatusOK || !strings.Contains(page, "Este enlace ya no es válido") {
			t.Errorf("%s: %d", u, resp.StatusCode)
		}
	}
	if f.lookedAt != before {
		t.Error("service asked without a token")
	}
}

func TestLinkPageOutcomes(t *testing.T) {
	for outcome, want := range map[domain.Outcome]string{
		domain.OutcomeInvalid:         "Este enlace ya no es válido",
		domain.OutcomeNotActive:       "Esta cita ya no está activa",
		domain.OutcomeCutoff:          "comunícate con Barbería El Paisa",
		domain.OutcomeSlotUnavailable: "Ese horario ya no está disponible",
		domain.OutcomeCancelled:       "Tu cita fue cancelada",
		domain.OutcomeRescheduled:     "Tu cita cambió de hora",
	} {
		v := view()
		v.Outcome = outcome
		srv := linksServer(t, &fakeLinks{view: v})
		resp, page := get(t, srv.URL+"/citas/cancelar?token=x")
		if resp.StatusCode != http.StatusOK || !strings.Contains(page, want) || strings.Contains(page, "<form") {
			t.Errorf("%s: %d %s", outcome, resp.StatusCode, page)
		}
	}
	srv := linksServer(t, &fakeLinks{err: errors.New("db down")})
	if resp, page := get(t, srv.URL+"/citas/confirmar?token=x"); resp.StatusCode != 500 || !strings.Contains(page, "Algo salió mal") || strings.Contains(page, "db down") {
		t.Errorf("failure: %d %s", resp.StatusCode, page)
	}
}

func postJSON(t *testing.T, u, body string) (int, string) {
	t.Helper()
	return do(t, http.MethodPost, u, body, map[string]string{"Content-Type": "application/json"})
}

func TestLinksAPI(t *testing.T) {
	v := view()
	v.BusinessSlug = "el-paisa"
	f := &fakeLinks{view: v}
	srv := linksServer(t, f)
	code, body := postJSON(t, srv.URL+"/api/v1/appointment-links/lookup", `{"token":"abc","action":"RESCHEDULE"}`)
	if code != 200 || !strings.Contains(body, `"outcome":null`) || !strings.Contains(body, `"businessSlug":"el-paisa"`) ||
		!strings.Contains(body, `"timeZone":"America/Bogota"`) || !strings.Contains(body, `"startAt":"2026-12-02T15:00:00Z"`) {
		t.Errorf("lookup: %d %s", code, body)
	}
	code, body = postJSON(t, srv.URL+"/api/v1/appointment-links/apply", `{"token":"abc","action":"RESCHEDULE","startAt":"2026-12-02T11:00:00-05:00"}`)
	if code != 200 || !strings.Contains(body, `"outcome":"RESCHEDULED"`) || !f.lastStart.Equal(time.Date(2026, 12, 2, 16, 0, 0, 0, time.UTC)) {
		t.Errorf("apply: %d %s %s", code, body, f.lastStart)
	}
	for name, in := range map[string]string{
		"bad action":          `{"token":"abc","action":"DELETE"}`,
		"no token":            `{"token":"","action":"CANCEL"}`,
		"reschedule no start": `{"token":"abc","action":"RESCHEDULE"}`,
	} {
		if code, _ := postJSON(t, srv.URL+"/api/v1/appointment-links/apply", in); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := do(t, http.MethodPost, srv.URL+"/api/v1/appointment-links/lookup", `{"token":"a","action":"CANCEL"}`, map[string]string{"Content-Type": "text/plain"}); code != 415 {
		t.Errorf("not JSON: %d", code)
	}
	// An invalid link says nothing about the appointment.
	inv := view()
	inv.Outcome = domain.OutcomeInvalid
	srv = linksServer(t, &fakeLinks{view: inv})
	if code, body := postJSON(t, srv.URL+"/api/v1/appointment-links/lookup", `{"token":"abc","action":"CANCEL"}`); code != 200 || !strings.Contains(body, `"appointment":null`) {
		t.Errorf("invalid: %d %s", code, body)
	}
	srv = linksServer(t, &fakeLinks{err: errors.New("db down")})
	if code, body := postJSON(t, srv.URL+"/api/v1/appointment-links/lookup", `{"token":"abc","action":"CANCEL"}`); code != 500 || strings.Contains(body, "db down") {
		t.Errorf("failure: %d %s", code, body)
	}
}

// TestPreviewPages writes the pages to LINK_PREVIEW_DIR, to look at them in a browser.
func TestPreviewPages(t *testing.T) {
	dir := os.Getenv("LINK_PREVIEW_DIR")
	if dir == "" {
		t.Skip("LINK_PREVIEW_DIR not set")
	}
	srv := linksServer(t, &fakeLinks{view: view()})
	for name, u := range map[string]string{"confirmar": "/citas/confirmar?token=x", "cancelar": "/citas/cancelar?token=x", "reagendar": "/citas/reagendar?token=x"} {
		_, body := get(t, srv.URL+u)
		if err := os.WriteFile(filepath.Join(dir, "page-"+name+".html"), []byte(body), 0o600); err != nil { //nolint:gosec // a directory chosen by the developer
			t.Fatal(err)
		}
	}
	inv := view()
	inv.Outcome = domain.OutcomeInvalid
	_, body := get(t, linksServer(t, &fakeLinks{view: inv}).URL+"/citas/cancelar?token=x")
	if err := os.WriteFile(filepath.Join(dir, "page-invalido.html"), []byte(body), 0o600); err != nil { //nolint:gosec // a directory chosen by the developer
		t.Fatal(err)
	}
}
