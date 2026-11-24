package app

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
)

// TestContractPublicAvailability (T6.2): the public availability against api/openapi.yaml with
// the real composition. The test clock is Monday 2026-11-23 12:00 UTC (07:00 in Cali).
func TestContractPublicAvailability(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	tok := e.signIn(t, "dueno@example.com")
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	created := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Agenda","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, tok, http.StatusCreated)
	base := "/api/v1/business/" + idOf(created)
	prof := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"dueno@example.com","displayName":"El Dueño"}`, tok, http.StatusCreated))
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+prof+"/schedule",
		`{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"09:00","end":"12:00"}],"breaks":[{"start":"10:00","end":"10:30"}]}]}`, tok, http.StatusOK)
	// 11:00–11:30 in Cali (UTC-5).
	c.callAuth(t, e.srv, http.MethodPost, base+"/professionals/"+prof+"/absences", `{"start":"2026-11-24T16:00:00Z","end":"2026-11-24T16:30:00Z","reason":"Diligencia"}`, tok, http.StatusCreated)
	svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, tok, http.StatusCreated))
	other := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Barba","durationMinutes":20,"priceMinor":1500000}`, tok, http.StatusCreated))
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+prof+"/services", `{"serviceIds":["`+svc+`"]}`, tok, http.StatusOK)

	url := func(slug, service, date string) string {
		return "/api/v1/businesses/" + slug + "/availability?serviceId=" + service + "&professionalId=" + prof + "&date=" + date
	}
	got := c.callAuth(t, e.srv, http.MethodGet, url("barberia-agenda", svc, "2026-11-24"), "", "", http.StatusOK)
	starts := regexp.MustCompile(`"startLocal":"2026-11-24T(\d\d:\d\d):00-05:00"`).FindAllStringSubmatch(got, -1)
	var hours []string
	for _, m := range starts {
		hours = append(hours, m[1])
	}
	if strings.Join(hours, " ") != "09:00 09:10 09:20 09:30 10:30 11:30" || !strings.Contains(got, `"timeZone":"America/Bogota"`) ||
		!strings.Contains(got, `"startAt":"2026-11-24T14:00:00Z"`) || !strings.Contains(got, `"durationMinutes":30`) {
		t.Errorf("availability = %s (starts %v)", got, hours)
	}
	if got := c.callAuth(t, e.srv, http.MethodGet, url("barberia-agenda", svc, "2026-11-30"), "", "", http.StatusOK); !strings.Contains(got, `"slots":[]`) {
		t.Errorf("a Monday without schedule = %s", got)
	}
	for _, d := range []string{"2026-11-22", "2027-01-23"} { // yesterday; beyond 60 days
		if p := c.callAuth(t, e.srv, http.MethodGet, url("barberia-agenda", svc, d), "", "", http.StatusBadRequest); !strings.Contains(p, "DATE_OUTSIDE_BOOKING_WINDOW") {
			t.Errorf("%s: %s", d, p)
		}
	}
	// Indistinguishable 404: unknown business, service not performed by the professional,
	// unknown service.
	c.callAuth(t, e.srv, http.MethodGet, url("no-existe", svc, "2026-11-24"), "", "", http.StatusNotFound)
	c.callAuth(t, e.srv, http.MethodGet, url("barberia-agenda", other, "2026-11-24"), "", "", http.StatusNotFound)
	c.callAuth(t, e.srv, http.MethodGet, url("barberia-agenda", "01a20aae-0000-7000-8000-000000000000", "2026-11-24"), "", "", http.StatusNotFound)
	// The professional becomes INACTIVE: no longer bookable.
	c.callAuth(t, e.srv, http.MethodPatch, base+"/professionals/"+prof, `{"status":"INACTIVE"}`, tok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, url("barberia-agenda", svc, "2026-11-24"), "", "", http.StatusNotFound)
}
