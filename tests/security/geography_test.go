package security_test

// Phase 4 (T4.1) security checks for the geography module: PLATFORM_ADMIN-only writes,
// immediate revocation, mass assignment and path handling, on the composed handler.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
)

// TestGeographyWritesRequirePlatformAdmin: a verified, authenticated non-admin gets 403 on every
// admin route, nothing changes, and each attempt is audited as authz.denied.
func TestGeographyWritesRequirePlatformAdmin(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	tok := h.bearerToken(t, suiteEmail)
	before := len(h.store.AuditActions())

	for _, rt := range geographyAdminRoutes {
		r := h.call(t, rt.method, rt.path, rt.body, bearerH(tok))
		if r.status != http.StatusForbidden || !strings.Contains(r.body, `"FORBIDDEN"`) {
			t.Errorf("%s %s as non-admin: %d %s", rt.method, rt.path, r.status, r.body)
		}
	}
	if len(h.geo.Audit) != 0 {
		t.Errorf("geography changed by non-admin: %v", h.geo.AuditActions())
	}
	denied := 0
	for _, a := range h.store.AuditActions()[before:] {
		if a == application.AuditAuthzDenied+":DENIED" {
			denied++
		}
	}
	if denied != len(geographyAdminRoutes) {
		t.Errorf("authz.denied entries = %d, want %d", denied, len(geographyAdminRoutes))
	}
	// Public reads are untouched by the attempts.
	if r := h.call(t, http.MethodGet, "/api/v1/countries", "", nil); r.status != http.StatusOK || !strings.Contains(r.body, `"Colombia"`) {
		t.Errorf("countries after denied writes: %d %s", r.status, r.body)
	}
}

// TestPlatformAdminRevocationIsImmediate: the flag is read per request, so a revoked admin's
// existing session loses access on the next call.
func TestPlatformAdminRevocationIsImmediate(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	tok := h.bearerToken(t, suiteEmail)
	ctx := context.Background()
	if err := h.svc.SetPlatformAdmin(ctx, suiteEmail, true); err != nil {
		t.Fatal(err)
	}
	create := func(name string) reply {
		return h.call(t, http.MethodPost, "/api/v1/admin/countries",
			`{"code":"`+name+`","name":"Test","phoneCode":"+1","defaultCurrency":"USD"}`, bearerH(tok))
	}
	if r := create("US"); r.status != http.StatusCreated {
		t.Fatalf("admin create: %d %s", r.status, r.body)
	}
	if err := h.svc.SetPlatformAdmin(ctx, suiteEmail, false); err != nil {
		t.Fatal(err)
	}
	if r := create("CA"); r.status != http.StatusForbidden {
		t.Errorf("after revocation: %d %s (same session must lose access)", r.status, r.body)
	}
}

// TestGeographyRejectsMassAssignmentAndBadPaths: unknown fields (id, timestamps, foreign keys
// that cannot change) are rejected; malformed or lower-case path parameters are a plain 404.
func TestGeographyRejectsMassAssignmentAndBadPaths(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	tok := h.bearerToken(t, suiteEmail)
	if err := h.svc.SetPlatformAdmin(context.Background(), suiteEmail, true); err != nil {
		t.Fatal(err)
	}
	for _, rt := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/admin/countries", `{"code":"MX","name":"México","phoneCode":"+52","defaultCurrency":"USD","createdAt":"2020-01-01T00:00:00Z"}`},
		{http.MethodPatch, "/api/v1/admin/countries/CO", `{"code":"MX"}`},
		{http.MethodPost, "/api/v1/admin/divisions", `{"id":"01a20aae-7e56-796c-9de9-5409ce51e7d4","countryCode":"CO","name":"Cauca","type":"DEPARTMENT"}`},
		{http.MethodPatch, "/api/v1/admin/divisions/01a20aae-7e56-796c-9de9-5409ce51e7d4", `{"countryCode":"MX"}`},
		{http.MethodPatch, "/api/v1/admin/cities/01a20aae-7e5d-73d7-b7d3-a11c5afb9655", `{"divisionId":"01a20aae-7e5a-7f3e-953c-9ec690399d8d"}`},
	} {
		if r := h.call(t, rt.method, rt.path, rt.body, bearerH(tok)); r.status != http.StatusBadRequest {
			t.Errorf("%s %s with a non-writable field: %d %s", rt.method, rt.path, r.status, r.body)
		}
	}
	if len(h.geo.Audit) != 0 {
		t.Errorf("rejected requests changed data: %v", h.geo.AuditActions())
	}

	notFound := h.call(t, http.MethodGet, "/api/v1/nope", "", nil)
	for _, path := range []string{
		"/api/v1/countries/co/divisions",
		"/api/v1/countries/COL/divisions",
		"/api/v1/countries/%27%20OR%201=1--/divisions",
		"/api/v1/divisions/not-a-uuid/cities",
		"/api/v1/divisions/00000000-0000-0000-0000-000000000000/cities",
	} {
		r := h.call(t, http.MethodGet, path, "", nil)
		if r.status != http.StatusNotFound || normalizeReply(t, r) != normalizeReply(t, notFound) {
			t.Errorf("GET %s: %d %s, want the generic 404", path, r.status, r.body)
		}
	}
	// Public reads expose only active records and no internal fields.
	r := h.call(t, http.MethodGet, "/api/v1/countries", "", nil)
	var list struct{ Items []map[string]any }
	if err := json.Unmarshal([]byte(r.body), &list); err != nil || len(list.Items) != 1 {
		t.Fatalf("countries: %s", r.body)
	}
	for k := range list.Items[0] {
		switch k {
		case "code", "name", "phoneCode", "defaultCurrency", "active":
		default:
			t.Errorf("unexpected field %q in public country", k)
		}
	}
}
