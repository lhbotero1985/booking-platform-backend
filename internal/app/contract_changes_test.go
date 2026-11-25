package app

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
)

// TestContractAppointmentChanges (T6.6): the changes of an appointment against api/openapi.yaml.
// Tuesday 2026-11-24 in Cali (UTC-5); the test clock starts Monday 2026-11-23 12:00 UTC.
func TestContractAppointmentChanges(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	owner := e.signIn(t, "dueno@example.com")
	barber := e.signIn(t, "juan@example.com")
	ana := e.signIn(t, "ana@example.com")
	bob := e.signIn(t, "bob@example.com")
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	biz := idOf(c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Cambios","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, owner, http.StatusCreated))
	base := "/api/v1/business/" + biz
	c.callAuth(t, e.srv, http.MethodPost, base+"/members", `{"email":"juan@example.com","role":"PROFESSIONAL"}`, owner, http.StatusCreated)
	juan := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"juan@example.com","displayName":"Juan"}`, owner, http.StatusCreated))
	boss := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"dueno@example.com","displayName":"El Dueño"}`, owner, http.StatusCreated))
	svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, owner, http.StatusCreated))
	for _, p := range []string{juan, boss} {
		c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+p+"/schedule", `{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"09:00","end":"13:00"}]}]}`, owner, http.StatusOK)
		c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+p+"/services", `{"serviceIds":["`+svc+`"]}`, owner, http.StatusOK)
	}
	key := func(k string) map[string]string { return map[string]string{"Idempotency-Key": k} }
	appt := idOf(c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", `{"businessId":"`+biz+`","serviceId":"`+svc+`","professionalId":"`+juan+`","startAt":"2026-11-24T14:00:00Z"}`, ana, key("ana-cambios-0001"), http.StatusCreated))
	walk := idOf(c.callWith(t, e.srv, http.MethodPost, base+"/appointments", `{"professionalId":"`+boss+`","serviceId":"`+svc+`","startAt":"2026-11-24T15:00:00Z","customer":{"fullName":"Laura","phone":"3115550303"}}`, owner, key("owner-cambios-001"), http.StatusCreated))
	post := func(tok, path, body string, want int) string {
		if body == "" {
			return c.callAuth(t, e.srv, http.MethodPost, path, "", tok, want)
		}
		return c.callAuth(t, e.srv, http.MethodPost, path, body, tok, want)
	}
	mine := "/api/v1/me/appointments/" + appt
	staff := func(id string) string { return base + "/appointments/" + id }

	// The customer.
	if got := post(ana, mine+"/confirm", "", http.StatusOK); !strings.Contains(got, `"status":"CONFIRMED"`) || !strings.Contains(got, `"version":2`) {
		t.Errorf("confirm = %s", got)
	}
	post(bob, mine+"/confirm", "", http.StatusNotFound)
	if got := post(ana, mine+"/reschedule", `{"startAt":"2026-11-24T14:30:00Z"}`, http.StatusOK); !strings.Contains(got, `"startAt":"2026-11-24T14:30:00Z"`) || !strings.Contains(got, `"status":"CONFIRMED"`) {
		t.Errorf("customer reschedule = %s", got)
	}
	// The staff.
	if got := post(barber, staff(appt)+"/reschedule", `{"startAt":"2026-11-24T15:30:00Z"}`, http.StatusOK); !strings.Contains(got, `"status":"PENDING_CONFIRMATION"`) {
		t.Errorf("staff reschedule of an app customer = %s", got)
	}
	post(barber, staff(appt)+"/cancel", `{"reason":""}`, http.StatusBadRequest)
	post(barber, staff(walk)+"/confirm", "", http.StatusForbidden)
	post(barber, staff(walk)+"/reschedule", `{"startAt":"2026-11-24T16:00:00Z","professionalId":"`+juan+`"}`, http.StatusForbidden)
	if got := post(owner, staff(walk)+"/reschedule", `{"startAt":"2026-11-24T16:00:00Z","professionalId":"`+juan+`"}`, http.StatusOK); !strings.Contains(got, `"professionalId":"`+juan+`"`) || !strings.Contains(got, `"status":"CONFIRMED"`) {
		t.Errorf("owner reassigns a walk-in = %s", got)
	}
	if h := c.callAuth(t, e.srv, http.MethodGet, staff(walk)+"/history", "", owner, http.StatusOK); !strings.Contains(h, `"type":"REASSIGNED"`) || !strings.Contains(h, `"previousProfessionalId":"`+boss+`"`) {
		t.Errorf("history = %s", h)
	}
	// Cut-off: Tuesday 08:50 in Cali, the appointment is at 10:30 (less than 2 hours).
	e.clock.Advance(time.Date(2026, 11, 24, 13, 50, 0, 0, time.UTC).Sub(e.clock.Now()))
	if p := post(ana, mine+"/cancel", `{"reason":"viaje"}`, http.StatusConflict); !strings.Contains(p, "CUTOFF_PASSED") {
		t.Errorf("cut-off = %s", p)
	}
	if p := post(barber, staff(appt)+"/start", "", http.StatusConflict); !strings.Contains(p, "TOO_EARLY") {
		t.Errorf("start 40 min early = %s", p)
	}
	// The day: start, complete; a no-show.
	e.clock.Advance(time.Date(2026, 11, 24, 15, 20, 0, 0, time.UTC).Sub(e.clock.Now()))
	post(barber, staff(appt)+"/start", "", http.StatusOK)
	if got := post(barber, staff(appt)+"/complete", "", http.StatusOK); !strings.Contains(got, `"status":"COMPLETED"`) {
		t.Errorf("complete = %s", got)
	}
	if p := post(owner, staff(appt)+"/cancel", `{"reason":"tarde"}`, http.StatusConflict); !strings.Contains(p, "INVALID_TRANSITION") {
		t.Errorf("cancel a completed one = %s", p)
	}
	e.clock.Advance(time.Date(2026, 11, 24, 16, 5, 0, 0, time.UTC).Sub(e.clock.Now()))
	if p := post(owner, staff(walk)+"/reschedule", `{"startAt":"2026-11-24T17:00:00Z"}`, http.StatusConflict); !strings.Contains(p, "APPOINTMENT_STARTED") {
		t.Errorf("move after the start = %s", p)
	}
	if got := post(barber, staff(walk)+"/no-show", "", http.StatusOK); !strings.Contains(got, `"status":"NO_SHOW"`) {
		t.Errorf("no-show = %s", got)
	}
}
