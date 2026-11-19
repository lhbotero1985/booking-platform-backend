package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

func (f fixture) anaID(t *testing.T) uuid.UUID {
	t.Helper()
	u := f.store.User("ana@example.com")
	if u == nil {
		t.Fatal("ana not registered")
	}
	return u.ID()
}

func TestMyMembershipsEndpoint(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	tok := f.mobileToken(t)

	r := f.do(t, http.MethodGet, "/api/v1/me/memberships", "", bearer(tok))
	if r.status != http.StatusOK || strings.TrimSpace(r.body) != `{"items":[]}` {
		t.Fatalf("customer: %d %s", r.status, r.body)
	}

	biz := uuid.New()
	f.store.GrantMembership(f.anaID(t), biz, contract.RoleManager)
	r = f.do(t, http.MethodGet, "/api/v1/me/memberships", "", bearer(tok))
	want := fmt.Sprintf(`{"items":[{"businessId":"%s","role":"MANAGER"}]}`, biz)
	if r.status != http.StatusOK || strings.TrimSpace(r.body) != want {
		t.Fatalf("manager: %d %s", r.status, r.body)
	}

	if r := f.do(t, http.MethodGet, "/api/v1/me/memberships", "", nil); r.status != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", r.status)
	}
}

// TestAuthorizationPatternEndToEnd exercises the pattern every module will follow: RequireAuth
// → use case calls Authorizer.RequireBusinessRole → errors mapped by httpx.WriteAuthzProblem.
// The probe routes stand in for Phase 4+ business endpoints.
func TestAuthorizationPatternEndToEnd(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	tok := f.mobileToken(t)
	bizA, bizB := uuid.New(), uuid.New()
	f.store.GrantMembership(f.anaID(t), bizA, contract.RoleProfessional)

	probe := func(allowed []contract.Role) http.Handler {
		return f.authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, _ := contract.PrincipalFrom(r.Context())
			biz, err := uuid.Parse(r.PathValue("businessID"))
			if err != nil {
				biz = uuid.Nil // malformed IDs are just "not found"
			}
			role, err := f.svc.RequireBusinessRole(r.Context(), p, biz, allowed...)
			if httpx.WriteAuthzProblem(w, r, err) {
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"role": string(role)})
		}))
	}
	rt := httpx.NewRouter()
	rt.Handle("GET /probe/businesses/{businessID}/agenda", probe(contract.RolesStaff))
	rt.Handle("GET /probe/businesses/{businessID}/settings", probe(contract.RolesOwner))
	h := httpx.Chain(rt, httpx.RequestID)

	get := func(path string) authResp {
		f2 := f
		f2.h = h
		return f2.do(t, http.MethodGet, path, "", bearer(tok))
	}
	unknownRoute := get("/probe/no-such-route")

	if r := get("/probe/businesses/" + bizA.String() + "/agenda"); r.status != http.StatusOK || !strings.Contains(r.body, `"role":"PROFESSIONAL"`) {
		t.Errorf("own business, allowed: %d %s", r.status, r.body)
	}
	if r := get("/probe/businesses/" + bizA.String() + "/settings"); r.status != http.StatusForbidden || !strings.Contains(r.body, `"code":"FORBIDDEN"`) {
		t.Errorf("own business, insufficient role: %d %s", r.status, r.body)
	}
	for _, path := range []string{
		"/probe/businesses/" + bizB.String() + "/agenda",
		"/probe/businesses/" + bizB.String() + "/settings",
		"/probe/businesses/not-a-uuid/agenda",
	} {
		r := get(path)
		if r.status != http.StatusNotFound {
			t.Errorf("%s: %d %s", path, r.status, r.body)
			continue
		}
		if stripVolatile(t, r.body) != stripVolatile(t, unknownRoute.body) {
			t.Errorf("%s: cross-tenant 404 differs from a missing route:\n%s\n%s", path, r.body, unknownRoute.body)
		}
	}

	// Revocation takes effect on the very next request (no cached claims).
	f.store.RevokeMembership(f.anaID(t), bizA)
	if r := get("/probe/businesses/" + bizA.String() + "/agenda"); r.status != http.StatusNotFound {
		t.Errorf("after revoke: %d %s", r.status, r.body)
	}
}

// authMW is the fixture handler's RequireAuth.
func (f fixture) authMW(next http.Handler) http.Handler {
	return f.api.RequireAuth(next)
}

// stripVolatile removes the per-request fields (requestId, instance) from a problem body.
func stripVolatile(t *testing.T, body string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("not JSON: %s", body)
	}
	delete(m, "requestId")
	delete(m, "instance")
	out, _ := json.Marshal(m)
	return string(out)
}
