package security_test

// Phase 7 security (T7.10): the links of the appointment emails and the WhatsApp webhook, attacked
// end to end on the composed handler.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/app"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	notifdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	notificationshttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/interfaces/http"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
)

// emailLinks books an appointment by hand for a customer with an email, sends that email and
// returns its links (as the real sender issues them) and the appointment's id.
func emailLinks(t *testing.T, h authHarness) (map[notifdomain.Action]string, string) {
	t.Helper()
	h.account(t, suiteEmail)
	ana := h.bearerToken(t, suiteEmail)
	post := func(path, body string, headers map[string]string) string {
		t.Helper()
		r := h.call(t, http.MethodPost, path, body, headers)
		var v struct{ ID string }
		if r.status != http.StatusCreated || json.Unmarshal([]byte(r.body), &v) != nil {
			t.Fatalf("%s: %d %s", path, r.status, r.body)
		}
		return v.ID
	}
	biz := post("/api/v1/businesses", `{"name":"Barbería Enlaces","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, bearerH(ana))
	base := "/api/v1/business/" + biz
	pro := post(base+"/professionals", `{"email":"`+suiteEmail+`","displayName":"Ana"}`, bearerH(ana))
	svc := post(base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, bearerH(ana))
	for path, body := range map[string]string{
		"/schedule": `{"days":[{"weekday":"MONDAY","workingHours":[{"start":"09:00","end":"18:00"}]}]}`,
		"/services": `{"serviceIds":["` + svc + `"]}`,
	} {
		if r := h.call(t, http.MethodPut, base+"/professionals/"+pro+path, body, bearerH(ana)); r.status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, r.status, r.body)
		}
	}
	bookH := bearerH(ana)
	bookH["Idempotency-Key"] = "enlaces-seguridad-1"
	appt := post(base+"/appointments", `{"professionalId":"`+pro+`","serviceId":"`+svc+`","startAt":"2026-11-23T14:00:00Z","customer":{"fullName":"Laura","phone":"3002223344","email":"laura@example.com"}}`, bookH)
	for _, m := range h.outbox.Messages() {
		if m.Channel == notifdomain.ChannelEmail && m.RecipientRole == notifdomain.RecipientCustomer && m.AppointmentID.String() == appt {
			if err := h.notif.Send(context.Background(), m.ID, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	out := h.outbox.Outbox()
	if len(out) == 0 || len(out[len(out)-1].Links) < 2 {
		t.Fatalf("no email with links: %+v", out)
	}
	return out[len(out)-1].Links, appt
}

// TestEmailLinksGiveNothingAway: a link that cannot be used (unknown, of another action, spent or
// expired) answers exactly the same page and JSON, so a guess learns nothing; opening a link
// changes nothing; what is typed in the URL is never reflected; the codes are never logged.
func TestEmailLinksGiveNothingAway(t *testing.T) {
	h := startAuth(t)
	links, _ := emailLinks(t, h)
	cancel, resched := links[notifdomain.ActionCancel], links[notifdomain.ActionReschedule]
	lookup := func(tok, action string) string {
		t.Helper()
		r := h.call(t, http.MethodPost, "/api/v1/appointment-links/lookup", `{"token":"`+tok+`","action":"`+action+`"}`, nil)
		if r.status != http.StatusOK {
			t.Fatalf("lookup: %d %s", r.status, r.body)
		}
		return r.body
	}
	page := func(path, tok string) reply {
		t.Helper()
		return h.call(t, http.MethodGet, path+"?token="+url.QueryEscape(tok), "", nil)
	}

	// Opening the links (mail scanners do it) changes nothing, however many times.
	for range 5 {
		for _, p := range []string{"/citas/cancelar", "/citas/reagendar"} {
			if r := page(p, map[string]string{"/citas/cancelar": cancel, "/citas/reagendar": resched}[p]); r.status != http.StatusOK || strings.Contains(r.body, "no es válido") {
				t.Fatalf("open %s: %d", p, r.status)
			}
		}
	}
	if got := lookup(cancel, "CANCEL"); !strings.Contains(got, `"outcome":null`) {
		t.Fatalf("after opening: %s", got)
	}

	// Unusable links, all alike.
	unknown := strings.Repeat("A", len(cancel))
	invalidPage := page("/citas/cancelar", unknown)
	invalidJSON := lookup(unknown, "CANCEL")
	same := func(name string, r reply, js string) {
		t.Helper()
		if r.status != invalidPage.status || r.body != invalidPage.body || js != invalidJSON {
			t.Errorf("%s answers differently from an unknown code:\n%s\n%s", name, r.body, js)
		}
	}
	same("a code of another action", page("/citas/cancelar", resched), lookup(resched, "CANCEL"))
	injected := `"><script>alert(1)</script>`
	if r := page("/citas/cancelar", injected); strings.Contains(r.body, "<script>alert") || r.body != invalidPage.body {
		t.Error("the URL is reflected")
	}
	// Spend every link (a new time), then expire: still the same answers.
	if r := h.call(t, http.MethodPost, "/api/v1/appointment-links/apply", `{"token":"`+cancel+`","action":"CANCEL"}`, nil); r.status != http.StatusOK || !strings.Contains(r.body, `"CANCELLED"`) { //nolint:misspell // stored value
		t.Fatalf("cancel: %d %s", r.status, r.body)
	}
	same("a spent code", page("/citas/cancelar", cancel), lookup(cancel, "CANCEL"))
	same("a spent code of the same email", page("/citas/cancelar", resched), lookup(resched, "CANCEL"))
	h.clock.Advance(30 * 24 * time.Hour)
	same("an expired code", page("/citas/cancelar", cancel), lookup(cancel, "CANCEL"))
	if r := h.call(t, http.MethodPost, "/citas/cancelar", url.Values{"token": {cancel}}.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}); r.status != http.StatusOK || r.body != invalidPage.body {
		t.Errorf("second tap of an expired code: %d", r.status)
	}

	for _, raw := range links {
		if strings.Contains(h.logs.String(), raw) {
			t.Error("a link code was logged")
		}
	}
	if !strings.Contains(invalidPage.header.Get("Content-Security-Policy"), "default-src 'none'") || invalidPage.header.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("headers %v", invalidPage.header)
	}
}

