package security_test

// Phase 2 security suite: authentication, sessions and anti-enumeration exercised end to end on
// the composed HTTP handler (app.NewHTTPHandler + identity module), as wired in cmd/api.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/app"
	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application/biztest"
	businesseshttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/interfaces/http"
	catapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application/catalogtest"
	cataloghttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/interfaces/http"
	geoapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	geographyhttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/interfaces/http"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identityhttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/interfaces/http"
	profapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application/proftest"
	professionalshttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/interfaces/http"
	subapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/application/subtest"
	subscriptionshttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/interfaces/http"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/logging"
)

const (
	suitePassword = "una clave suficientemente larga"
	suiteEmail    = "ana@example.com"
)

type authHarness struct {
	srv    *httptest.Server
	svc    *application.Service
	store  *apptest.Store
	mailer *apptest.Mailer
	clock  *clock.Fixed
	logs   *syncBuffer
	geo    *geotest.Store
	biz    *biztest.Store
	subs   *subtest.Store
}

func startAuth(t *testing.T) authHarness {
	t.Helper()
	cfg, err := config.Load(func(k string) string {
		return map[string]string{
			"APP_ENV": "test", "DATABASE_URL": "postgres://u:p@127.0.0.1:1/db",
			"CORS_ALLOWED_ORIGINS": allowedOrigin,
		}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	h := authHarness{store: apptest.NewStore(), mailer: &apptest.Mailer{}, logs: &syncBuffer{},
		clock: clock.NewFixed(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)), geo: geotest.NewSeededStore(), biz: biztest.NewStore(), subs: subtest.NewStore()}
	logger := logging.New(h.logs, slog.LevelDebug)
	h.svc = application.NewService(application.Deps{
		Store: h.store, Hasher: &apptest.Hasher{}, Mailer: h.mailer, Clock: h.clock,
		Config: application.Config{PublicAppURL: "https://app.example.test"}, Logger: logger,
	})
	// Same composition as cmd/api: geography uses identity's authentication and authorizer.
	idh := identityhttp.New(h.svc, logger, identityhttp.Options{AllowedOrigins: cfg.HTTP.CORSAllowedOrigins})
	geo := geoapp.NewService(h.geo, h.svc, h.geo, h.clock)
	subs := subapp.NewService(h.subs, h.svc, h.clock)
	biz := bizapp.NewService(bizapp.Deps{Store: h.biz, Authz: h.svc, Members: h.svc, Owners: h.svc, Trials: subs, Summaries: subs, Entitlements: subs, Places: geo, Clock: h.clock})
	profs := profapp.NewService(proftest.NewStore(), h.svc, h.clock)
	h.srv = httptest.NewServer(app.NewHTTPHandler(cfg, logger, app.Deps{Modules: []app.RouteRegistrar{
		idh, geographyhttp.New(geo, logger, idh.RequireAuth), businesseshttp.New(biz, logger, idh.RequireAuth, nil),
		subscriptionshttp.New(subs, logger, idh.RequireAuth),
		professionalshttp.New(profs, logger, idh.RequireAuth),
		cataloghttp.New(catapp.NewService(catalogtest.NewStore(), h.svc, biz, profs, h.clock), logger, idh.RequireAuth),
	}}))
	t.Cleanup(h.srv.Close)
	return h
}

type reply struct {
	status int
	header http.Header
	body   string
}

func (h authHarness) call(t *testing.T, method, path, body string, headers map[string]string, cookies ...*http.Cookie) reply {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, h.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return reply{resp.StatusCode, resp.Header, string(b)}
}

var linkToken = regexp.MustCompile(`token=([^\s]+)`)

func (h authHarness) lastLinkToken(t *testing.T) string {
	t.Helper()
	m := linkToken.FindStringSubmatch(h.mailer.Last().Body)
	if m == nil {
		t.Fatal("no token link in last email")
	}
	raw, _ := url.QueryUnescape(m[1])
	return raw
}

// account registers and verifies email, returning the verification token already used.
func (h authHarness) account(t *testing.T, email string) string {
	t.Helper()
	h.call(t, http.MethodPost, "/api/v1/auth/register", `{"email":"`+email+`","password":"`+suitePassword+`","fullName":"Ana Pérez"}`, nil)
	if err := h.svc.SendVerificationEmail(context.Background(), h.store.Enqueued[len(h.store.Enqueued)-1]); err != nil {
		t.Fatal(err)
	}
	tok := h.lastLinkToken(t)
	if r := h.call(t, http.MethodPost, "/api/v1/auth/verify-email", `{"token":"`+tok+`"}`, nil); r.status != http.StatusOK {
		t.Fatalf("verify: %d %s", r.status, r.body)
	}
	return tok
}

func (h authHarness) login(t *testing.T, email, client string) reply {
	t.Helper()
	r := h.call(t, http.MethodPost, "/api/v1/auth/login", `{"email":"`+email+`","password":"`+suitePassword+`","clientType":"`+client+`"}`, nil)
	if r.status != http.StatusOK {
		t.Fatalf("login %s: %d %s", client, r.status, r.body)
	}
	return r
}

func (h authHarness) bearerToken(t *testing.T, email string) string {
	t.Helper()
	var body struct{ Token string }
	_ = json.Unmarshal([]byte(h.login(t, email, "MOBILE").body), &body)
	return body.Token
}

func bearerH(tok string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + tok}
}

