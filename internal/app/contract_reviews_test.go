package app

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	notifdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
)

// TestContractReviews (2026-11-28): the customer of a completed appointment reviews it, the owner
// replies, the public sees the reviews and the ratings, an administrator hides an abusive one;
// every response against api/openapi.yaml. Tuesday 2026-11-24 in Cali; the clock starts Monday.
func TestContractReviews(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	owner := e.signIn(t, "dueno@example.com")
	barber := e.signIn(t, "juan@example.com")
	ana := e.signIn(t, "ana@example.com")
	bob := e.signIn(t, "bob@example.com")
	admin := e.signIn(t, "admin@example.com")
	if err := e.identity.SetPlatformAdmin(context.Background(), "admin@example.com", true); err != nil {
		t.Fatal(err)
	}
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	created := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Reseñas","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, owner, http.StatusCreated)
	biz, slug := idOf(created), regexp.MustCompile(`"slug":"([^"]+)"`).FindStringSubmatch(created)[1]
	base := "/api/v1/business/" + biz
	c.callAuth(t, e.srv, http.MethodPost, base+"/members", `{"email":"juan@example.com","role":"PROFESSIONAL"}`, owner, http.StatusCreated)
	juan := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"juan@example.com","displayName":"Juan"}`, owner, http.StatusCreated))
	svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, owner, http.StatusCreated))
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+juan+"/schedule", `{"days":[{"weekday":"TUESDAY","workingHours":[{"start":"09:00","end":"12:00"}]}]}`, owner, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+juan+"/services", `{"serviceIds":["`+svc+`"]}`, owner, http.StatusOK)
	appt := idOf(c.callWith(t, e.srv, http.MethodPost, "/api/v1/appointments", `{"businessId":"`+biz+`","serviceId":"`+svc+`","professionalId":"`+juan+`","startAt":"2026-11-24T14:00:00Z"}`, ana, map[string]string{"Idempotency-Key": "ana-resena-0001"}, http.StatusCreated))

	mine := "/api/v1/me/appointments/" + appt + "/review"
	review := `{"businessRating":5,"professionalRating":4,"comment":"Excelente corte"}`
	if got := c.callAuth(t, e.srv, http.MethodPut, mine, review, ana, http.StatusConflict); !strings.Contains(got, `"NOT_REVIEWABLE"`) {
		t.Errorf("before completing = %s", got)
	}
	if got := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/me/appointments/"+appt, "", ana, http.StatusOK); !strings.Contains(got, `"review":null,"reviewableUntil":null`) {
		t.Errorf("pending appointment = %s", got)
	}
	e.clock.Advance(time.Date(2026, 11, 24, 14, 5, 0, 0, time.UTC).Sub(e.clock.Now()))
	c.callAuth(t, e.srv, http.MethodPost, base+"/appointments/"+appt+"/start", "", barber, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPost, base+"/appointments/"+appt+"/complete", "", barber, http.StatusOK)
	asked := 0
	for _, m := range e.outbox.Messages() {
		if m.Kind == notifdomain.KindReviewRequest && m.AppointmentID.String() == appt && m.Channel == notifdomain.ChannelEmail {
			asked++
		}
	}
	if asked != 1 {
		t.Errorf("review requests queued: %d", asked)
	}

	c.callAuth(t, e.srv, http.MethodPut, mine, review, bob, http.StatusNotFound)
	if got := c.callAuth(t, e.srv, http.MethodPut, mine, `{"businessRating":0,"professionalRating":5}`, ana, http.StatusBadRequest); !strings.Contains(got, `"INVALID_RATING"`) {
		t.Errorf("invalid rating = %s", got)
	}
	if got := c.callAuth(t, e.srv, http.MethodPut, mine, review, ana, http.StatusCreated); !strings.Contains(got, `"comment":"Excelente corte"`) || !strings.Contains(got, `"reply":null`) {
		t.Errorf("review = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodPut, mine, `{"businessRating":5,"professionalRating":5}`, ana, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, mine, "", ana, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, mine, "", bob, http.StatusNotFound)
	if got := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/me/appointments/"+appt, "", ana, http.StatusOK); !strings.Contains(got, `"reviewableUntil":"2026-12-24T14:30:00Z"`) || !strings.Contains(got, `"businessRating":5`) {
		t.Errorf("completed appointment = %s", got)
	}

	// Public: the list and the ratings in the business, the search and the catalog.
	public := "/api/v1/businesses/" + slug + "/reviews"
	if got := c.callAuth(t, e.srv, http.MethodGet, public, "", "", http.StatusOK); !strings.Contains(got, `"rating":{"average":5,"count":1}`) ||
		!strings.Contains(got, `"authorName":"QA"`) || !strings.Contains(got, `"name":"Juan"`) || strings.Contains(got, `"hidden"`) || strings.Contains(got, appt) {
		t.Errorf("public reviews = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodGet, public+"?professionalId="+juan+"&limit=1", "", "", http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, public+"?limit=99", "", "", http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/no-existe/reviews", "", "", http.StatusNotFound)
	if got := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/"+slug, "", "", http.StatusOK); !strings.Contains(got, `"rating":{"average":5,"count":1}`) {
		t.Errorf("public business = %s", got)
	}
	if got := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses?q=Rese&cityId="+geotest.CaliID.String(), "", "", http.StatusOK); !strings.Contains(got, `"rating":{"average":5,"count":1}`) {
		t.Errorf("search = %s", got)
	}
	if got := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/"+slug+"/catalog", "", "", http.StatusOK); !strings.Contains(got, `"rating":{"average":5,"count":1}`) {
		t.Errorf("catalog = %s", got)
	}

	// Staff: the owner and the barber read them; the owner replies.
	rev := idOf(c.callAuth(t, e.srv, http.MethodGet, base+"/reviews", "", owner, http.StatusOK))
	if got := c.callAuth(t, e.srv, http.MethodGet, base+"/reviews", "", barber, http.StatusOK); !strings.Contains(got, `"hidden":false`) || !strings.Contains(got, appt) {
		t.Errorf("barber's reviews = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodGet, base+"/reviews", "", ana, http.StatusNotFound)
	reply := base + "/reviews/" + rev + "/reply"
	c.callAuth(t, e.srv, http.MethodPut, reply, `{"reply":"Gracias"}`, barber, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodPut, reply, `{"reply":""}`, owner, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPut, base+"/reviews/01a20aae-0000-7000-8000-000000000000/reply", `{"reply":"Gracias"}`, owner, http.StatusNotFound)
	if got := c.callAuth(t, e.srv, http.MethodPut, reply, `{"reply":"¡Gracias por venir!"}`, owner, http.StatusOK); !strings.Contains(got, `"text":"¡Gracias por venir!"`) {
		t.Errorf("reply = %s", got)
	}
	if got := c.callAuth(t, e.srv, http.MethodGet, public, "", "", http.StatusOK); !strings.Contains(got, `"text":"¡Gracias por venir!"`) {
		t.Errorf("public reply = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodDelete, reply, "", owner, http.StatusOK)

	// Hiding: only a platform administrator, with a reason.
	hide := "/api/v1/admin/reviews/" + rev + "/hide"
	c.callAuth(t, e.srv, http.MethodPost, hide, `{"reason":"Ofensiva"}`, owner, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodPost, hide, `{"reason":""}`, admin, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/reviews/01a20aae-0000-7000-8000-000000000000/hide", `{"reason":"Ofensiva"}`, admin, http.StatusNotFound)
	if got := c.callAuth(t, e.srv, http.MethodPost, hide, `{"reason":"Lenguaje ofensivo"}`, admin, http.StatusOK); !strings.Contains(got, `"hidden":true`) {
		t.Errorf("hide = %s", got)
	}
	if got := c.callAuth(t, e.srv, http.MethodGet, public, "", "", http.StatusOK); !strings.Contains(got, `"rating":{"average":null,"count":0},"items":[]`) {
		t.Errorf("public after hiding = %s", got)
	}
	if got := c.callAuth(t, e.srv, http.MethodGet, base+"/reviews", "", owner, http.StatusOK); !strings.Contains(got, `"hiddenReason":"Lenguaje ofensivo"`) {
		t.Errorf("owner after hiding = %s", got)
	}
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/reviews/"+rev+"/unhide", "", admin, http.StatusOK)

	c.callAuth(t, e.srv, http.MethodDelete, mine, "", bob, http.StatusNotFound)
	c.callAuth(t, e.srv, http.MethodDelete, mine, "", ana, http.StatusNoContent)
	c.callAuth(t, e.srv, http.MethodGet, mine, "", ana, http.StatusNotFound)
	if got := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/"+slug, "", "", http.StatusOK); !strings.Contains(got, `"rating":{"average":null,"count":0}`) {
		t.Errorf("public business without reviews = %s", got)
	}
}
