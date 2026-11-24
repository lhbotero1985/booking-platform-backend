package app

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
)

// TestContractAgenda (T6.5): agenda and "my appointments" against api/openapi.yaml, with who may
// see what. Tuesday 2026-11-24 in Cali; the test clock is Monday 2026-11-23 12:00 UTC.
func TestContractAgenda(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	owner := e.signIn(t, "dueno@example.com")
	barber := e.signIn(t, "juan@example.com")
	ana := e.signIn(t, "ana@example.com")
	bob := e.signIn(t, "bob@example.com")
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	biz := idOf(c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Agenda Semanal","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, owner, http.StatusCreated))
	base := "/api/v1/business/" + biz
	c.callAuth(t, e.srv, http.MethodPost, base+"/members", `{"email":"juan@example.com","role":"PROFESSIONAL"}`, owner, http.StatusCreated)
	juan := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"juan@example.com","displayName":"Juan"}`, owner, http.StatusCreated))
	boss := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"dueno@example.com","displayName":"El Dueño"}`, owner, http.StatusCreated))
	svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, owner, http.StatusCreated))
	for _, p := range []string{juan, boss} {
		c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+p+"/schedule", `{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"09:00","end":"12:00"}]}]}`, owner, http.StatusOK)
		c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+p+"/services", `{"serviceIds":["`+svc+`"]}`, owner, http.StatusOK)
	}
	key := func(k string) map[string]string { return map[string]string{"Idempotency-Key": k} }
	appt := idOf(c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", `{"businessId":"`+biz+`","serviceId":"`+svc+`","professionalId":"`+juan+`","startAt":"2026-11-24T14:00:00Z"}`, ana, key("ana-agenda-0001"), http.StatusCreated))
	c.callWith(t, e.srv, http.MethodPost, base+"/appointments", `{"professionalId":"`+boss+`","serviceId":"`+svc+`","startAt":"2026-11-24T15:00:00Z","customer":{"fullName":"Laura","phone":"3115550303"}}`, owner, key("owner-agenda-0001"), http.StatusCreated)

	agenda := func(tok, query string, want int) string {
		return c.callAuth(t, e.srv, http.MethodGet, base+"/agenda"+query, "", tok, want)
	}
	if got := agenda(owner, "?view=week&date=2026-11-24", http.StatusOK); strings.Count(got, `"professionalName"`) != 2 ||
		!strings.Contains(got, `"from":"2026-11-23"`) || !strings.Contains(got, `"to":"2026-11-29"`) || !strings.Contains(got, `"startLocal":"2026-11-24T09:00:00-05:00"`) {
		t.Errorf("owner, week = %s", got)
	}
	if got := agenda(owner, "?view=day&date=2026-11-24&professionalId="+boss, http.StatusOK); strings.Count(got, `"professionalName"`) != 1 || !strings.Contains(got, `"name":"Laura"`) {
		t.Errorf("owner, his own agenda = %s", got)
	}
	if got := agenda(barber, "?view=month&date=2026-11-24", http.StatusOK); strings.Count(got, `"professionalName":"Juan"`) != 1 || strings.Contains(got, "Laura") {
		t.Errorf("barber, his month = %s", got)
	}
	agenda(barber, "?date=2026-11-24&professionalId="+boss, http.StatusForbidden)
	agenda(ana, "?date=2026-11-24", http.StatusNotFound) // a customer is not staff
	agenda(owner, "?view=decade", http.StatusBadRequest)
	if got := agenda(owner, "?view=day&date=2026-11-24&limit=1", http.StatusOK); !strings.Contains(got, `"nextCursor":"`) {
		t.Errorf("paginated = %s", got)
	}

	mine := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/me/appointments", "", ana, http.StatusOK)
	if !strings.Contains(mine, `"name":"Barbería Agenda Semanal"`) || !strings.Contains(mine, `"name":"Juan"`) || !strings.Contains(mine, `"nextCursor":null`) {
		t.Errorf("my appointments = %s", mine)
	}
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/me/appointments?scope=past", "", ana, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/me/appointments?scope=all", "", ana, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/me/appointments/"+appt, "", ana, http.StatusOK)
	other := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/me/appointments/"+appt, "", bob, http.StatusNotFound)
	missing := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/me/appointments/01a20aae-0000-7000-8000-000000000000", "", bob, http.StatusNotFound)
	strip := regexp.MustCompile(`"(instance|requestId)":"[^"]*"`)
	if strip.ReplaceAllString(other, "") != strip.ReplaceAllString(missing, "") {
		t.Errorf("another customer's appointment differs from a missing one:\n%s\n%s", other, missing)
	}
}
