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
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	for i, email := range []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com"} {
		tok := e.signIn(t, email)
		base := "/api/v1/business/" + idOf(c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería `+string(rune('A'+i))+`","contactPhone":"3001234567","cityId":"`+cali+`","addressLine":"Calle 1","coordinates":{"latitude":3.45,"longitude":-76.53}}`, tok, http.StatusCreated))
		svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, tok, http.StatusCreated))
		if i == 3 {
			continue // Barbería D has a service but nobody performs it: not listed (T5.7)
		}
		prof := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"`+email+`","displayName":"Barbero `+string(rune('A'+i))+`"}`, tok, http.StatusCreated))
		c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+prof+"/services", `{"serviceIds":["`+svc+`"]}`, tok, http.StatusOK)
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
	if all := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses?cityId="+cali+"&limit=50", "", "", http.StatusOK); strings.Contains(all, "Barbería D") || !strings.Contains(all, "Barbería C") {
		t.Errorf("search without offer filter: %s", all)
	}

	// Public catalog (T5.7).
	cat := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/barberia-a/catalog", "", "", http.StatusOK)
	if !strings.Contains(cat, `"name":"Corte"`) || !strings.Contains(cat, `"displayName":"Barbero A"`) || strings.Contains(cat, "userId") || strings.Contains(cat, "status") {
		t.Errorf("public catalog = %s", cat)
	}
	if empty := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/barberia-d/catalog", "", "", http.StatusOK); !strings.Contains(empty, `"services":[]`) {
		t.Errorf("catalog without professionals = %s", empty)
	}
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/no-existe/catalog", "", "", http.StatusNotFound)
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
	ownerProf := regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(
		c.callAuth(t, e.srv, http.MethodPost, biz, `{"email":"dueno@example.com","displayName":"El Dueño"}`, ownerTok, http.StatusCreated))[1] // the OWNER also serves
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

	// Weekly schedule (T5.3): the OWNER sets it, the barber adjusts his own only.
	week := `{"days":[{"weekday":"MONDAY","workingHours":[{"start":"09:00","end":"13:00"},{"start":"14:00","end":"19:00"}],"breaks":[{"start":"11:00","end":"11:15"}]}]}`
	if got := c.callAuth(t, e.srv, http.MethodGet, biz+"/"+id+"/schedule", "", barberTok, http.StatusOK); !strings.Contains(got, `{"weekday":"MONDAY","workingHours":[],"breaks":[]}`) {
		t.Errorf("empty schedule = %s", got)
	}
	if got := c.callAuth(t, e.srv, http.MethodPut, biz+"/"+id+"/schedule", week, ownerTok, http.StatusOK); !strings.Contains(got, `{"start":"14:00","end":"19:00"}`) {
		t.Errorf("schedule = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodPut, biz+"/"+id+"/schedule", `{"days":[{"weekday":"MONDAY","workingHours":[{"start":"09:00","end":"13:00"}],"breaks":[{"start":"12:30","end":"13:30"}]}]}`, ownerTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPut, biz+"/"+id+"/schedule", `{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"10:00","end":"18:00"}]}]}`, barberTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPut, biz+"/"+ownerProf+"/schedule", week, barberTok, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodGet, biz+"/00000000-0000-0000-0000-000000000001/schedule", "", ownerTok, http.StatusNotFound)

	// Absences (T5.4): the OWNER for anyone, the barber his own.
	abs := biz + "/" + id + "/absences"
	created = c.callAuth(t, e.srv, http.MethodPost, abs, `{"start":"2030-12-01T08:00:00-05:00","end":"2030-12-03T19:00:00-05:00","reason":"Incapacidad"}`, barberTok, http.StatusCreated)
	if !strings.Contains(created, `"start":"2030-12-01T13:00:00Z"`) {
		t.Errorf("absence = %s", created)
	}
	absID := regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(created)[1]
	c.callAuth(t, e.srv, http.MethodPost, abs, `{"start":"2030-12-02T00:00:00Z","end":"2030-12-04T00:00:00Z"}`, ownerTok, http.StatusConflict)
	c.callAuth(t, e.srv, http.MethodPost, abs, `{"start":"2030-12-05T00:00:00Z","end":"2030-12-04T00:00:00Z"}`, ownerTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPost, biz+"/"+ownerProf+"/absences", `{"start":"2030-12-05T00:00:00Z","end":"2030-12-06T00:00:00Z"}`, barberTok, http.StatusForbidden)
	if list := c.callAuth(t, e.srv, http.MethodGet, abs+"?from=2030-01-01T00:00:00Z", "", ownerTok, http.StatusOK); !strings.Contains(list, absID) {
		t.Errorf("absences = %s", list)
	}
	c.callAuth(t, e.srv, http.MethodGet, abs+"?from=ayer", "", ownerTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPatch, abs+"/"+absID, `{"reason":"Cita médica"}`, barberTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodDelete, abs+"/"+absID, "", barberTok, http.StatusNoContent)
	c.callAuth(t, e.srv, http.MethodDelete, abs+"/"+absID, "", ownerTok, http.StatusNotFound)
}

func TestContractCatalog(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	ownerTok := e.signIn(t, "dueno@example.com")
	barberTok := e.signIn(t, "barbero@example.com")
	body := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Catálogo","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, ownerTok, http.StatusCreated)
	base := "/api/v1/business/" + regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(body)[1]
	c.callAuth(t, e.srv, http.MethodPost, base+"/members", `{"email":"barbero@example.com","role":"PROFESSIONAL"}`, ownerTok, http.StatusCreated)
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }

	cat := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/service-categories", `{"name":"Barba","sortOrder":1}`, ownerTok, http.StatusCreated))
	c.callAuth(t, e.srv, http.MethodPost, base+"/service-categories", `{"name":"barba"}`, ownerTok, http.StatusConflict)
	c.callAuth(t, e.srv, http.MethodPost, base+"/service-categories", `{"name":"Uñas"}`, barberTok, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodGet, base+"/service-categories", "", barberTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPatch, base+"/service-categories/"+cat, `{"status":"INACTIVE"}`, ownerTok, http.StatusOK)

	created := c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte + barba","description":"Toalla caliente","categoryId":"`+cat+`","durationMinutes":45,"priceMinor":3500000}`, ownerTok, http.StatusCreated)
	if !strings.Contains(created, `"currency":"COP"`) || !strings.Contains(created, `"categoryId":"`+cat+`"`) {
		t.Errorf("service = %s", created)
	}
	svc := idOf(created)
	c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, ownerTok, http.StatusCreated)
	c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"CORTE","durationMinutes":30,"priceMinor":2500000}`, ownerTok, http.StatusConflict)
	c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Tinte","durationMinutes":30,"priceMinor":2500000,"categoryId":"01a20aae-0000-7000-8000-000000000000"}`, ownerTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Tinte","durationMinutes":30,"priceMinor":2500000}`, barberTok, http.StatusForbidden)
	if list := c.callAuth(t, e.srv, http.MethodGet, base+"/services", "", barberTok, http.StatusOK); !strings.Contains(list, `"name":"Corte + barba"`) {
		t.Errorf("services = %s", list)
	}
	c.callAuth(t, e.srv, http.MethodGet, base+"/services/"+svc, "", barberTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, base+"/services/01a20aae-0000-7000-8000-000000000000", "", ownerTok, http.StatusNotFound)
	if upd := c.callAuth(t, e.srv, http.MethodPatch, base+"/services/"+svc, `{"priceMinor":4000000,"categoryId":null}`, ownerTok, http.StatusOK); !strings.Contains(upd, `"categoryId":null`) {
		t.Errorf("updated = %s", upd)
	}
	c.callAuth(t, e.srv, http.MethodPatch, base+"/services/"+svc, `{"priceMinor":4000000}`, barberTok, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodDelete, base+"/service-categories/"+cat, "", ownerTok, http.StatusNoContent)
	c.callAuth(t, e.srv, http.MethodDelete, base+"/service-categories/"+cat, "", ownerTok, http.StatusNotFound)
	c.callAuth(t, e.srv, http.MethodGet, base+"/services", "", "", http.StatusUnauthorized)

	// Services per professional (T5.6).
	prof := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"barbero@example.com","displayName":"Juan"}`, ownerTok, http.StatusCreated))
	ps := base + "/professionals/" + prof + "/services"
	if got := c.callAuth(t, e.srv, http.MethodPut, ps, `{"serviceIds":["`+svc+`"]}`, ownerTok, http.StatusOK); !strings.Contains(got, svc) {
		t.Errorf("assigned = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodGet, ps, "", barberTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPut, ps, `{"serviceIds":["`+svc+`"]}`, barberTok, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodPut, ps, `{"serviceIds":["01a20aae-0000-7000-8000-000000000000"]}`, ownerTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodGet, base+"/professionals/01a20aae-0000-7000-8000-000000000000/services", "", ownerTok, http.StatusNotFound)
}
