package app

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
)

func TestContractBusinessEndpoints(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	userTok := e.signIn(t, "ana@example.com")
	e.signIn(t, "dueno@example.com")
	adminTok := e.signIn(t, "admin@example.com")
	if err := e.identity.SetPlatformAdmin(context.Background(), "admin@example.com", true); err != nil {
		t.Fatal(err)
	}
	cali := geotest.CaliID.String()
	create := `{"name":"Barbería El Paisa","contactPhone":"300 123 4567","cityId":"` + cali + `","addressLine":"Calle 5 # 38-25","coordinates":{"latitude":3.4516,"longitude":-76.532},"ownerEmail":"dueno@example.com"}`

	body := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses", create, adminTok, http.StatusCreated)
	for _, want := range []string{`"status":"ACTIVE"`, `"slug":"barberia-el-paisa"`, `"contactPhone":"+573001234567"`, `"currency":"COP"`, `"timeZone":"America/Bogota"`} {
		if !strings.Contains(body, want) {
			t.Errorf("created business lacks %s: %s", want, body)
		}
	}
	id := regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(body)[1]

	// Second business without coordinates (null in the representation).
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses", `{"name":"Uñas Spa","contactPhone":"+57 602 486 1234","cityId":"`+cali+`","addressLine":"Cra 1","ownerEmail":"ana@example.com"}`, adminTok, http.StatusCreated)
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses", `{"name":"X","contactPhone":"1","cityId":"`+cali+`","addressLine":"","ownerEmail":"nadie@example.com"}`, adminTok, http.StatusBadRequest)

	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses/"+id, "", adminTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses/00000000-0000-0000-0000-000000000001", "", adminTok, http.StatusNotFound)
	page := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses?limit=1", "", adminTok, http.StatusOK)
	if !strings.Contains(page, `"nextCursor"`) {
		t.Errorf("first page of 2 must have nextCursor: %s", page)
	}
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses?status=ACTIVE&cityId="+cali, "", adminTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses?limit=0", "", adminTok, http.StatusBadRequest)

	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses/"+id+"/activate", "", adminTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses/"+id+"/suspend", "", adminTok, http.StatusOK)
	// Inactive location: activation refused with LOCATION_INACTIVE.
	e.geo.ActiveBusinesses = map[string]int{}
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/admin/cities/"+cali, `{"active":false}`, adminTok, http.StatusOK)
	if b := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses/"+id+"/activate", "", adminTok, http.StatusConflict); !strings.Contains(b, `"LOCATION_INACTIVE"`) {
		t.Errorf("activation with inactive city: %s", b)
	}

	// Authentication and authorization.
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses", "", "", http.StatusUnauthorized)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses", "", userTok, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses", create, userTok, http.StatusForbidden)

	// Born ACTIVE: the first "activate" is a no-op (not audited); then suspended.
	if got := e.biz.AuditActions(); len(got) != 3 {
		t.Errorf("business audit = %v, want created×2, suspended", got)
	}
}

