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
	"PATCH /api/v1/business/{businessId}":                                `{"name":"Hackeada","description":"x"}`,
	"POST /api/v1/business/{businessId}/members":                         `{"email":"intruso@example.com","role":"OWNER"}`,
	"PATCH /api/v1/business/{businessId}/members/{userId}":               `{"role":"OWNER"}`,
	"POST /api/v1/business/{businessId}/professionals":                   `{"email":"intruso@example.com","displayName":"Intruso"}`,
	"PATCH /api/v1/business/{businessId}/professionals/{professionalId}": `{"displayName":"Hackeado"}`,
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
	create(ana, "Barbería Ana")
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
				return strings.NewReplacer("{businessId}", biz, "{userId}", staffID, "{professionalId}", prof.ID).Replace(path)
			}
			cross := h.call(t, method, url(bobBiz), body, bearerH(ana))
			none := h.call(t, method, url(missing), body, bearerH(ana))
			if cross.status != http.StatusNotFound || normalizeReply(t, cross) != normalizeReply(t, none) {
				t.Errorf("%s on another business: %d %s\n  vs nonexistent: %d %s", op, cross.status, cross.body, none.status, none.body)
			}
			tested++
		}
	}
	if tested < 10 {
		t.Fatalf("only %d business-scoped operations found in the contract", tested)
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
	if denied := countAction(h.store.AuditActions(), application.AuditAuthzDenied+":DENIED") - deniedBefore; denied < tested {
		t.Errorf("authz.denied entries = %d, want at least %d (one per cross-tenant attempt)", denied, tested)
	}
	t.Logf("%d business-scoped operations checked", tested)
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
