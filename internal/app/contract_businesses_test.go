package app

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
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
	for _, want := range []string{`"status":"PENDING_REVIEW"`, `"slug":"barberia-el-paisa"`, `"contactPhone":"+573001234567"`, `"currency":"COP"`, `"timeZone":"America/Bogota"`} {
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
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/admin/businesses?status=PENDING_REVIEW&cityId="+cali, "", adminTok, http.StatusOK)
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

	if got := e.biz.AuditActions(); len(got) != 4 {
		t.Errorf("business audit = %v, want created×2, activated, suspended", got)
	}
}
