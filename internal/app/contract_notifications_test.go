package app

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
)

// TestContractAppointmentNotifications (T7.1): a booking queues its WhatsApp message; the staff
// see it with the number masked; others get the same 404 or 403 as the history.
func TestContractAppointmentNotifications(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	owner := e.signIn(t, "dueno@example.com")
	ana := e.signIn(t, "ana@example.com")
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	biz := idOf(c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Avisos","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, owner, http.StatusCreated))
	base := "/api/v1/business/" + biz
	juan := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"dueno@example.com","displayName":"Juan"}`, owner, http.StatusCreated))
	svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, owner, http.StatusCreated))
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+juan+"/schedule", `{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"09:00","end":"12:00"}]}]}`, owner, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+juan+"/services", `{"serviceIds":["`+svc+`"]}`, owner, http.StatusOK)
	appt := idOf(c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", `{"businessId":"`+biz+`","serviceId":"`+svc+`","professionalId":"`+juan+`","startAt":"2026-11-24T15:00:00Z"}`, ana, map[string]string{"Idempotency-Key": "ana-avisos-1"}, http.StatusCreated))

	path := base + "/appointments/" + appt + "/notifications"
	got := c.callAuth(t, e.srv, http.MethodGet, path, "", owner, http.StatusOK)
	if !strings.Contains(got, `"kind":"APPOINTMENT_CREATED"`) || !strings.Contains(got, `"channel":"WHATSAPP"`) ||
		!strings.Contains(got, `"status":"PENDING"`) || !strings.Contains(got, `"to":"+57••••••4567"`) || strings.Contains(got, "3001234567") {
		t.Errorf("notifications = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodGet, path, "", ana, http.StatusNotFound) // not a member
	c.callAuth(t, e.srv, http.MethodGet, base+"/appointments/"+biz+"/notifications", "", owner, http.StatusNotFound)
}