func TestContractSelfServiceAndSubscriptions(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	ownerTok := e.signIn(t, "dueno@example.com")
	userTok := e.signIn(t, "ana@example.com")
	adminTok := e.signIn(t, "admin@example.com")
	if err := e.identity.SetPlatformAdmin(context.Background(), "admin@example.com", true); err != nil {
		t.Fatal(err)
	}
	cali := geotest.CaliID.String()
	own := `{"name":"Barbería Propia","contactPhone":"3001234567","cityId":"` + cali + `","addressLine":"Calle 1"}`

	body := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", own, ownerTok, http.StatusCreated)
	if !strings.Contains(body, `"status":"ACTIVE"`) {
		t.Errorf("self-service business must be ACTIVE: %s", body)
	}
	id := regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(body)[1]
	if b := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", own, ownerTok, http.StatusConflict); !strings.Contains(b, `"ALREADY_OWNER"`) {
		t.Errorf("second business of the same account: %s", b)
	}
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"x","contactPhone":"1","cityId":"`+cali+`","addressLine":""}`, userTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", own, "", http.StatusUnauthorized)

	sub := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses/"+id+"/subscription", "", adminTok, http.StatusOK)
	if !strings.Contains(sub, `"status":"TRIALING"`) || !strings.Contains(sub, `"entitled":true`) {
		t.Errorf("subscription = %s", sub)
	}
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses/"+id+"/subscription/extend-trial", `{"days":14}`, adminTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses/"+id+"/subscription/extend-trial", `{"days":0}`, adminTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses/"+id+"/subscription/activate", "", adminTok, http.StatusOK)
	if b := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/businesses/"+id+"/subscription/extend-trial", `{"days":3}`, adminTok, http.StatusConflict); !strings.Contains(b, `"NOT_EXTENDABLE"`) {
		t.Errorf("extend ACTIVE: %s", b)
	}
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses/00000000-0000-0000-0000-000000000001/subscription", "", adminTok, http.StatusNotFound)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses/"+id+"/subscription", "", ownerTok, http.StatusForbidden)
}

func TestContractBusinessManagement(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	ownerTok := e.signIn(t, "dueno@example.com")
	managerTok := e.signIn(t, "gerente@example.com")
	otherTok := e.signIn(t, "otro@example.com")
	ctx := context.Background()
	cali := geotest.CaliID.String()

	body := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Propia","contactPhone":"3001234567","cityId":"`+cali+`","addressLine":"Calle 1","coordinates":{"latitude":3.45,"longitude":-76.53}}`, ownerTok, http.StatusCreated)
	id := regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(body)[1]
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Otra","contactPhone":"3001234567","cityId":"`+cali+`","addressLine":"Calle 2"}`, otherTok, http.StatusCreated)
	admin := identityPrincipalOf(t, e, "admin-mgmt@example.com")
	if _, err := e.identity.AddMember(ctx, admin, uuidOf(t, id), "gerente@example.com", "PROFESSIONAL"); err != nil {
		t.Fatal(err)
	}

	got := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/business/"+id, "", ownerTok, http.StatusOK)
	if !strings.Contains(got, `"subscription":{"status":"TRIALING"`) || !strings.Contains(got, `"entitled":true`) {
		t.Errorf("managed business = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/business/"+id, "", managerTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/business/"+id, "", otherTok, http.StatusNotFound)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/business/"+id, "", "", http.StatusUnauthorized)

	patch := `{"name":"Barbería Renovada","coordinates":null,"bookingPolicy":{"slotIntervalMinutes":15},"reminders":{"hoursBefore":2}}`
	updated := c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/business/"+id, patch, ownerTok, http.StatusOK)
	if !strings.Contains(updated, `"coordinates":null`) || !strings.Contains(updated, `"slotIntervalMinutes":15`) {
		t.Errorf("updated = %s", updated)
	}
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/business/"+id, `{"name":"x"}`, ownerTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/business/"+id, patch, managerTok, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/business/"+id, patch, otherTok, http.StatusNotFound)
}

// identityPrincipalOf signs in email as PLATFORM_ADMIN and returns its principal.
func identityPrincipalOf(t *testing.T, e geoEnv, email string) identitycontract.Principal {
	t.Helper()
	e.signIn(t, email)
	if err := e.identity.SetPlatformAdmin(context.Background(), email, true); err != nil {
		t.Fatal(err)
	}
	return identitycontract.Principal{UserID: e.users.User(email).ID(), IsPlatformAdmin: true}
}

