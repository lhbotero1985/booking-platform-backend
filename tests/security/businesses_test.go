package security_test

// Phase 4 (T4.3) security checks for business onboarding: PLATFORM_ADMIN only, no mass
// assignment of server-controlled fields, the OWNER cannot be an unverified account.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
)

func TestBusinessOnboardingRequiresPlatformAdmin(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	tok := h.bearerToken(t, suiteEmail)
	before := len(h.store.AuditActions())
	for _, rt := range businessAdminRoutes {
		if r := h.call(t, rt.method, rt.path, rt.body, bearerH(tok)); r.status != http.StatusForbidden {
			t.Errorf("%s %s as non-admin: %d %s", rt.method, rt.path, r.status, r.body)
		}
	}
	if h.biz.Count() != 0 || len(h.biz.Audit) != 0 {
		t.Errorf("non-admin changed businesses: %d %v", h.biz.Count(), h.biz.AuditActions())
	}
	denied := 0
	for _, a := range h.store.AuditActions()[before:] {
		if a == application.AuditAuthzDenied+":DENIED" {
			denied++
		}
	}
	if denied != len(businessAdminRoutes) {
		t.Errorf("authz.denied = %d, want %d", denied, len(businessAdminRoutes))
	}
}

func TestBusinessCreationRejectsServerControlledFields(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	tok := h.bearerToken(t, suiteEmail)
	if err := h.svc.SetPlatformAdmin(context.Background(), suiteEmail, true); err != nil {
		t.Fatal(err)
	}
	base := `"name":"Barbería","contactPhone":"3001234567","cityId":"` + geotest.CaliID.String() + `","addressLine":"Calle 1","ownerEmail":"` + suiteEmail + `"`
	for _, extra := range []string{
		`"status":"ACTIVE"`, `"slug":"admin"`, `"id":"01a20aae-7e5d-73d7-b7d3-a11c5afb9655"`, `"timeZone":"UTC"`,
		`"currency":"USD"`, `"countryCode":"US"`, `"divisionId":"01a20aae-7e5a-7f3e-953c-9ec690399d8d"`,
		`"bookingPolicy":{"slotIntervalMinutes":5}`, `"createdAt":"2020-01-01T00:00:00Z"`,
	} {
		r := h.call(t, http.MethodPost, "/api/v1/admin/businesses", "{"+base+","+extra+"}", bearerH(tok))
		if r.status != http.StatusBadRequest {
			t.Errorf("extra field %s: %d %s", extra, r.status, r.body)
		}
	}
	if h.biz.Count() != 0 {
		t.Fatal("a request with server-controlled fields created a business")
	}
	// The accepted request ignores nothing silently: status and slug are decided by the server.
	r := h.call(t, http.MethodPost, "/api/v1/admin/businesses", "{"+base+"}", bearerH(tok))
	var b struct{ Status, Slug, TimeZone, Currency string }
	if r.status != http.StatusCreated || json.Unmarshal([]byte(r.body), &b) != nil ||
		b.Status != "PENDING_REVIEW" || b.Slug != "barberia" || b.TimeZone != "America/Bogota" || b.Currency != "COP" {
		t.Errorf("create: %d %s", r.status, r.body)
	}

	// An unverified account cannot become OWNER (no business is created).
	h.call(t, http.MethodPost, "/api/v1/auth/register", `{"email":"pending@example.com","password":"`+suitePassword+`","fullName":"P"}`, nil)
	r = h.call(t, http.MethodPost, "/api/v1/admin/businesses", `{"name":"Otra","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1","ownerEmail":"pending@example.com"}`, bearerH(tok))
	if r.status != http.StatusBadRequest || !strings.Contains(r.body, "ACCOUNT_NOT_ELIGIBLE") || h.biz.Count() != 1 {
		t.Errorf("unverified owner: %d %s (businesses %d)", r.status, r.body, h.biz.Count())
	}
}
