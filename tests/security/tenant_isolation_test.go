package security_test

// Phase 4 closure (T4.7): tenant isolation over EVERY business-scoped operation of the contract.
// The routes are read from api/openapi.yaml, so an endpoint added later under
// /business/{businessId} is covered automatically.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pb33f/libopenapi"

	"github.com/lhbotero1985/booking-platform-backend/api"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
)

// tenantBodies are valid request bodies for the business-scoped write operations, so a 404
// cannot come from body validation. Unknown operations default to "{}" and still must be 404.
var tenantBodies = map[string]string{
	"PATCH /api/v1/business/{businessId}":                                                     `{"name":"Hackeada","description":"x"}`,
	"POST /api/v1/business/{businessId}/members":                                              `{"email":"intruso@example.com","role":"OWNER"}`,
	"PATCH /api/v1/business/{businessId}/members/{userId}":                                    `{"role":"OWNER"}`,
	"POST /api/v1/business/{businessId}/professionals":                                        `{"email":"intruso@example.com","displayName":"Intruso"}`,
	"PATCH /api/v1/business/{businessId}/professionals/{professionalId}":                      `{"displayName":"Hackeado"}`,
	"PUT /api/v1/business/{businessId}/professionals/{professionalId}/schedule":               `{"days":[{"weekday":"SUNDAY","workingHours":[{"start":"00:00","end":"23:00"}]}]}`,
	"POST /api/v1/business/{businessId}/professionals/{professionalId}/absences":              `{"start":"2030-01-01T00:00:00Z","end":"2030-01-02T00:00:00Z","reason":"Intruso"}`,
	"PATCH /api/v1/business/{businessId}/professionals/{professionalId}/absences/{absenceId}": `{"reason":"Hackeado"}`,
	"POST /api/v1/business/{businessId}/service-categories":                                   `{"name":"Intrusa"}`,
	"PATCH /api/v1/business/{businessId}/service-categories/{categoryId}":                     `{"name":"Hackeada"}`,
	"POST /api/v1/business/{businessId}/services":                                             `{"name":"Intruso","durationMinutes":30,"priceMinor":100}`,
	"PUT /api/v1/business/{businessId}/professionals/{professionalId}/services":               `{"serviceIds":[]}`,
	"PATCH /api/v1/business/{businessId}/services/{serviceId}":                                `{"priceMinor":100}`,
	"POST /api/v1/business/{businessId}/customers":                                            `{"fullName":"Intruso","phone":"3009990000"}`,
	"PATCH /api/v1/business/{businessId}/customers/{customerId}":                              `{"fullName":"Hackeado"}`,
}

