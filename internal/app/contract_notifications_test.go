package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	notifapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	notifdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
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

// TestContractWhatsAppWebhook (T7.5): verification and signed deliveries against the contract.
func TestContractWhatsAppWebhook(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	path := "/webhooks/whatsapp"
	if got := c.callWith(t, e.srv, http.MethodGet, path+"?hub.mode=subscribe&hub.verify_token="+testWebhookVerifyToken+"&hub.challenge=987654", "", "", nil, http.StatusOK); got != "987654" {
		t.Errorf("challenge = %q", got)
	}
	c.callWith(t, e.srv, http.MethodGet, path+"?hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=1", "", "", nil, http.StatusForbidden)
	body := `{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{"statuses":[{"id":"wamid.X","status":"read"}]}}]}]}`
	mac := hmac.New(sha256.New, []byte(testWebhookSecret))
	mac.Write([]byte(body))
	c.callWith(t, e.srv, http.MethodPost, path, body, "", map[string]string{"X-Hub-Signature-256": "sha256=" + hex.EncodeToString(mac.Sum(nil))}, http.StatusOK)
	c.callWith(t, e.srv, http.MethodPost, path, body, "", map[string]string{"X-Hub-Signature-256": "sha256=" + strings.Repeat("0", 64)}, http.StatusUnauthorized)
}

// TestContractAppointmentLinks (T7.7): the pages of the email links (two taps) and the API for the
// frontend's reschedule page, against the contract.
func TestContractAppointmentLinks(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	owner := e.signIn(t, "dueno@example.com")
	ana := e.signIn(t, "ana@example.com")
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	biz := idOf(c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Enlaces","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, owner, http.StatusCreated))
	base := "/api/v1/business/" + biz
	juan := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"dueno@example.com","displayName":"Juan"}`, owner, http.StatusCreated))
	svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, owner, http.StatusCreated))
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+juan+"/schedule", `{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"09:00","end":"12:00"}]}]}`, owner, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+juan+"/services", `{"serviceIds":["`+svc+`"]}`, owner, http.StatusOK)
	appt := idOf(c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", `{"businessId":"`+biz+`","serviceId":"`+svc+`","professionalId":"`+juan+`","startAt":"2026-11-24T15:00:00Z"}`, ana, map[string]string{"Idempotency-Key": "ana-enlaces-1"}, http.StatusCreated))

	// The links of the customer's email (issued as the sender would).
	links := map[notifdomain.Action]string{}
	var email notifdomain.Message
	for _, m := range e.outbox.Messages() {
		if m.Channel == notifdomain.ChannelEmail && m.RecipientRole == notifdomain.RecipientCustomer && m.AppointmentID.String() == appt {
			email = m
		}
	}
	var toks []notifdomain.ActionToken
	for _, a := range []notifdomain.Action{notifdomain.ActionConfirm, notifdomain.ActionReschedule, notifdomain.ActionCancel} {
		raw, hash, err := notifdomain.NewActionToken(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		links[a] = raw
		toks = append(toks, notifdomain.ActionToken{Hash: hash, MessageID: email.ID, BusinessID: email.BusinessID, AppointmentID: email.AppointmentID,
			Action: a, Recipient: "ana@example.com", ExpiresAt: email.Start, CreatedAt: e.clock.Now()})
	}
	if err := e.outbox.WithinTx(t.Context(), func(ctx context.Context, tx notifapp.Tx) error { return tx.InsertActionTokens(ctx, toks) }); err != nil {
		t.Fatal(err)
	}
	status := func() string {
		return regexp.MustCompile(`"status":"([A-Z_]+)"`).FindStringSubmatch(c.callAuth(t, e.srv, http.MethodGet, "/api/v1/me/appointments/"+appt, "", ana, http.StatusOK))[1]
	}
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}

	// First tap: the page, nothing changes.
	page := c.callWith(t, e.srv, http.MethodGet, "/citas/confirmar?token="+url.QueryEscape(links[notifdomain.ActionConfirm]), "", "", nil, http.StatusOK)
	if !strings.Contains(page, "Confirma tu cita") || !strings.Contains(page, `name="token"`) || status() != "PENDING_CONFIRMATION" {
		t.Fatalf("first tap: %s", page)
	}
	// Second tap.
	body := "token=" + url.QueryEscape(links[notifdomain.ActionConfirm])
	if page := c.callWith(t, e.srv, http.MethodPost, "/citas/confirmar", body, "", form, http.StatusOK); !strings.Contains(page, "quedó confirmada") || status() != "CONFIRMED" {
		t.Errorf("second tap: %s", page)
	}
	for _, again := range []string{c.callWith(t, e.srv, http.MethodPost, "/citas/confirmar", body, "", form, http.StatusOK), // used
		c.callWith(t, e.srv, http.MethodGet, "/citas/cancelar?token=nope", "", "", nil, http.StatusOK)} {
		if !strings.Contains(again, "Este enlace ya no es válido") {
			t.Errorf("invalid link page: %s", again)
		}
	}
	c.callWith(t, e.srv, http.MethodGet, "/citas/reagendar?token="+url.QueryEscape(links[notifdomain.ActionReschedule]), "", "", nil, http.StatusOK)

	// The frontend's reschedule page.
	in := func(a notifdomain.Action, extra string) string {
		return `{"token":"` + links[a] + `","action":"` + string(a) + `"` + extra + `}`
	}
	got := c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointment-links/lookup", in(notifdomain.ActionReschedule, ""), "", nil, http.StatusOK)
	if !strings.Contains(got, `"outcome":null`) || !strings.Contains(got, `"serviceId":"`+svc+`"`) || !strings.Contains(got, `"startAt":"2026-11-24T15:00:00Z"`) {
		t.Errorf("lookup: %s", got)
	}
	c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointment-links/apply", in(notifdomain.ActionReschedule, ""), "", nil, http.StatusBadRequest) // no startAt
	c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointment-links/lookup", `{"token":"x","action":"DELETE"}`, "", nil, http.StatusBadRequest)
	got = c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointment-links/apply", in(notifdomain.ActionReschedule, `,"startAt":"2026-11-24T16:00:00Z"`), "", nil, http.StatusOK)
	if !strings.Contains(got, `"outcome":"RESCHEDULED"`) || !strings.Contains(got, `"startAt":"2026-11-24T16:00:00Z"`) {
		t.Errorf("apply: %s", got)
	}
	// The move spent the email's other links.
	if got := c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointment-links/lookup", in(notifdomain.ActionCancel, ""), "", nil, http.StatusOK); !strings.Contains(got, `"outcome":"INVALID","appointment":null`) {
		t.Errorf("after the move: %s", got)
	}
}