// TestEmailLinksAreRateLimited: guessing codes from one client is throttled as in cmd/api.
func TestEmailLinksAreRateLimited(t *testing.T) {
	h := startAuth(t)
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"APP_ENV": "test", "DATABASE_URL": "postgres://u:p@127.0.0.1:1/db"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	limiters := app.NewRateLimiters(cfg.RateLimit, h.clock)
	srv := httptest.NewServer(app.NewHTTPHandler(cfg, logger, app.Deps{Modules: []app.RouteRegistrar{
		notificationshttp.NewLinks(h.notif, notificationshttp.LinksConfig{}, limiters.Auth, logger),
	}}))
	t.Cleanup(srv.Close)
	for i := range 200 {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/citas/confirmar?token=adivina"+strings.Repeat("x", i%7), nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			if resp.Header.Get("Retry-After") == "" || i > 60 {
				t.Errorf("throttled after %d guesses, Retry-After %q", i, resp.Header.Get("Retry-After"))
			}
			return
		}
	}
	t.Error("200 guesses were not throttled")
}

// TestWebhookRejectsForgeries: only bodies signed with the App Secret are taken, as received
// (a byte changed after signing is refused); the verification needs the verify token.
func TestWebhookRejectsForgeries(t *testing.T) {
	h := startAuth(t)
	body := `{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{"messages":[{"id":"wamid.forjado","from":"573001112233","type":"button","button":{"payload":"x"}}]}}]}]}`
	sign := func(secret, b string) string {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(b))
		return "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
	post := func(b, sig string) int {
		headers := map[string]string{}
		if sig != "" {
			headers["X-Hub-Signature-256"] = sig
		}
		return h.call(t, http.MethodPost, "/webhooks/whatsapp", b, headers).status
	}
	for name, code := range map[string]int{
		"unsigned":                post(body, ""),
		"signed with another key": post(body, sign("otra-clave", body)),
		"changed after signing":   post(strings.Replace(body, "573001112233", "573009999999", 1), sign(testWebhookSecret, body)),
		"sha1 header":             post(body, "sha1="+strings.TrimPrefix(sign(testWebhookSecret, body), "sha256=")),
		"malformed signature":     post(body, "sha256=zz"),
	} {
		if code < 400 || code >= 500 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if n := len(h.outbox.WebhookItems()); n != 0 {
		t.Fatalf("%d forged items queued", n)
	}
	if code := post(body, sign(testWebhookSecret, body)); code != http.StatusOK || len(h.outbox.WebhookItems()) != 1 {
		t.Errorf("signed by Meta: %d, %d items", code, len(h.outbox.WebhookItems()))
	}
	verify := "/webhooks/whatsapp?hub.mode=subscribe&hub.challenge=1234&hub.verify_token="
	if r := h.call(t, http.MethodGet, verify+"adivinado", "", nil); r.status != http.StatusForbidden || strings.Contains(r.body, "1234") {
		t.Errorf("wrong verify token: %d %q", r.status, r.body)
	}
	if r := h.call(t, http.MethodGet, verify+testWebhookVerifyToken, "", nil); r.status != http.StatusOK || r.body != "1234" {
		t.Errorf("right verify token: %d %q", r.status, r.body)
	}
}
