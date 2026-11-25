package app

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
)

// TestContractAgendaCoherence (T6.7): absences and schedules over appointments are refused with
// the list (409 AGENDA_CONFLICTS); a deactivated professional leaves appointments to reassign.
func TestContractAgendaCoherence(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	owner := e.signIn(t, "dueno@example.com")
	ana := e.signIn(t, "ana@example.com")
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	biz := idOf(c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Coherente","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, owner, http.StatusCreated))
	base := "/api/v1/business/" + biz
	juan := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"dueno@example.com","displayName":"Juan"}`, owner, http.StatusCreated))
	svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, owner, http.StatusCreated))
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+juan+"/schedule", `{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"09:00","end":"12:00"}]}]}`, owner, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+juan+"/services", `{"serviceIds":["`+svc+`"]}`, owner, http.StatusOK)
	appt := idOf(c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", `{"businessId":"`+biz+`","serviceId":"`+svc+`","professionalId":"`+juan+`","startAt":"2026-11-24T15:00:00Z"}`, ana, map[string]string{"Idempotency-Key": "ana-coherencia-1"}, http.StatusCreated))

	abs := base + "/professionals/" + juan + "/absences"
	p := c.callAuth(t, e.srv, http.MethodPost, abs, `{"start":"2026-11-24T14:30:00Z","end":"2026-11-24T16:00:00Z","reason":"Médico"}`, owner, http.StatusConflict)
	if !strings.Contains(p, `"code":"AGENDA_CONFLICTS"`) || !strings.Contains(p, `"id":"`+appt+`"`) || !strings.Contains(p, `"customerName":"QA"`) {
		t.Errorf("absence over an appointment = %s", p)
	}
	c.callAuth(t, e.srv, http.MethodPost, abs, `{"start":"2026-11-24T15:30:00Z","end":"2026-11-24T16:00:00Z"}`, owner, http.StatusCreated)
	if p := c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+juan+"/schedule", `{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"11:00","end":"12:00"}]}]}`, owner, http.StatusConflict); !strings.Contains(p, appt) {
		t.Errorf("schedule leaving it out = %s", p)
	}

	if got := c.callAuth(t, e.srv, http.MethodGet, base+"/appointments/to-reassign", "", owner, http.StatusOK); !strings.Contains(got, `"items":[]`) {
		t.Errorf("nothing to reassign yet = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodPatch, base+"/professionals/"+juan, `{"status":"INACTIVE"}`, owner, http.StatusOK)
	if got := c.callAuth(t, e.srv, http.MethodGet, base+"/appointments/to-reassign", "", owner, http.StatusOK); !strings.Contains(got, `"id":"`+appt+`"`) || !strings.Contains(got, `"needsReassignment":true`) {
		t.Errorf("to reassign = %s", got)
	}
	if got := c.callAuth(t, e.srv, http.MethodGet, base+"/agenda?date=2026-11-24", "", owner, http.StatusOK); !strings.Contains(got, `"needsReassignment":true`) {
		t.Errorf("agenda flag = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodGet, base+"/appointments/to-reassign", "", ana, http.StatusNotFound) // not a member
}