func uuidOf(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestContractBusinessMembers(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	ownerTok := e.signIn(t, "dueno@example.com")
	barberTok := e.signIn(t, "barbero@example.com")
	e.signIn(t, "socio@example.com")
	cali := geotest.CaliID.String()
	body := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Miembros","contactPhone":"3001234567","cityId":"`+cali+`","addressLine":"Calle 1"}`, ownerTok, http.StatusCreated)
	id := regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(body)[1]
	base := "/api/v1/business/" + id + "/members"

	added := c.callAuth(t, e.srv, http.MethodPost, base, `{"email":"barbero@example.com","role":"PROFESSIONAL"}`, ownerTok, http.StatusCreated)
	barber := regexp.MustCompile(`"userId":"([^"]+)"`).FindStringSubmatch(added)[1]
	c.callAuth(t, e.srv, http.MethodPost, base, `{"email":"socio@example.com","role":"OWNER"}`, ownerTok, http.StatusCreated)
	c.callAuth(t, e.srv, http.MethodPost, base, `{"email":"barbero@example.com","role":"PROFESSIONAL"}`, ownerTok, http.StatusConflict)
	c.callAuth(t, e.srv, http.MethodPost, base, `{"email":"nadie@example.com","role":"PROFESSIONAL"}`, ownerTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPost, base, `{"email":"socio@example.com","role":"BOSS"}`, ownerTok, http.StatusBadRequest)

	list := c.callAuth(t, e.srv, http.MethodGet, base, "", ownerTok, http.StatusOK)
	if strings.Count(list, `"role":"OWNER"`) != 2 || !strings.Contains(list, `"email":"barbero@example.com"`) {
		t.Errorf("members = %s", list)
	}
	c.callAuth(t, e.srv, http.MethodGet, base, "", barberTok, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodPatch, base+"/"+barber, `{"role":"PROFESSIONAL"}`, ownerTok, http.StatusNoContent)
	c.callAuth(t, e.srv, http.MethodPatch, base+"/00000000-0000-0000-0000-000000000001", `{"role":"PROFESSIONAL"}`, ownerTok, http.StatusNotFound)
	c.callAuth(t, e.srv, http.MethodDelete, base+"/"+barber, "", ownerTok, http.StatusNoContent)
	c.callAuth(t, e.srv, http.MethodGet, base, "", barberTok, http.StatusNotFound) // removed: no longer a member

	// The last OWNER cannot leave: remove the partner first, then try to remove oneself.
	socio := e.users.User("socio@example.com").ID().String()
	dueno := e.users.User("dueno@example.com").ID().String()
	c.callAuth(t, e.srv, http.MethodDelete, base+"/"+socio, "", ownerTok, http.StatusNoContent)
	if b := c.callAuth(t, e.srv, http.MethodDelete, base+"/"+dueno, "", ownerTok, http.StatusConflict); !strings.Contains(b, `"LAST_OWNER"`) {
		t.Errorf("last owner removal: %s", b)
	}
	c.callAuth(t, e.srv, http.MethodPatch, base+"/"+dueno, `{"role":"PROFESSIONAL"}`, ownerTok, http.StatusConflict)
}

func TestContractPublicBusinesses(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	cali := geotest.CaliID.String()
	for i, email := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		tok := e.signIn(t, email)
		c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería `+string(rune('A'+i))+`","contactPhone":"3001234567","cityId":"`+cali+`","addressLine":"Calle 1","coordinates":{"latitude":3.45,"longitude":-76.53}}`, tok, http.StatusCreated)
	}
	page := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses?cityId="+cali+"&limit=2", "", "", http.StatusOK)
	if !strings.Contains(page, `"nextCursor"`) || strings.Contains(page, `"status"`) || strings.Contains(page, `"reminders"`) {
		t.Errorf("public page = %s", page)
	}
	cursor := regexp.MustCompile(`"nextCursor":"([^"]+)"`).FindStringSubmatch(page)[1]
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses?cityId="+cali+"&cursor="+cursor, "", "", http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses?cityId="+cali+"&q=barb", "", "", http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses", "", "", http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses?cityId="+cali+"&limit=500", "", "", http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses?cityId="+cali+"&q=x", "", "", http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/barberia-a", "", "", http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/no-existe", "", "", http.StatusNotFound)
}

func TestContractProfessionals(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	ownerTok := e.signIn(t, "dueno@example.com")
	barberTok := e.signIn(t, "barbero@example.com")
	cali := geotest.CaliID.String()
	body := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Profesionales","contactPhone":"3001234567","cityId":"`+cali+`","addressLine":"Calle 1"}`, ownerTok, http.StatusCreated)
	biz := "/api/v1/business/" + regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(body)[1] + "/professionals"

	created := c.callAuth(t, e.srv, http.MethodPost, biz, `{"email":"barbero@example.com","displayName":"Juan el Barbero","bio":"Fades","sortOrder":1}`, ownerTok, http.StatusCreated)
	id := regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(created)[1]
	c.callAuth(t, e.srv, http.MethodPost, biz, `{"email":"dueno@example.com","displayName":"El Dueño"}`, ownerTok, http.StatusCreated) // the OWNER also serves
	c.callAuth(t, e.srv, http.MethodPost, biz, `{"email":"barbero@example.com","displayName":"Otra vez"}`, ownerTok, http.StatusConflict)
	c.callAuth(t, e.srv, http.MethodPost, biz, `{"email":"nadie@example.com","displayName":"Nadie"}`, ownerTok, http.StatusBadRequest)
	list := c.callAuth(t, e.srv, http.MethodGet, biz, "", barberTok, http.StatusOK)
	if !strings.Contains(list, `"displayName":"Juan el Barbero"`) || !strings.Contains(list, `"bookable":true`) {
		t.Errorf("list = %s", list)
	}
	c.callAuth(t, e.srv, http.MethodGet, biz+"/"+id, "", ownerTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, biz+"/00000000-0000-0000-0000-000000000001", "", ownerTok, http.StatusNotFound)
	c.callAuth(t, e.srv, http.MethodPatch, biz+"/"+id, `{"status":"INACTIVE","sortOrder":3}`, ownerTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPatch, biz+"/"+id, `{"status":"PAUSED"}`, ownerTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPatch, biz+"/"+id, `{"displayName":"Yo mismo"}`, barberTok, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodGet, biz, "", "", http.StatusUnauthorized)
}