// authenticatedRoutes are every endpoint behind RequireAuth.
var authenticatedRoutes = []struct{ method, path, body string }{
	{http.MethodGet, "/api/v1/me", ""},
	{http.MethodGet, "/api/v1/me/memberships", ""},
	{http.MethodPost, "/api/v1/me/password", `{"currentPassword":"x","newPassword":"otra frase nueva bastante larga"}`},
	{http.MethodPost, "/api/v1/auth/logout", ""},
	{http.MethodPost, "/api/v1/auth/logout-all", ""},
}

// geographyAdminRoutes are the PLATFORM_ADMIN endpoints of the geography module (T4.1).
var geographyAdminRoutes = []struct{ method, path, body string }{
	{http.MethodPost, "/api/v1/admin/countries", `{"code":"MX","name":"México","phoneCode":"+52","defaultCurrency":"USD"}`},
	{http.MethodPatch, "/api/v1/admin/countries/CO", `{"active":false}`},
	{http.MethodPost, "/api/v1/admin/divisions", `{"countryCode":"CO","name":"Cauca","type":"DEPARTMENT"}`},
	{http.MethodPatch, "/api/v1/admin/divisions/" + geotest.ValleID.String(), `{"active":false}`},
	{http.MethodPost, "/api/v1/admin/cities", `{"divisionId":"` + geotest.ValleID.String() + `","name":"Palmira","timeZone":"America/Bogota"}`},
	{http.MethodPatch, "/api/v1/admin/cities/" + geotest.CaliID.String(), `{"active":false}`},
}

// businessAdminRoutes are the PLATFORM_ADMIN endpoints of the businesses module (T4.3).
var businessAdminRoutes = []struct{ method, path, body string }{
	{http.MethodPost, "/api/v1/admin/businesses", `{"name":"Barbería","contactPhone":"3001234567","cityId":"` + geotest.CaliID.String() + `","addressLine":"Calle 1","ownerEmail":"ana@example.com"}`},
	{http.MethodGet, "/api/v1/admin/businesses", ""},
	{http.MethodGet, "/api/v1/admin/businesses/01a20aae-7e5d-73d7-b7d3-a11c5afb9655", ""},
	{http.MethodPost, "/api/v1/admin/businesses/01a20aae-7e5d-73d7-b7d3-a11c5afb9655/activate", ""},
	{http.MethodPost, "/api/v1/admin/businesses/01a20aae-7e5d-73d7-b7d3-a11c5afb9655/suspend", ""},
	{http.MethodGet, "/api/v1/admin/businesses/01a20aae-7e5d-73d7-b7d3-a11c5afb9655/subscription", ""},
	{http.MethodPost, "/api/v1/admin/businesses/01a20aae-7e5d-73d7-b7d3-a11c5afb9655/subscription/extend-trial", `{"days":30}`},
	{http.MethodPost, "/api/v1/admin/businesses/01a20aae-7e5d-73d7-b7d3-a11c5afb9655/subscription/activate", ""},
}

