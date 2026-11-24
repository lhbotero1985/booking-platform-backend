package security_test

// Phase 4 (T4.3) security checks for business onboarding: PLATFORM_ADMIN only, no mass
// assignment of server-controlled fields, the OWNER cannot be an unverified account.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
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
		b.Status != "ACTIVE" || b.Slug != "barberia" || b.TimeZone != "America/Bogota" || b.Currency != "COP" {
		t.Errorf("create: %d %s", r.status, r.body)
	}

	// An unverified account cannot become OWNER (no business is created).
	h.call(t, http.MethodPost, "/api/v1/auth/register", `{"email":"pending@example.com","password":"`+suitePassword+`","fullName":"P"}`, nil)
	r = h.call(t, http.MethodPost, "/api/v1/admin/businesses", `{"name":"Otra","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1","ownerEmail":"pending@example.com"}`, bearerH(tok))
	if r.status != http.StatusBadRequest || !strings.Contains(r.body, "ACCOUNT_NOT_ELIGIBLE") || h.biz.Count() != 1 {
		t.Errorf("unverified owner: %d %s (businesses %d)", r.status, r.body, h.biz.Count())
	}
}

// TestSelfServiceOnboardingRules (ADR-0018): the caller becomes OWNER of an ACTIVE business with a
// demo; it cannot pick another owner, status, slug or subscription; one business per account.
func TestSelfServiceOnboardingRules(t *testing.T) {
	h := startAuth(t)
	h.account(t, suiteEmail)
	tok := h.bearerToken(t, suiteEmail)
	base := `"name":"Barbería Propia","contactPhone":"3001234567","cityId":"` + geotest.CaliID.String() + `","addressLine":"Calle 1"`
	for _, extra := range []string{
		`"ownerEmail":"otro@example.com"`, `"status":"ACTIVE"`, `"slug":"admin"`, `"subscription":{"status":"ACTIVE"}`,
		`"trialEndsAt":"2099-01-01T00:00:00Z"`, `"currency":"USD"`,
	} {
		if r := h.call(t, http.MethodPost, "/api/v1/businesses", "{"+base+","+extra+"}", bearerH(tok)); r.status != http.StatusBadRequest {
			t.Errorf("extra field %s: %d %s", extra, r.status, r.body)
		}
	}
	if h.biz.Count() != 0 {
		t.Fatal("a request with server-controlled fields created a business")
	}
	r := h.call(t, http.MethodPost, "/api/v1/businesses", "{"+base+"}", bearerH(tok))
	var b struct{ ID, Status string }
	if r.status != http.StatusCreated || json.Unmarshal([]byte(r.body), &b) != nil || b.Status != "ACTIVE" {
		t.Fatalf("create: %d %s", r.status, r.body)
	}
	// The caller is OWNER of exactly that business, and the demo runs.
	ms := h.call(t, http.MethodGet, "/api/v1/me/memberships", "", bearerH(tok))
	if !strings.Contains(ms.body, `"businessId":"`+b.ID+`","role":"OWNER"`) {
		t.Errorf("memberships = %s", ms.body)
	}
	if got := h.subs.AuditActions(); len(got) != 1 || got[0] != "subscription.trial_started" {
		t.Errorf("subscription audit = %v", got)
	}
	// One business per account.
	r = h.call(t, http.MethodPost, "/api/v1/businesses", "{"+base+"}", bearerH(tok))
	if r.status != http.StatusConflict || !strings.Contains(r.body, "ALREADY_OWNER") || h.biz.Count() != 1 {
		t.Errorf("second business: %d %s (businesses %d)", r.status, r.body, h.biz.Count())
	}
}

