package httpapi_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var resetTokenInLink = regexp.MustCompile(`reset-password\?token=([^\s]+)`)

func (f fixture) resetToken(t *testing.T) string {
	t.Helper()
	if err := f.svc.SendPasswordResetEmail(context.Background(), f.store.ResetEnqueued[len(f.store.ResetEnqueued)-1]); err != nil {
		t.Fatal(err)
	}
	raw, _ := url.QueryUnescape(resetTokenInLink.FindStringSubmatch(f.mailer.Last().Body)[1])
	return raw
}

func TestForgotPasswordIdenticalResponses(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	known := f.post(t, "/api/v1/auth/password/forgot", `{"email":"ana@example.com"}`)
	unknown := f.post(t, "/api/v1/auth/password/forgot", `{"email":"nadie@example.com"}`)
	if known != unknown || known.status != http.StatusAccepted {
		t.Fatalf("responses differ: %+v vs %+v", known, unknown)
	}
}

func TestResetPasswordEndpoint(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	tok := f.mobileToken(t)
	f.post(t, "/api/v1/auth/password/forgot", `{"email":"ana@example.com"}`)
	raw := f.resetToken(t)

	weak := f.post(t, "/api/v1/auth/password/reset", `{"token":"`+raw+`","newPassword":"corta"}`)
	if weak.status != http.StatusBadRequest || !strings.Contains(weak.body, `"field":"newPassword"`) {
		t.Fatalf("weak password: %d %s", weak.status, weak.body)
	}
	r := f.do(t, http.MethodPost, "/api/v1/auth/password/reset", `{"token":"`+raw+`","newPassword":"otra frase nueva bastante larga"}`, nil)
	if r.status != http.StatusOK || strings.TrimSpace(r.body) != `{"status":"password_reset"}` || len(r.cookies) != 1 || r.cookies[0].MaxAge >= 0 {
		t.Fatalf("reset: %d %s %v", r.status, r.body, r.cookies)
	}
	if me := f.do(t, http.MethodGet, "/api/v1/me", "", bearer(tok)); me.status != http.StatusUnauthorized {
		t.Fatal("existing session survived the reset")
	}
	again := f.post(t, "/api/v1/auth/password/reset", `{"token":"`+raw+`","newPassword":"una tercera clave larga"}`)
	if again.status != http.StatusBadRequest || !strings.Contains(again.body, `"code":"INVALID_TOKEN"`) {
		t.Fatalf("reused token: %d %s", again.status, again.body)
	}
}

func TestChangePasswordEndpoint(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	tok := f.mobileToken(t)
	other := f.mobileToken(t)

	if r := f.do(t, http.MethodPost, "/api/v1/me/password", `{"currentPassword":"x","newPassword":"otra frase nueva bastante larga"}`, nil); r.status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated change: %d", r.status)
	}
	wrong := f.do(t, http.MethodPost, "/api/v1/me/password", `{"currentPassword":"no es la actual 123","newPassword":"otra frase nueva bastante larga"}`, bearer(tok))
	if wrong.status != http.StatusBadRequest || !strings.Contains(wrong.body, "INVALID_CURRENT_PASSWORD") {
		t.Fatalf("wrong current: %d %s", wrong.status, wrong.body)
	}
	ok := f.do(t, http.MethodPost, "/api/v1/me/password", `{"currentPassword":"una clave suficientemente larga","newPassword":"otra frase nueva bastante larga"}`, bearer(tok))
	if ok.status != http.StatusNoContent {
		t.Fatalf("change: %d %s", ok.status, ok.body)
	}
	if r := f.do(t, http.MethodGet, "/api/v1/me", "", bearer(tok)); r.status != http.StatusOK {
		t.Fatal("current session must survive")
	}
	if r := f.do(t, http.MethodGet, "/api/v1/me", "", bearer(other)); r.status != http.StatusUnauthorized {
		t.Fatal("other session must be revoked")
	}
}

func TestPasswordsNeverLogged(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	tok := f.mobileToken(t)
	f.do(t, http.MethodPost, "/api/v1/me/password", `{"currentPassword":"una clave suficientemente larga","newPassword":"otra frase nueva bastante larga"}`, bearer(tok))
	f.post(t, "/api/v1/auth/password/forgot", `{"email":"ana@example.com"}`)
	raw := f.resetToken(t)
	f.post(t, "/api/v1/auth/password/reset", `{"token":"`+raw+`","newPassword":"una tercera clave larguisima"}`)
	logs := f.logs.String()
	for _, secret := range []string{"una clave suficientemente larga", "otra frase nueva bastante larga", "una tercera clave larguisima", raw[6:]} {
		if strings.Contains(logs, secret) {
			t.Fatalf("secret %q in logs", secret)
		}
	}
}
