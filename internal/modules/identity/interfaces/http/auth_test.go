package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpapi "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/interfaces/http"
)

type authResp struct {
	status  int
	header  http.Header
	body    string
	cookies []*http.Cookie
}

func (f fixture) do(t *testing.T, method, path, body string, headers map[string]string, cookies ...*http.Cookie) authResp {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	return authResp{rec.Code, rec.Header(), rec.Body.String(), rec.Result().Cookies()}
}

// activeAccount registers and verifies ana@example.com.
func (f fixture) activeAccount(t *testing.T) {
	t.Helper()
	f.post(t, "/api/v1/auth/register", regBody)
	if r := f.post(t, "/api/v1/auth/verify-email", `{"token":"`+f.issueToken(t)+`"}`); r.status != http.StatusOK {
		t.Fatalf("verify: %d", r.status)
	}
}

const loginMobile = `{"email":"ana@example.com","password":"una clave suficientemente larga","clientType":"MOBILE"}`
const loginWeb = `{"email":"ana@example.com","password":"una clave suficientemente larga","clientType":"WEB"}`

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func (f fixture) mobileToken(t *testing.T) string {
	t.Helper()
	r := f.do(t, http.MethodPost, "/api/v1/auth/login", loginMobile, nil)
	var body struct{ Token, TokenType string }
	if err := json.Unmarshal([]byte(r.body), &body); err != nil || r.status != http.StatusOK || body.TokenType != "Bearer" {
		t.Fatalf("mobile login: %d %s", r.status, r.body)
	}
	return body.Token
}

func (f fixture) webCookie(t *testing.T) *http.Cookie {
	t.Helper()
	r := f.do(t, http.MethodPost, "/api/v1/auth/login", loginWeb, nil)
	if r.status != http.StatusOK || len(r.cookies) != 1 {
		t.Fatalf("web login: %d %s %v", r.status, r.body, r.cookies)
	}
	return r.cookies[0]
}

func TestMobileLoginReturnsBearerToken(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	tok := f.mobileToken(t)
	if !strings.HasPrefix(tok, "bp_s_") {
		t.Fatalf("token %q", tok)
	}
	me := f.do(t, http.MethodGet, "/api/v1/me", "", bearer(tok))
	if me.status != http.StatusOK || !strings.Contains(me.body, `"email":"ana@example.com"`) || !strings.Contains(me.body, `"status":"ACTIVE"`) {
		t.Fatalf("me: %d %s", me.status, me.body)
	}
	if strings.Contains(me.body, "password") || strings.Contains(me.body, "argon2") {
		t.Fatalf("/me exposes the password hash: %s", me.body)
	}
}

func TestWebLoginUsesHardenedCookieOnly(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	r := f.do(t, http.MethodPost, "/api/v1/auth/login", loginWeb, nil)
	if r.status != http.StatusOK || strings.Contains(r.body, "bp_s_") || strings.Contains(r.body, `"token"`) {
		t.Fatalf("web login must not return the token in the body: %s", r.body)
	}
	c := r.cookies[0]
	if c.Name != httpapi.SessionCookie || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" || !strings.HasPrefix(c.Value, "bp_s_") {
		t.Fatalf("cookie not hardened: %+v", c)
	}
	// Safe method with cookie and no Origin is fine.
	if me := f.do(t, http.MethodGet, "/api/v1/me", "", nil, c); me.status != http.StatusOK {
		t.Fatalf("GET /me with cookie: %d", me.status)
	}
}

func TestLoginErrorsDoNotRevealAccounts(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	unknown := f.do(t, http.MethodPost, "/api/v1/auth/login", strings.Replace(loginMobile, "ana@", "nadie@", 1), nil)
	wrong := f.do(t, http.MethodPost, "/api/v1/auth/login", strings.Replace(loginMobile, "larga", "corta", 1), nil)
	if unknown.status != http.StatusUnauthorized || wrong.status != http.StatusUnauthorized {
		t.Fatalf("statuses %d / %d", unknown.status, wrong.status)
	}
	strip := func(s string) string { // request IDs differ by design
		i := strings.Index(s, `"requestId"`)
		if i < 0 {
			return s
		}
		return s[:i]
	}
	if strip(unknown.body) != strip(wrong.body) || !strings.Contains(unknown.body, `"code":"INVALID_CREDENTIALS"`) {
		t.Fatalf("bodies differ:\n%s\n%s", unknown.body, wrong.body)
	}
}