// TestBusinessManagementIsTenantScoped (T4.4, ADR-0007): another business is a 404 byte-for-byte
// identical to a business that does not exist; a PROFESSIONAL reads but cannot edit; non-editable
// fields are rejected; PLATFORM_ADMIN can act on any business.
func TestBusinessManagementIsTenantScoped(t *testing.T) {
	h := startAuth(t)
	for _, email := range []string{suiteEmail, "bob@example.com", "gerente@example.com", "admin@example.com"} {
		h.account(t, email)
	}
	ana, bob, gerente, admin := h.bearerToken(t, suiteEmail), h.bearerToken(t, "bob@example.com"), h.bearerToken(t, "gerente@example.com"), h.bearerToken(t, "admin@example.com")
	if err := h.svc.SetPlatformAdmin(context.Background(), "admin@example.com", true); err != nil {
		t.Fatal(err)
	}
	create := func(tok, name string) string {
		r := h.call(t, http.MethodPost, "/api/v1/businesses", `{"name":"`+name+`","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, bearerH(tok))
		var b struct{ ID string }
		if r.status != http.StatusCreated || json.Unmarshal([]byte(r.body), &b) != nil {
			t.Fatalf("create %s: %d %s", name, r.status, r.body)
		}
		return b.ID
	}
	anaBiz, bobBiz := create(ana, "Barbería Ana"), create(bob, "Barbería Bob")
	adminP := h.store.User("admin@example.com").ID()
	if _, err := h.svc.AddMember(context.Background(), identityAdmin(adminP), uuidMust(anaBiz), "gerente@example.com", "PROFESSIONAL"); err != nil {
		t.Fatal(err)
	}

	// Cross-tenant: Ana on Bob's business == Ana on a business that does not exist.
	missing := "/api/v1/business/01a20aae-0000-7000-8000-000000000000"
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		body := ""
		if method == http.MethodPatch {
			body = `{"name":"Hackeada"}`
		}
		cross := h.call(t, method, "/api/v1/business/"+bobBiz, body, bearerH(ana))
		none := h.call(t, method, missing, body, bearerH(ana))
		if cross.status != http.StatusNotFound || normalizeReply(t, cross) != normalizeReply(t, none) {
			t.Errorf("%s other business: %d %s vs nonexistent %d %s", method, cross.status, cross.body, none.status, none.body)
		}
	}
	if b := h.call(t, http.MethodGet, "/api/v1/business/"+bobBiz, "", bearerH(bob)); strings.Contains(b.body, "Hackeada") {
		t.Error("Bob's business was modified by Ana")
	}

	// A PROFESSIONAL (barber) reads, cannot edit.
	if r := h.call(t, http.MethodGet, "/api/v1/business/"+anaBiz, "", bearerH(gerente)); r.status != http.StatusOK {
		t.Errorf("manager read: %d", r.status)
	}
	if r := h.call(t, http.MethodPatch, "/api/v1/business/"+anaBiz, `{"name":"Del Gerente"}`, bearerH(gerente)); r.status != http.StatusForbidden {
		t.Errorf("manager edit: %d %s", r.status, r.body)
	}
	// Non-editable fields.
	for _, f := range []string{`"slug":"otra"`, `"status":"SUSPENDED"`, `"currency":"USD"`, `"timeZone":"UTC"`, `"countryCode":"PE"`, `"id":"` + bobBiz + `"`} {
		if r := h.call(t, http.MethodPatch, "/api/v1/business/"+anaBiz, "{"+f+"}", bearerH(ana)); r.status != http.StatusBadRequest {
			t.Errorf("non-editable %s: %d %s", f, r.status, r.body)
		}
	}
	// PLATFORM_ADMIN acts on any business.
	if r := h.call(t, http.MethodPatch, "/api/v1/business/"+bobBiz, `{"description":"Revisado por soporte"}`, bearerH(admin)); r.status != http.StatusOK {
		t.Errorf("admin edit: %d %s", r.status, r.body)
	}
}

func identityAdmin(id uuid.UUID) identitycontract.Principal {
	return identitycontract.Principal{UserID: id, IsPlatformAdmin: true}
}

func uuidMust(s string) uuid.UUID { return uuid.MustParse(s) }

// TestMembersAreTenantScoped (T4.5): only OWNERs manage members; members of another business
// are invisible (404 identical to a missing business); a removed member loses access at once;
// there is always an OWNER.
func TestMembersAreTenantScoped(t *testing.T) {
	h := startAuth(t)
	for _, email := range []string{suiteEmail, "bob@example.com", "barbero@example.com"} {
		h.account(t, email)
	}
	ana, bob, barbero := h.bearerToken(t, suiteEmail), h.bearerToken(t, "bob@example.com"), h.bearerToken(t, "barbero@example.com")
	create := func(tok, name string) string {
		r := h.call(t, http.MethodPost, "/api/v1/businesses", `{"name":"`+name+`","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, bearerH(tok))
		var b struct{ ID string }
		if r.status != http.StatusCreated || json.Unmarshal([]byte(r.body), &b) != nil {
			t.Fatalf("create: %d %s", r.status, r.body)
		}
		return b.ID
	}
	anaBiz, bobBiz := create(ana, "Barbería Ana"), create(bob, "Barbería Bob")
	if r := h.call(t, http.MethodPost, "/api/v1/business/"+anaBiz+"/members", `{"email":"barbero@example.com","role":"PROFESSIONAL"}`, bearerH(ana)); r.status != http.StatusCreated {
		t.Fatalf("add barber: %d %s", r.status, r.body)
	}

	// Ana cannot see or touch Bob's members; same 404 as a business that does not exist.
	bobID := h.store.User("bob@example.com").ID().String()
	for _, rt := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/business/%s/members", ""},
		{http.MethodPost, "/api/v1/business/%s/members", `{"email":"barbero@example.com","role":"OWNER"}`},
		{http.MethodPatch, "/api/v1/business/%s/members/" + bobID, `{"role":"PROFESSIONAL"}`},
		{http.MethodDelete, "/api/v1/business/%s/members/" + bobID, ""},
	} {
		cross := h.call(t, rt.method, strings.Replace(rt.path, "%s", bobBiz, 1), rt.body, bearerH(ana))
		none := h.call(t, rt.method, strings.Replace(rt.path, "%s", "01a20aae-0000-7000-8000-000000000000", 1), rt.body, bearerH(ana))
		if cross.status != http.StatusNotFound || normalizeReply(t, cross) != normalizeReply(t, none) {
			t.Errorf("%s %s on another business: %d %s", rt.method, rt.path, cross.status, cross.body)
		}
	}
	// A PROFESSIONAL cannot manage members (not even promote itself).
	barberoID := h.store.User("barbero@example.com").ID().String()
	if r := h.call(t, http.MethodPatch, "/api/v1/business/"+anaBiz+"/members/"+barberoID, `{"role":"OWNER"}`, bearerH(barbero)); r.status != http.StatusForbidden {
		t.Errorf("self-promotion: %d %s", r.status, r.body)
	}
	// Removal is immediate: the same session loses access to the business.
	if r := h.call(t, http.MethodGet, "/api/v1/business/"+anaBiz, "", bearerH(barbero)); r.status != http.StatusOK {
		t.Fatalf("barber before removal: %d", r.status)
	}
	if r := h.call(t, http.MethodDelete, "/api/v1/business/"+anaBiz+"/members/"+barberoID, "", bearerH(ana)); r.status != http.StatusNoContent {
		t.Fatalf("remove: %d %s", r.status, r.body)
	}
	if r := h.call(t, http.MethodGet, "/api/v1/business/"+anaBiz, "", bearerH(barbero)); r.status != http.StatusNotFound {
		t.Errorf("barber after removal: %d", r.status)
	}
	// There is always an OWNER.
	anaID := h.store.User(suiteEmail).ID().String()
	if r := h.call(t, http.MethodDelete, "/api/v1/business/"+anaBiz+"/members/"+anaID, "", bearerH(ana)); r.status != http.StatusConflict || !strings.Contains(r.body, "LAST_OWNER") {
		t.Errorf("last owner leaves: %d %s", r.status, r.body)
	}
}

// TestPublicSearchExposesOnlyBookableBusinesses (T4.6): customers see only ACTIVE businesses
// with a valid demo/subscription, with no internal fields; hidden ones are a plain 404; the
// text filter cannot be used as a pattern.
func TestPublicSearchExposesOnlyBookableBusinesses(t *testing.T) {
	h := startAuth(t)
	for _, email := range []string{suiteEmail, "bob@example.com", "admin@example.com", "carla@example.com"} {
		h.account(t, email)
	}
	ana, bob, admin := h.bearerToken(t, suiteEmail), h.bearerToken(t, "bob@example.com"), h.bearerToken(t, "admin@example.com")
	carla := h.bearerToken(t, "carla@example.com")
	if err := h.svc.SetPlatformAdmin(context.Background(), "admin@example.com", true); err != nil {
		t.Fatal(err)
	}
	cali := geotest.CaliID.String()
	mk := func(tok, name string) string {
		r := h.call(t, http.MethodPost, "/api/v1/businesses", `{"name":"`+name+`","contactPhone":"3001234567","cityId":"`+cali+`","addressLine":"Calle 1"}`, bearerH(tok))
		var b struct{ ID string }
		if r.status != http.StatusCreated || json.Unmarshal([]byte(r.body), &b) != nil {
			t.Fatalf("create: %d %s", r.status, r.body)
		}
		return b.ID
	}
	post := func(tok, path, body string, want int) string {
		r := h.call(t, http.MethodPost, path, body, bearerH(tok))
		var v struct{ ID string }
		if r.status != want || json.Unmarshal([]byte(r.body), &v) != nil {
			t.Fatalf("POST %s: %d %s", path, r.status, r.body)
		}
		return v.ID
	}
	// offer creates a service and, if performed, a professional (the owner) who performs it.
	// It also gives the professional a Friday 09:00–10:00 schedule (public availability, T6.2).
	offer := func(tok, biz, email string, performed bool) (svc, prof string) {
		base := "/api/v1/business/" + biz
		svc = post(tok, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, http.StatusCreated)
		if !performed {
			return svc, ""
		}
		prof = post(tok, base+"/professionals", `{"email":"`+email+`","displayName":"Barbero"}`, http.StatusCreated)
		if r := h.call(t, http.MethodPut, base+"/professionals/"+prof+"/services", `{"serviceIds":["`+svc+`"]}`, bearerH(tok)); r.status != http.StatusOK {
			t.Fatalf("assign: %d %s", r.status, r.body)
		}
		if r := h.call(t, http.MethodPut, base+"/professionals/"+prof+"/schedule", `{"days":[{"weekday":"FRIDAY","workingHours":[{"start":"09:00","end":"10:00"}]}]}`, bearerH(tok)); r.status != http.StatusOK {
			t.Fatalf("schedule: %d %s", r.status, r.body)
		}
		return svc, prof
	}
	visBiz := mk(ana, "Barbería Visible")
	visSvc, visProf := offer(ana, visBiz, suiteEmail, true)
	offer(carla, mk(carla, "Barbería Sin Barberos"), "carla@example.com", false) // nothing bookable: hidden
	hidden := mk(bob, "Barbería Suspendida")
	hidSvc, hidProf := offer(bob, hidden, "bob@example.com", true)
	if r := h.call(t, http.MethodPost, "/api/v1/admin/businesses/"+hidden+"/suspend", "", bearerH(admin)); r.status != http.StatusOK {
		t.Fatalf("suspend: %d", r.status)
	}

	r := h.call(t, http.MethodGet, "/api/v1/businesses?cityId="+cali, "", nil)
	var page struct{ Items []map[string]any }
	if r.status != http.StatusOK || json.Unmarshal([]byte(r.body), &page) != nil || len(page.Items) != 1 || page.Items[0]["name"] != "Barbería Visible" {
		t.Fatalf("public list: %d %s", r.status, r.body)
	}
	for k := range page.Items[0] {
		switch k {
		case "id", "name", "slug", "description", "contactPhone", "location", "timeZone", "currency", "bookingPolicy":
		default:
			t.Errorf("internal field %q exposed publicly", k)
		}
	}
	// Public catalog: only public fields, and the same 404 for suspended and missing businesses.
	cat := h.call(t, http.MethodGet, "/api/v1/businesses/barberia-visible/catalog", "", nil)
	var pc struct {
		Services      []map[string]any
		Professionals []map[string]any
	}
	if cat.status != http.StatusOK || json.Unmarshal([]byte(cat.body), &pc) != nil || len(pc.Services) != 1 || len(pc.Professionals) != 1 {
		t.Fatalf("public catalog: %d %s", cat.status, cat.body)
	}
	for _, item := range append(pc.Services, pc.Professionals...) {
		for k := range item {
			switch k {
			case "id", "categoryId", "name", "description", "durationMinutes", "priceMinor", "currency", "professionalIds", "displayName", "bio", "serviceIds":
			default:
				t.Errorf("internal field %q exposed in the public catalog", k)
			}
		}
	}
	catMissing := h.call(t, http.MethodGet, "/api/v1/businesses/no-existe/catalog", "", nil)
	catSuspended := h.call(t, http.MethodGet, "/api/v1/businesses/barberia-suspendida/catalog", "", nil)
	if catSuspended.status != http.StatusNotFound || normalizeReply(t, catSuspended) != normalizeReply(t, catMissing) {
		t.Errorf("suspended business catalog: %d %s", catSuspended.status, catSuspended.body)
	}
	// Public availability (T6.2): only public fields; suspended business, and another business's
	// service and professional under a visible slug, are the same 404 as a missing business.
	avail := func(slug, svc, prof string) reply {
		return h.call(t, http.MethodGet, "/api/v1/businesses/"+slug+"/availability?serviceId="+svc+"&professionalId="+prof+"&date=2026-11-20", "", nil)
	}
	av := avail("barberia-visible", visSvc, visProf)
	var a map[string]any
	if av.status != http.StatusOK || json.Unmarshal([]byte(av.body), &a) != nil {
		t.Fatalf("public availability: %d %s", av.status, av.body)
	}
	for k := range a {
		switch k {
		case "date", "timeZone", "serviceId", "professionalId", "durationMinutes", "slots":
		default:
			t.Errorf("internal field %q exposed in the public availability", k)
		}
	}
	slots, _ := a["slots"].([]any)
	if len(slots) != 4 {
		t.Fatalf("slots = %s", av.body)
	}
	for k := range slots[0].(map[string]any) {
		if k != "startAt" && k != "endAt" && k != "startLocal" {
			t.Errorf("internal field %q exposed in a slot", k)
		}
	}
	avMissing := avail("no-existe", visSvc, visProf)
	for name, r := range map[string]reply{
		"suspended business":              avail("barberia-suspendida", hidSvc, hidProf),
		"another business's service/prof": avail("barberia-visible", hidSvc, hidProf),
		"another business's professional": avail("barberia-visible", visSvc, hidProf),
	} {
		if r.status != http.StatusNotFound || normalizeReply(t, r) != normalizeReply(t, avMissing) {
			t.Errorf("availability, %s: %d %s", name, r.status, r.body)
		}
	}
	// Booking (T6.3): a suspended business, or another business's service or professional under
	// a visible business, is the same 404 as a business that does not exist; nothing is booked.
	if r := h.call(t, http.MethodPut, "/api/v1/me/phone", `{"phone":"3001234567"}`, bearerH(carla)); r.status != http.StatusOK {
		t.Fatalf("phone: %d %s", r.status, r.body)
	}
	book := func(biz, svc, prof, key string) reply {
		hdr := bearerH(carla)
		hdr["Idempotency-Key"] = key
		return h.call(t, http.MethodPost, "/api/v1/appointments", `{"businessId":"`+biz+`","serviceId":"`+svc+`","professionalId":"`+prof+`","startAt":"2026-11-20T14:00:00Z"}`, hdr)
	}
	bookMissing := book("01a20aae-0000-7000-8000-000000000000", visSvc, visProf, "seguridad-0000")
	for name, r := range map[string]reply{
		"suspended business":              book(hidden, hidSvc, hidProf, "seguridad-0001"),
		"another business's service/prof": book(visBiz, hidSvc, hidProf, "seguridad-0002"),
		"another business's professional": book(visBiz, visSvc, hidProf, "seguridad-0003"),
		"another business's service":      book(visBiz, hidSvc, visProf, "seguridad-0004"),
	} {
		if r.status != http.StatusNotFound || normalizeReply(t, r) != normalizeReply(t, bookMissing) {
			t.Errorf("booking, %s: %d %s", name, r.status, r.body)
		}
	}
	if r := book(visBiz, visSvc, visProf, "seguridad-0005"); r.status != http.StatusCreated {
		t.Errorf("a valid booking: %d %s", r.status, r.body)
	}
	notFound := h.call(t, http.MethodGet, "/api/v1/businesses/no-existe", "", nil)
	suspended := h.call(t, http.MethodGet, "/api/v1/businesses/barberia-suspendida", "", nil)
	if suspended.status != http.StatusNotFound || normalizeReply(t, suspended) != normalizeReply(t, notFound) {
		t.Errorf("suspended business detail: %d %s", suspended.status, suspended.body)
	}
	for _, q := range []string{"%25%25", "_%25", "%27%20OR%201%3D1--"} {
		r := h.call(t, http.MethodGet, "/api/v1/businesses?cityId="+cali+"&q="+q, "", nil)
		if r.status != http.StatusOK || strings.Contains(r.body, "Visible") {
			t.Errorf("q=%s: %d %s (must match literally, nothing here)", q, r.status, r.body)
		}
	}
}