func init() {
	authenticatedRoutes = append(authenticatedRoutes, geographyAdminRoutes...)
	authenticatedRoutes = append(authenticatedRoutes, businessAdminRoutes...)
	authenticatedRoutes = append(authenticatedRoutes,
		struct{ method, path, body string }{http.MethodGet, "/api/v1/business/01a20aae-7e5d-73d7-b7d3-a11c5afb9655", ""},
		struct{ method, path, body string }{http.MethodPatch, "/api/v1/business/01a20aae-7e5d-73d7-b7d3-a11c5afb9655", `{"name":"Nuevo"}`},
		struct{ method, path, body string }{http.MethodGet, "/api/v1/business/01a20aae-7e5d-73d7-b7d3-a11c5afb9655/members", ""},
		struct{ method, path, body string }{http.MethodPost, "/api/v1/business/01a20aae-7e5d-73d7-b7d3-a11c5afb9655/members", `{"email":"x@example.com","role":"PROFESSIONAL"}`},
		struct{ method, path, body string }{http.MethodPatch, "/api/v1/business/01a20aae-7e5d-73d7-b7d3-a11c5afb9655/members/01a20aae-7e5d-73d7-b7d3-a11c5afb9655", `{"role":"PROFESSIONAL"}`},
		struct{ method, path, body string }{http.MethodDelete, "/api/v1/business/01a20aae-7e5d-73d7-b7d3-a11c5afb9655/members/01a20aae-7e5d-73d7-b7d3-a11c5afb9655", ""},
	)
	authenticatedRoutes = append(authenticatedRoutes, struct{ method, path, body string }{
		http.MethodPost, "/api/v1/businesses", `{"name":"Mía","contactPhone":"3001234567","cityId":"` + geotest.CaliID.String() + `","addressLine":"Calle 1"}`,
	})
}

// TestAuthenticatedRoutesRejectInvalidCredentials: route × credential matrix. Every case is
// 401 UNAUTHENTICATED with WWW-Authenticate and no-store.
func TestAuthenticatedRoutesRejectInvalidCredentials(t *testing.T) {
	h := startAuth(t)
	verifyTok := h.account(t, suiteEmail)
	h.account(t, "bob@example.com")
	revoked := h.bearerToken(t, suiteEmail)
	h.call(t, http.MethodPost, "/api/v1/auth/logout", "", bearerH(revoked))
	suspended := h.bearerToken(t, "bob@example.com")
	h.store.User("bob@example.com").Suspend(h.clock.Now())
	h.call(t, http.MethodPost, "/api/v1/auth/password/forgot", `{"email":"`+suiteEmail+`"}`, nil)
	if err := h.svc.SendPasswordResetEmail(context.Background(), h.store.ResetEnqueued[0]); err != nil {
		t.Fatal(err)
	}
	resetTok := h.lastLinkToken(t)
	expiring := h.bearerToken(t, suiteEmail)

	creds := map[string]map[string]string{
		"none":                    nil,
		"empty bearer":            {"Authorization": "Bearer "},
		"basic":                   {"Authorization": "Basic YW5hOnB3"},
		"garbage":                 bearerH("garbage"),
		"verification token":      bearerH(verifyTok),
		"reset token":             bearerH(resetTok),
		"forged session token":    bearerH("bp_s_" + strings.Repeat("A", 43)),
		"truncated session token": bearerH(expiring[:len(expiring)-1]),
		"revoked session":         bearerH(revoked),
		"suspended user":          bearerH(suspended),
		"garbage session cookie":  {"Cookie": identityhttp.SessionCookie + "=garbage", "Origin": allowedOrigin},
	}
	check := func(name string, hdr map[string]string) {
		for _, rt := range authenticatedRoutes {
			r := h.call(t, rt.method, rt.path, rt.body, hdr)
			if r.status != http.StatusUnauthorized || r.header.Get("WWW-Authenticate") == "" ||
				!strings.Contains(r.body, `"code":"UNAUTHENTICATED"`) || r.header.Get("Cache-Control") != "no-store" {
				t.Errorf("%s %s with %s: %d %s", rt.method, rt.path, name, r.status, r.body)
			}
		}
	}
	for name, hdr := range creds {
		check(name, hdr)
	}
	// Idle expiry of a mobile session (30 days without activity).
	h.clock.Advance(31 * 24 * time.Hour)
	check("expired session", bearerH(expiring))
}