func TestLoginUnverifiedAccount(t *testing.T) {
	f := newFixture()
	f.post(t, "/api/v1/auth/register", regBody)
	r := f.do(t, http.MethodPost, "/api/v1/auth/login", loginMobile, nil)
	if r.status != http.StatusForbidden || !strings.Contains(r.body, `"code":"EMAIL_NOT_VERIFIED"`) {
		t.Fatalf("unverified login: %d %s", r.status, r.body)
	}
}

// TestUnauthenticatedMatrix: every way of presenting a bad credential gets 401 with a
// WWW-Authenticate challenge and no cookie fallback for non-Bearer Authorization headers.
func TestUnauthenticatedMatrix(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	cookie := f.webCookie(t)
	cases := map[string]map[string]string{
		"no credentials":        nil,
		"empty bearer":          {"Authorization": "Bearer "},
		"basic scheme":          {"Authorization": "Basic YW5hOnB3"},
		"garbage token":         {"Authorization": "Bearer not-a-token"},
		"verification token":    {"Authorization": "Bearer bp_ev_" + strings.Repeat("A", 43)},
		"unknown session token": {"Authorization": "Bearer bp_s_" + strings.Repeat("A", 43)},
	}
	for name, h := range cases {
		var cs []*http.Cookie
		if name == "basic scheme" {
			cs = []*http.Cookie{cookie} // a non-Bearer header must not fall back to the cookie
		}
		r := f.do(t, http.MethodGet, "/api/v1/me", "", h, cs...)
		if r.status != http.StatusUnauthorized || r.header.Get("WWW-Authenticate") == "" || !strings.Contains(r.body, `"code":"UNAUTHENTICATED"`) {
			t.Errorf("%s: %d %s", name, r.status, r.body)
		}
	}
}

func TestCSRFProtectionForCookieSessions(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	cookie := f.webCookie(t)

	for name, origin := range map[string]string{"no origin": "", "foreign origin": "https://evil.example.com", "lookalike": "https://business.example.test.evil.com"} {
		h := map[string]string{}
		if origin != "" {
			h["Origin"] = origin
		}
		r := f.do(t, http.MethodPost, "/api/v1/auth/logout", "", h, cookie)
		if r.status != http.StatusForbidden || !strings.Contains(r.body, `"code":"CSRF_REJECTED"`) {
			t.Errorf("%s: %d %s", name, r.status, r.body)
		}
	}
	// Session still valid after the rejected attempts.
	if r := f.do(t, http.MethodGet, "/api/v1/me", "", nil, cookie); r.status != http.StatusOK {
		t.Fatal("rejected CSRF attempt must not log the user out")
	}
	// Allowed origin → logout works and clears the cookie.
	r := f.do(t, http.MethodPost, "/api/v1/auth/logout", "", map[string]string{"Origin": allowedOrigin}, cookie)
	if r.status != http.StatusNoContent || len(r.cookies) != 1 || r.cookies[0].MaxAge >= 0 {
		t.Fatalf("logout: %d cookies %+v", r.status, r.cookies)
	}
	if r := f.do(t, http.MethodGet, "/api/v1/me", "", nil, cookie); r.status != http.StatusUnauthorized {
		t.Fatal("cookie still valid after logout")
	}
	// Bearer clients (mobile) are not subject to the Origin check.
	tok := f.mobileToken(t)
	if r := f.do(t, http.MethodPost, "/api/v1/auth/logout", "", bearer(tok)); r.status != http.StatusNoContent {
		t.Fatalf("bearer logout: %d", r.status)
	}
}

func TestLogoutAllEndpoint(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	a, b := f.mobileToken(t), f.mobileToken(t)
	r := f.do(t, http.MethodPost, "/api/v1/auth/logout-all", "", bearer(a))
	if r.status != http.StatusOK || strings.TrimSpace(r.body) != `{"revoked":2}` {
		t.Fatalf("logout-all: %d %s", r.status, r.body)
	}
	for _, tok := range []string{a, b} {
		if r := f.do(t, http.MethodGet, "/api/v1/me", "", bearer(tok)); r.status != http.StatusUnauthorized {
			t.Fatal("token survived logout-all")
		}
	}
}

func TestSessionTokensNeverLogged(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	tok := f.mobileToken(t)
	f.do(t, http.MethodGet, "/api/v1/me", "", bearer(tok))
	cookie := f.webCookie(t)
	if strings.Contains(f.logs.String(), tok[5:]) || strings.Contains(f.logs.String(), cookie.Value[5:]) {
		t.Fatalf("session token in logs: %s", f.logs.String())
	}
}
