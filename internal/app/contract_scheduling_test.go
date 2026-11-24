package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
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

// callWith is callAuth with extra request headers.
func (c contract) callWith(t *testing.T, srv *httptest.Server, method, path, body, token string, headers map[string]string, wantStatus int) string {
	t.Helper()
	newReq := func() *http.Request {
		r, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return r
	}
	resp, err := srv.Client().Do(newReq())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: status %d, want %d (%s)", method, path, resp.StatusCode, wantStatus, b)
	}
	resp.Body = io.NopCloser(bytes.NewReader(b))
	if ok, errs := c.v.ValidateHttpResponse(newReq(), resp); !ok {
		for _, e := range errs {
			t.Errorf("%s %s %d does not conform: %s (%s)", method, path, wantStatus, e.Message, e.Reason)
		}
	}
	return string(b)
}

// TestContractBooking (T6.3): phone and booking against api/openapi.yaml. Tuesday 2026-11-24 in
// Cali; the test clock is Monday 2026-11-23 12:00 UTC.
func TestContractBooking(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	tok := e.signIn(t, "dueno@example.com")
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	biz := idOf(c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Citas","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, tok, http.StatusCreated))
	base := "/api/v1/business/" + biz
	prof := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"dueno@example.com","displayName":"El Dueño"}`, tok, http.StatusCreated))
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+prof+"/schedule", `{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"09:00","end":"12:00"}]}]}`, tok, http.StatusOK)
	svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, tok, http.StatusCreated))
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+prof+"/services", `{"serviceIds":["`+svc+`"]}`, tok, http.StatusOK)

	ana := e.signIn(t, "ana@example.com")
	body := func(start string) string {
		return `{"businessId":"` + biz + `","serviceId":"` + svc + `","professionalId":"` + prof + `","startAt":"` + start + `"}`
	}
	key := func(k string) map[string]string { return map[string]string{"Idempotency-Key": k} }
	// The phone comes from the registration (mandatory since T6.4); the customer can change it.
	if me := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/me", "", ana, http.StatusOK); !strings.Contains(me, `"phone":"+573001234567"`) {
		t.Errorf("phone from the registration: %s", me)
	}
	c.callAuth(t, e.srv, http.MethodPut, "/api/v1/me/phone", `{"phone":"12345"}`, ana, http.StatusBadRequest)
	if me := c.callAuth(t, e.srv, http.MethodPut, "/api/v1/me/phone", `{"phone":"300 765-4321"}`, ana, http.StatusOK); !strings.Contains(me, `"phone":"+573007654321"`) {
		t.Errorf("change phone: %s", me)
	}

	c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", body("2026-11-24T14:00:00Z"), ana, nil, http.StatusBadRequest) // no key
	created := c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", body("2026-11-24T14:00:00Z"), ana, key("ana-cita-0001"), http.StatusCreated)
	for _, want := range []string{`"status":"PENDING_CONFIRMATION"`, `"serviceName":"Corte"`, `"priceMinor":2500000`, `"endAt":"2026-11-24T14:30:00Z"`} {
		if !strings.Contains(created, want) {
			t.Errorf("appointment %s lacks %s", created, want)
		}
	}
	if replay := c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", body("2026-11-24T14:00:00Z"), ana, key("ana-cita-0001"), http.StatusCreated); idOf(replay) != idOf(created) {
		t.Errorf("replay = %s", replay)
	}
	if p := c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", body("2026-11-24T15:00:00Z"), ana, key("ana-cita-0001"), http.StatusUnprocessableEntity); !strings.Contains(p, "IDEMPOTENCY_KEY_REUSED") {
		t.Errorf("key reused: %s", p)
	}
	bob := e.signIn(t, "bob@example.com")
	c.callAuth(t, e.srv, http.MethodPut, "/api/v1/me/phone", `{"phone":"+573004445566"}`, bob, http.StatusOK)
	if p := c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", body("2026-11-24T14:10:00Z"), bob, key("bob-cita-0001"), http.StatusConflict); !strings.Contains(p, "SLOT_UNAVAILABLE") {
		t.Errorf("taken slot: %s", p)
	}
	if p := c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", body("2026-11-20T14:00:00Z"), bob, key("bob-cita-0002"), http.StatusBadRequest); !strings.Contains(p, "DATE_OUTSIDE_BOOKING_WINDOW") {
		t.Errorf("past: %s", p)
	}
	unknown := `{"businessId":"01a20aae-0000-7000-8000-000000000000","serviceId":"` + svc + `","professionalId":"` + prof + `","startAt":"2026-11-24T15:00:00Z"}`
	c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", unknown, bob, key("bob-cita-0003"), http.StatusNotFound)
	c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", `{"businessId":"`+biz+`"}`, bob, key("bob-cita-0004"), http.StatusBadRequest)
	c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", body("2026-11-24T15:00:00Z"), "", key("anonimo-0001"), http.StatusUnauthorized)
	c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", body("2026-11-24T14:30:00Z"), bob, key("bob-cita-0005"), http.StatusCreated)

	// The availability no longer offers 09:00–09:50 (09:00 and 09:30 are booked, 30 min each).
	got := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/barberia-citas/availability?serviceId="+svc+"&professionalId="+prof+"&date=2026-11-24", "", "", http.StatusOK)
	if !strings.Contains(got, `"startLocal":"2026-11-24T10:00:00-05:00"`) || strings.Contains(got, `T09:`) {
		t.Errorf("availability after bookings = %s", got)
	}
}

// TestContractStaffBooking (T6.4): customers of the business and manual appointments against
// api/openapi.yaml. Tuesday 2026-11-24 in Cali.
func TestContractStaffBooking(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	owner := e.signIn(t, "dueno@example.com")
	barber := e.signIn(t, "juan@example.com")
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	biz := idOf(c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Manual","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, owner, http.StatusCreated))
	base := "/api/v1/business/" + biz
	c.callAuth(t, e.srv, http.MethodPost, base+"/members", `{"email":"juan@example.com","role":"PROFESSIONAL"}`, owner, http.StatusCreated)
	juan := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"juan@example.com","displayName":"Juan"}`, owner, http.StatusCreated))
	boss := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"dueno@example.com","displayName":"El Dueño"}`, owner, http.StatusCreated))
	svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, owner, http.StatusCreated))
	for _, p := range []string{juan, boss} {
		c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+p+"/schedule", `{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"09:00","end":"12:00"}]}]}`, owner, http.StatusOK)
		c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+p+"/services", `{"serviceIds":["`+svc+`"]}`, owner, http.StatusOK)
	}

	// Customers: the barber adds one; the mobile is unique per business.
	created := c.callAuth(t, e.srv, http.MethodPost, base+"/customers", `{"fullName":"Carlos Pérez","phone":"300 555 0101"}`, barber, http.StatusCreated)
	if !strings.Contains(created, `"phone":"+573005550101"`) || !strings.Contains(created, `"hasAccount":false`) {
		t.Errorf("customer = %s", created)
	}
	carlos := idOf(created)
	if p := c.callAuth(t, e.srv, http.MethodPost, base+"/customers", `{"fullName":"Otro","phone":"+573005550101"}`, owner, http.StatusConflict); !strings.Contains(p, "CUSTOMER_PHONE_TAKEN") {
		t.Errorf("duplicate: %s", p)
	}
	c.callAuth(t, e.srv, http.MethodPost, base+"/customers", `{"fullName":"","phone":"x"}`, owner, http.StatusBadRequest)
	if list := c.callAuth(t, e.srv, http.MethodGet, base+"/customers?q=carlos", "", barber, http.StatusOK); !strings.Contains(list, carlos) {
		t.Errorf("search = %s", list)
	}
	if upd := c.callAuth(t, e.srv, http.MethodPatch, base+"/customers/"+carlos, `{"fullName":"Carlos P."}`, owner, http.StatusOK); !strings.Contains(upd, `"fullName":"Carlos P."`) {
		t.Errorf("update = %s", upd)
	}
	c.callAuth(t, e.srv, http.MethodPatch, base+"/customers/01a20aae-0000-7000-8000-000000000000", `{"fullName":"X"}`, owner, http.StatusNotFound)

	book := func(tok, prof, start, key, confirmed string, want int) string {
		body := `{"professionalId":"` + prof + `","serviceId":"` + svc + `","customerId":"` + carlos + `","startAt":"` + start + `"` + confirmed + `}`
		return c.callWith(t, e.srv, http.MethodPost, base+"/appointments", body, tok, map[string]string{"Idempotency-Key": key}, want)
	}
	if a := book(barber, juan, "2026-11-24T14:00:00Z", "juan-manual-1", `,"confirmed":false`, http.StatusCreated); !strings.Contains(a, `"status":"PENDING_CONFIRMATION"`) || !strings.Contains(a, `"customerName":"Carlos P."`) {
		t.Errorf("barber, own agenda = %s", a)
	}
	book(barber, boss, "2026-11-24T14:00:00Z", "juan-manual-2", "", http.StatusForbidden) // another agenda
	if a := book(owner, juan, "2026-11-24T15:00:00Z", "owner-manual-1", "", http.StatusCreated); !strings.Contains(a, `"status":"CONFIRMED"`) {
		t.Errorf("owner, default confirmed = %s", a)
	}
	if p := book(owner, juan, "2026-11-24T14:10:00Z", "owner-manual-2", "", http.StatusConflict); !strings.Contains(p, "SLOT_UNAVAILABLE") {
		t.Errorf("overlap: %s", p)
	}
	c.callWith(t, e.srv, http.MethodPost, base+"/appointments", `{"professionalId":"`+juan+`","serviceId":"`+svc+`","customerId":"01a20aae-0000-7000-8000-000000000000","startAt":"2026-11-24T16:00:00Z"}`, owner, map[string]string{"Idempotency-Key": "owner-manual-3"}, http.StatusBadRequest)
	c.callWith(t, e.srv, http.MethodPost, base+"/appointments", `{"professionalId":"`+juan+`"}`, owner, map[string]string{"Idempotency-Key": "owner-manual-4"}, http.StatusBadRequest)
	book(owner, juan, "2026-11-24T16:00:00Z", "", "", http.StatusBadRequest) // no key
}