// TestSessionTokensAreFreshAndOpaque: every login issues a new random token (no fixation:
// a client-chosen cookie value is never adopted), and a token stops working after logout.
func TestSessionTokensAreFreshAndOpaque(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	seen := map[string]bool{}
	for range 5 {
		tok := h.bearerToken(t, suiteEmail)
		if !regexp.MustCompile(`^bp_s_[A-Za-z0-9_-]{43}$`).MatchString(tok) || seen[tok] {
			t.Fatalf("token %q malformed or repeated", tok)
		}
		seen[tok] = true
	}
	planted := &http.Cookie{Name: identityhttp.SessionCookie, Value: "bp_s_" + strings.Repeat("P", 43), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
	r := h.call(t, http.MethodPost, "/api/v1/auth/login", `{"email":"`+suiteEmail+`","password":"`+suitePassword+`","clientType":"WEB"}`, nil, planted)
	var issued string
	for _, c := range (&http.Response{Header: r.header}).Cookies() {
		if c.Name == identityhttp.SessionCookie {
			issued = c.Value
		}
	}
	if issued == "" || issued == planted.Value {
		t.Fatalf("session fixation: issued %q", issued)
	}
	if r := h.call(t, http.MethodGet, "/api/v1/me", "", nil, planted); r.status != http.StatusUnauthorized {
		t.Fatalf("planted cookie accepted: %d", r.status)
	}
}

// TestWebSessionCookieIsHardened checks every attribute of the web session cookie.
func TestWebSessionCookieIsHardened(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	r := h.login(t, suiteEmail, "WEB")
	raw := r.header.Get("Set-Cookie")
	for _, attr := range []string{identityhttp.SessionCookie + "=bp_s_", "Path=/", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(raw, attr) {
			t.Errorf("Set-Cookie %q lacks %q", raw, attr)
		}
	}
	if strings.Contains(strings.ToLower(raw), "domain=") || !strings.HasPrefix(identityhttp.SessionCookie, "__Host-") {
		t.Errorf("cookie must be host-only: %q", raw)
	}
	if strings.Contains(r.body, "bp_s_") {
		t.Errorf("web token leaked in body: %s", r.body)
	}
}

// TestCookieSessionsRequireAllowedOrigin: CSRF defense for cookie-authenticated writes.
func TestCookieSessionsRequireAllowedOrigin(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	var cookie *http.Cookie
	for _, c := range (&http.Response{Header: h.login(t, suiteEmail, "WEB").header}).Cookies() {
		cookie = c
	}
	for name, origin := range map[string]string{"missing": "", "foreign": "https://evil.example", "look-alike": allowedOrigin + ".evil.example", "http": strings.Replace(allowedOrigin, "https", "http", 1)} {
		hdr := map[string]string{}
		if origin != "" {
			hdr["Origin"] = origin
		}
		if r := h.call(t, http.MethodPost, "/api/v1/auth/logout-all", "", hdr, cookie); r.status != http.StatusForbidden || !strings.Contains(r.body, "CSRF_REJECTED") {
			t.Errorf("origin %s: %d %s", name, r.status, r.body)
		}
	}
	if r := h.call(t, http.MethodGet, "/api/v1/me", "", nil, cookie); r.status != http.StatusOK {
		t.Errorf("safe method with cookie: %d", r.status)
	}
	if r := h.call(t, http.MethodPost, "/api/v1/auth/logout-all", "", map[string]string{"Origin": allowedOrigin}, cookie); r.status != http.StatusOK {
		t.Errorf("allowed origin: %d %s", r.status, r.body)
	}
}

// TestMassAssignmentIsRejected: privileged or server-owned fields are unknown fields → 400.
func TestMassAssignmentIsRejected(t *testing.T) {
	h := startAuth(t)
	for _, extra := range []string{`"isPlatformAdmin":true`, `"status":"ACTIVE"`, `"emailVerifiedAt":"2026-01-01T00:00:00Z"`, `"id":"0193f000-0000-7000-8000-000000000001"`} {
		body := `{"email":"x@example.com","password":"` + suitePassword + `","fullName":"X",` + extra + `}`
		if r := h.call(t, http.MethodPost, "/api/v1/auth/register", body, nil); r.status != http.StatusBadRequest {
			t.Errorf("register with %s: %d", extra, r.status)
		}
	}
	if h.store.UserCount() != 0 {
		t.Fatal("an account was created through mass assignment")
	}
	if r := h.call(t, http.MethodPost, "/api/v1/auth/login", `{"email":"x@example.com","password":"x","clientType":"MOBILE","admin":true}`, nil); r.status != http.StatusBadRequest {
		t.Errorf("login with extra field: %d", r.status)
	}
}

// TestPublicEndpointsDoNotRevealAccounts compares responses for a registered and an unknown
// email on every public endpoint that takes an email.
func TestPublicEndpointsDoNotRevealAccounts(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	h.call(t, http.MethodPost, "/api/v1/auth/register", `{"email":"pendiente@example.com","password":"`+suitePassword+`","fullName":"P"}`, nil)
	cases := []struct{ path, body string }{
		{"/api/v1/auth/register", `{"email":"%s","password":"` + suitePassword + `","fullName":"Otro"}`},
		{"/api/v1/auth/verify-email/resend", `{"email":"%s"}`},
		{"/api/v1/auth/password/forgot", `{"email":"%s"}`},
		{"/api/v1/auth/login", `{"email":"%s","password":"clave equivocada 123","clientType":"MOBILE"}`},
	}
	for _, c := range cases {
		var ref string
		for _, email := range []string{suiteEmail, "pendiente@example.com", "nadie@example.com"} {
			r := h.call(t, http.MethodPost, c.path, strings.Replace(c.body, "%s", email, 1), nil)
			got := normalizeReply(t, r)
			if ref == "" {
				ref = got
			} else if got != ref {
				t.Errorf("%s reveals %s:\n%s\n%s", c.path, email, got, ref)
			}
		}
	}
}

// normalizeReply keeps status, body (without requestId/instance) and the headers that matter.
func normalizeReply(t *testing.T, r reply) string {
	t.Helper()
	body := r.body
	var m map[string]any
	if json.Unmarshal([]byte(r.body), &m) == nil {
		delete(m, "requestId")
		delete(m, "instance")
		b, _ := json.Marshal(m)
		body = string(b)
	}
	return strings.Join([]string{http.StatusText(r.status), r.header.Get("Content-Type"), r.header.Get("Set-Cookie"), r.header.Get("WWW-Authenticate"), body}, "|")
}

// TestTokensAreBoundToTheirPurpose: verification, reset and session tokens are not
// interchangeable.
func TestTokensAreBoundToTheirPurpose(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	session := h.bearerToken(t, suiteEmail)
	h.call(t, http.MethodPost, "/api/v1/auth/password/forgot", `{"email":"`+suiteEmail+`"}`, nil)
	if err := h.svc.SendPasswordResetEmail(context.Background(), h.store.ResetEnqueued[0]); err != nil {
		t.Fatal(err)
	}
	reset := h.lastLinkToken(t)
	h.call(t, http.MethodPost, "/api/v1/auth/register", `{"email":"nuevo@example.com","password":"`+suitePassword+`","fullName":"N"}`, nil)
	if err := h.svc.SendVerificationEmail(context.Background(), h.store.Enqueued[len(h.store.Enqueued)-1]); err != nil {
		t.Fatal(err)
	}
	verify := h.lastLinkToken(t)

	for name, c := range map[string]struct{ path, token string }{
		"reset token to verify":   {"/api/v1/auth/verify-email", reset},
		"session token to verify": {"/api/v1/auth/verify-email", session},
		"verify token to reset":   {"/api/v1/auth/password/reset", verify},
		"session token to reset":  {"/api/v1/auth/password/reset", session},
	} {
		body := `{"token":"` + c.token + `"}`
		if strings.HasSuffix(c.path, "/reset") {
			body = `{"token":"` + c.token + `","newPassword":"otra frase nueva bastante larga"}`
		}
		if r := h.call(t, http.MethodPost, c.path, body, nil); r.status != http.StatusBadRequest || !strings.Contains(r.body, "INVALID_TOKEN") {
			t.Errorf("%s: %d %s", name, r.status, r.body)
		}
	}
	// The legitimate tokens were not burned by the misuse.
	if r := h.call(t, http.MethodPost, "/api/v1/auth/verify-email", `{"token":"`+verify+`"}`, nil); r.status != http.StatusOK {
		t.Errorf("verify after misuse: %d", r.status)
	}
	if r := h.call(t, http.MethodPost, "/api/v1/auth/password/reset", `{"token":"`+reset+`","newPassword":"otra frase nueva bastante larga"}`, nil); r.status != http.StatusOK {
		t.Errorf("reset after misuse: %d", r.status)
	}
}

// TestSecretsNeverReachLogsOrAudit runs every flow and scans logs and audit entries.
func TestSecretsNeverReachLogsOrAudit(t *testing.T) {
	h := startAuth(t)
	verifyTok := h.account(t, suiteEmail)
	session := h.bearerToken(t, suiteEmail)
	cookieReply := h.login(t, suiteEmail, "WEB")
	h.call(t, http.MethodPost, "/api/v1/auth/login", `{"email":"`+suiteEmail+`","password":"intento fallido 999","clientType":"MOBILE"}`, nil)
	h.call(t, http.MethodPost, "/api/v1/me/password", `{"currentPassword":"`+suitePassword+`","newPassword":"otra frase nueva bastante larga"}`, bearerH(session))
	h.call(t, http.MethodPost, "/api/v1/auth/password/forgot", `{"email":"`+suiteEmail+`"}`, nil)
	if err := h.svc.SendPasswordResetEmail(context.Background(), h.store.ResetEnqueued[0]); err != nil {
		t.Fatal(err)
	}
	resetTok := h.lastLinkToken(t)
	h.call(t, http.MethodPost, "/api/v1/auth/password/reset", `{"token":"`+resetTok+`","newPassword":"una tercera clave larguisima"}`, nil)

	var webTok string
	for _, c := range (&http.Response{Header: cookieReply.header}).Cookies() {
		webTok = c.Value
	}
	audit, _ := json.Marshal(h.store.Audit)
	secrets := []string{suitePassword, "intento fallido 999", "otra frase nueva bastante larga", "una tercera clave larguisima",
		strings.TrimPrefix(verifyTok, "bp_ev_"), strings.TrimPrefix(resetTok, "bp_pr_"), strings.TrimPrefix(session, "bp_s_"), strings.TrimPrefix(webTok, "bp_s_")}
	for _, s := range secrets {
		if strings.Contains(h.logs.String(), s) {
			t.Errorf("secret %q in logs", s)
		}
		if strings.Contains(string(audit), s) {
			t.Errorf("secret %q in audit", s)
		}
	}
	if strings.Contains(string(audit), suiteEmail) {
		t.Error("email in audit")
	}
	if len(h.store.Audit) < 7 {
		t.Errorf("expected the flow to be audited, got %v", h.store.AuditActions())
	}
}