func TestEveryBusinessRouteIsTenantIsolated(t *testing.T) {
	h := startAuth(t)
	for _, email := range []string{suiteEmail, "bob@example.com", "staff@example.com", "intruso@example.com"} {
		h.account(t, email)
	}
	ana, bob := h.bearerToken(t, suiteEmail), h.bearerToken(t, "bob@example.com")
	create := func(tok, name string) string {
		r := h.call(t, http.MethodPost, "/api/v1/businesses", `{"name":"`+name+`","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, bearerH(tok))
		var b struct{ ID string }
		if r.status != http.StatusCreated || json.Unmarshal([]byte(r.body), &b) != nil {
			t.Fatalf("create %s: %d %s", name, r.status, r.body)
		}
		return b.ID
	}
	anaBiz := create(ana, "Barbería Ana")
	bobBiz := create(bob, "Barbería Bob")
	if r := h.call(t, http.MethodPost, "/api/v1/business/"+bobBiz+"/members", `{"email":"staff@example.com","role":"PROFESSIONAL"}`, bearerH(bob)); r.status != http.StatusCreated {
		t.Fatalf("bob adds staff: %d %s", r.status, r.body)
	}
	staffID := h.store.User("staff@example.com").ID().String()
	pr := h.call(t, http.MethodPost, "/api/v1/business/"+bobBiz+"/professionals", `{"email":"staff@example.com","displayName":"Staff de Bob"}`, bearerH(bob))
	var prof struct{ ID string }
	if pr.status != http.StatusCreated || json.Unmarshal([]byte(pr.body), &prof) != nil {
		t.Fatalf("bob adds a professional: %d %s", pr.status, pr.body)
	}
	profsBefore := h.call(t, http.MethodGet, "/api/v1/business/"+bobBiz+"/professionals", "", bearerH(bob)).body
	scheduleURL := "/api/v1/business/" + bobBiz + "/professionals/" + prof.ID + "/schedule"
	if r := h.call(t, http.MethodPut, scheduleURL, `{"days":[{"weekday":"MONDAY","workingHours":[{"start":"09:00","end":"18:00"}]}]}`, bearerH(bob)); r.status != http.StatusOK {
		t.Fatalf("bob sets a schedule: %d %s", r.status, r.body)
	}
	scheduleBefore := h.call(t, http.MethodGet, scheduleURL, "", bearerH(bob)).body
	absencesURL := "/api/v1/business/" + bobBiz + "/professionals/" + prof.ID + "/absences?from=2020-01-01T00:00:00Z"
	ab := h.call(t, http.MethodPost, "/api/v1/business/"+bobBiz+"/professionals/"+prof.ID+"/absences", `{"start":"2029-01-01T00:00:00Z","end":"2029-01-05T00:00:00Z","reason":"Vacaciones"}`, bearerH(bob))
	var absence struct{ ID string }
	if ab.status != http.StatusCreated || json.Unmarshal([]byte(ab.body), &absence) != nil {
		t.Fatalf("bob registers an absence: %d %s", ab.status, ab.body)
	}
	absencesBefore := h.call(t, http.MethodGet, absencesURL, "", bearerH(bob)).body
	created := func(path, body string) string {
		r := h.call(t, http.MethodPost, "/api/v1/business/"+bobBiz+path, body, bearerH(bob))
		var v struct{ ID string }
		if r.status != http.StatusCreated || json.Unmarshal([]byte(r.body), &v) != nil {
			t.Fatalf("bob creates %s: %d %s", path, r.status, r.body)
		}
		return v.ID
	}
	categoryID := created("/service-categories", `{"name":"Cortes"}`)
	customerID := created("/customers", `{"fullName":"Cliente de Bob","phone":"3001112233"}`)
	serviceID := created("/services", `{"name":"Corte + barba","durationMinutes":45,"priceMinor":3500000,"categoryId":"`+categoryID+`"}`)
	if r := h.call(t, http.MethodPut, "/api/v1/business/"+bobBiz+"/professionals/"+prof.ID+"/services", `{"serviceIds":["`+serviceID+`"]}`, bearerH(bob)); r.status != http.StatusOK {
		t.Fatalf("bob assigns a service: %d %s", r.status, r.body)
	}
	catalogURLs := []string{"/api/v1/business/" + bobBiz + "/services", "/api/v1/business/" + bobBiz + "/service-categories",
		"/api/v1/business/" + bobBiz + "/customers",
		"/api/v1/business/" + bobBiz + "/professionals/" + prof.ID + "/services"}
	catalogBefore := map[string]string{}
	for _, u := range catalogURLs {
		catalogBefore[u] = h.call(t, http.MethodGet, u, "", bearerH(bob)).body
	}
	before := h.call(t, http.MethodGet, "/api/v1/business/"+bobBiz, "", bearerH(bob)).body
	membersBefore := h.call(t, http.MethodGet, "/api/v1/business/"+bobBiz+"/members", "", bearerH(bob)).body
	deniedBefore := countAction(h.store.AuditActions(), application.AuditAuthzDenied+":DENIED")

	doc, err := libopenapi.NewDocument(api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	model, err := doc.BuildV3Model()
	if err != nil {
		t.Fatal(err)
	}
	const missing = "01a20aae-0000-7000-8000-000000000000"
	tested := 0
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		if !strings.Contains(path, "{businessId}") {
			continue
		}
		for method := range item.GetOperations().FromOldest() {
			method = strings.ToUpper(method)
			op := method + " " + path
			body, ok := tenantBodies[op]
			if !ok && method != http.MethodGet && method != http.MethodDelete {
				body = "{}"
			}
			url := func(biz string) string {
				return strings.NewReplacer("{businessId}", biz, "{userId}", staffID, "{professionalId}", prof.ID, "{absenceId}", absence.ID, "{categoryId}", categoryID, "{serviceId}", serviceID, "{customerId}", customerID).Replace(path)
			}
			if strings.Contains(url(bobBiz), "{") {
				t.Fatalf("%s: unresolved path placeholder in %s", op, url(bobBiz))
			}
			cross := h.call(t, method, url(bobBiz), body, bearerH(ana))
			none := h.call(t, method, url(missing), body, bearerH(ana))
			if cross.status != http.StatusNotFound || normalizeReply(t, cross) != normalizeReply(t, none) {
				t.Errorf("%s on another business: %d %s\n  vs nonexistent: %d %s", op, cross.status, cross.body, none.status, none.body)
			}
			tested++
		}
	}
	if tested < 26 {
		t.Fatalf("only %d business-scoped operations found in the contract", tested)
	}

	// IDOR inside the caller's own business (T5.8): Ana uses HER businessId with the ids of
	// Bob's professional, absence, category, service and member. Every operation with a child
	// id must answer exactly as if those ids did not exist.
	children := 0
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		if !strings.Contains(path, "{businessId}/") || !strings.Contains(strings.SplitN(path, "{businessId}", 2)[1], "{") {
			continue
		}
		for method := range item.GetOperations().FromOldest() {
			method = strings.ToUpper(method)
			op := method + " " + path
			body, ok := tenantBodies[op]
			if !ok && method != http.MethodGet && method != http.MethodDelete {
				body = "{}"
			}
			url := func(child func(string) string) string {
				return strings.NewReplacer("{businessId}", anaBiz, "{userId}", child(staffID), "{professionalId}", child(prof.ID),
					"{absenceId}", child(absence.ID), "{categoryId}", child(categoryID), "{serviceId}", child(serviceID), "{customerId}", child(customerID)).Replace(path)
			}
			foreign := h.call(t, method, url(func(id string) string { return id }), body, bearerH(ana))
			none := h.call(t, method, url(func(string) string { return missing }), body, bearerH(ana))
			if foreign.status != http.StatusNotFound || normalizeReply(t, foreign) != normalizeReply(t, none) {
				t.Errorf("%s with Bob's ids in Ana's business: %d %s\n  vs nonexistent: %d %s", op, foreign.status, foreign.body, none.status, none.body)
			}
			children++
		}
	}
	if children < 12 {
		t.Fatalf("only %d operations with child ids found in the contract", children)
	}

	// Foreign ids inside request bodies: Ana cannot assign Bob's service to her barber, nor file
	// her service under Bob's category.
	ap := h.call(t, http.MethodPost, "/api/v1/business/"+anaBiz+"/professionals", `{"email":"`+suiteEmail+`","displayName":"Ana"}`, bearerH(ana))
	var anaProf struct{ ID string }
	if ap.status != http.StatusCreated || json.Unmarshal([]byte(ap.body), &anaProf) != nil {
		t.Fatalf("ana registers herself as a professional: %d %s", ap.status, ap.body)
	}
	if r := h.call(t, http.MethodPut, "/api/v1/business/"+anaBiz+"/professionals/"+anaProf.ID+"/services", `{"serviceIds":["`+serviceID+`"]}`, bearerH(ana)); r.status != http.StatusBadRequest || !strings.Contains(r.body, "SERVICE_NOT_FOUND") {
		t.Errorf("assigning Bob's service: %d %s", r.status, r.body)
	}
	if r := h.call(t, http.MethodPost, "/api/v1/business/"+anaBiz+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":100,"categoryId":"`+categoryID+`"}`, bearerH(ana)); r.status < 400 || r.status >= 500 {
		t.Errorf("service under Bob's category: %d %s", r.status, r.body)
	}
	if r := h.call(t, http.MethodGet, "/api/v1/business/"+anaBiz+"/services", "", bearerH(ana)); r.status != http.StatusOK || strings.Contains(r.body, "Corte") {
		t.Errorf("Ana's services after the rejected attempts: %d %s", r.status, r.body)
	}
	// Nothing changed in Bob's business, and every attempt was audited as a denial.
	if after := h.call(t, http.MethodGet, "/api/v1/business/"+bobBiz, "", bearerH(bob)).body; after != before {
		t.Errorf("Bob's business changed:\n%s\n%s", before, after)
	}
	if after := h.call(t, http.MethodGet, "/api/v1/business/"+bobBiz+"/members", "", bearerH(bob)).body; after != membersBefore {
		t.Errorf("Bob's members changed:\n%s\n%s", membersBefore, after)
	}
	if after := h.call(t, http.MethodGet, "/api/v1/business/"+bobBiz+"/professionals", "", bearerH(bob)).body; after != profsBefore {
		t.Errorf("Bob's professionals changed:\n%s\n%s", profsBefore, after)
	}
	if after := h.call(t, http.MethodGet, scheduleURL, "", bearerH(bob)).body; after != scheduleBefore {
		t.Errorf("Bob's schedule changed:\n%s\n%s", scheduleBefore, after)
	}
	if after := h.call(t, http.MethodGet, absencesURL, "", bearerH(bob)).body; after != absencesBefore {
		t.Errorf("Bob's absences changed:\n%s\n%s", absencesBefore, after)
	}
	for _, u := range catalogURLs {
		if after := h.call(t, http.MethodGet, u, "", bearerH(bob)).body; after != catalogBefore[u] {
			t.Errorf("Bob's catalog changed (%s):\n%s\n%s", u, catalogBefore[u], after)
		}
	}
	if denied := countAction(h.store.AuditActions(), application.AuditAuthzDenied+":DENIED") - deniedBefore; denied < tested {
		t.Errorf("authz.denied entries = %d, want at least %d (one per cross-tenant attempt)", denied, tested)
	}
	t.Logf("%d business-scoped operations checked, %d with foreign child ids", tested, children)
}

func countAction(actions []string, want string) int {
	n := 0
	for _, a := range actions {
		if a == want {
			n++
		}
	}
	return n
}
