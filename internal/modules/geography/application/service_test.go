package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	geocontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

var (
	admin = contract.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	user  = contract.Principal{UserID: uuid.Must(uuid.NewV7())}
)

func newService() (*application.Service, *geotest.Store) {
	st := geotest.NewSeededStore()
	return application.NewService(st, geotest.Authorizer{}, st, clock.NewFixed(time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC))), st
}

func ptr[T any](v T) *T { return &v }

func fieldCodes(t *testing.T, err error) map[string]string {
	t.Helper()
	var verr *application.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	out := map[string]string{}
	for _, f := range verr.Fields {
		out[f.Field] = f.Code
	}
	return out
}

func TestPublicReadsReturnActiveDataOnly(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()

	cs, err := svc.ListCountries(ctx)
	if err != nil || len(cs) != 1 || cs[0].Code != "CO" {
		t.Fatalf("ListCountries = %+v, %v", cs, err)
	}
	ds, err := svc.ListDivisions(ctx, "CO")
	if err != nil || len(ds) != 2 || ds[0].Name != "Antioquia" {
		t.Fatalf("ListDivisions = %+v, %v (want 2 sorted by name)", ds, err)
	}
	cities, err := svc.ListCities(ctx, geotest.ValleID)
	if err != nil || len(cities) != 1 || cities[0].Name != "Cali" {
		t.Fatalf("ListCities = %+v, %v", cities, err)
	}

	// Unknown, malformed (path) or lower-case codes and unknown divisions: 404.
	for _, code := range []string{"MX", "co", "C", "../x"} {
		if _, err := svc.ListDivisions(ctx, code); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("ListDivisions(%q) err = %v, want ErrNotFound", code, err)
		}
	}
	if _, err := svc.ListCities(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("ListCities(unknown) err = %v", err)
	}

	// Deactivating hides the record and everything below it.
	if _, err := svc.UpdateDivision(ctx, admin, geotest.ValleID, application.DivisionPatch{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListCities(ctx, geotest.ValleID); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("cities of an inactive division err = %v, want ErrNotFound", err)
	}
	if ds, _ := svc.ListDivisions(ctx, "CO"); len(ds) != 1 {
		t.Errorf("inactive division still listed: %+v", ds)
	}
	if _, err := svc.UpdateCountry(ctx, admin, "CO", application.CountryPatch{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListDivisions(ctx, "CO"); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("divisions of an inactive country err = %v", err)
	}
	if _, err := svc.ListCities(ctx, geotest.AntioquiaID); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("cities of an inactive country err = %v", err)
	}
	if cs, _ := svc.ListCountries(ctx); len(cs) != 0 {
		t.Errorf("inactive country listed: %+v", cs)
	}
}

func TestWritesRequirePlatformAdmin(t *testing.T) {
	ctx := context.Background()
	svc, st := newService()
	calls := map[string]func(p contract.Principal) error{
		"CreateCountry": func(p contract.Principal) error {
			_, err := svc.CreateCountry(ctx, p, application.CountryInput{Code: "MX", Name: "México", PhoneCode: "+52", DefaultCurrency: "USD"})
			return err
		},
		"UpdateCountry": func(p contract.Principal) error {
			_, err := svc.UpdateCountry(ctx, p, "CO", application.CountryPatch{Name: ptr("X")})
			return err
		},
		"CreateDivision": func(p contract.Principal) error {
			_, err := svc.CreateDivision(ctx, p, application.DivisionInput{CountryCode: "CO", Name: "Cauca", Type: "DEPARTMENT"})
			return err
		},
		"UpdateDivision": func(p contract.Principal) error {
			_, err := svc.UpdateDivision(ctx, p, geotest.ValleID, application.DivisionPatch{Name: ptr("X")})
			return err
		},
		"CreateCity": func(p contract.Principal) error {
			_, err := svc.CreateCity(ctx, p, application.CityInput{DivisionID: geotest.ValleID, Name: "Palmira", TimeZone: "America/Bogota"})
			return err
		},
		"UpdateCity": func(p contract.Principal) error {
			_, err := svc.UpdateCity(ctx, p, geotest.CaliID, application.CityPatch{Name: ptr("X")})
			return err
		},
	}
	for name, call := range calls {
		for _, p := range []contract.Principal{user, {}} {
			if err := call(p); !errors.Is(err, authz.ErrForbidden) {
				t.Errorf("%s as non-admin: err = %v, want ErrForbidden", name, err)
			}
		}
	}
	if len(st.Audit) != 0 {
		t.Errorf("denied writes must not change data or audit here: %v", st.AuditActions())
	}
	if cs, _ := svc.ListCountries(ctx); len(cs) != 1 || cs[0].Name != "Colombia" {
		t.Errorf("data changed by a denied call: %+v", cs)
	}
}

func TestAdminCreatesAndUpdatesWithAudit(t *testing.T) {
	ctx := context.Background()
	svc, st := newService()

	c, err := svc.CreateCountry(ctx, admin, application.CountryInput{Code: "us", Name: " Estados Unidos ", PhoneCode: "+1", DefaultCurrency: "usd"})
	if err != nil || c.Code != "US" || c.Name != "Estados Unidos" || c.DefaultCurrency != "USD" || !c.Active {
		t.Fatalf("CreateCountry = %+v, %v", c, err)
	}
	d, err := svc.CreateDivision(ctx, admin, application.DivisionInput{CountryCode: "CO", Code: "co-cau", Name: "Cauca", Type: "department"})
	if err != nil || d.Code != "CO-CAU" || d.Type != "DEPARTMENT" || d.ID.Version() != 7 {
		t.Fatalf("CreateDivision = %+v, %v", d, err)
	}
	city, err := svc.CreateCity(ctx, admin, application.CityInput{DivisionID: d.ID, Name: "Popayán", TimeZone: "America/Bogota", Active: ptr(false)})
	if err != nil || city.Active || city.ID.Version() != 7 {
		t.Fatalf("CreateCity = %+v, %v", city, err)
	}
	if got, _ := svc.ListCities(ctx, d.ID); len(got) != 0 {
		t.Errorf("inactive city listed: %+v", got)
	}
	city, err = svc.UpdateCity(ctx, admin, city.ID, application.CityPatch{Active: ptr(true), Name: ptr("Popayán")})
	if err != nil || !city.Active {
		t.Fatalf("UpdateCity = %+v, %v", city, err)
	}
	d, err = svc.UpdateDivision(ctx, admin, d.ID, application.DivisionPatch{Code: ptr("")})
	if err != nil || d.Code != "" {
		t.Fatalf("removing the division code: %+v, %v", d, err)
	}
	// A patch that changes nothing is a no-op (no audit entry).
	if _, err := svc.UpdateCountry(ctx, admin, "US", application.CountryPatch{Name: ptr("Estados Unidos")}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		application.AuditCountryCreated, application.AuditDivisionCreated, application.AuditCityCreated,
		application.AuditCityUpdated, application.AuditDivisionUpdated,
	}
	if got := st.AuditActions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("audit = %v, want %v", got, want)
	}
	for _, e := range st.Audit {
		if e.ActorUserID == nil || *e.ActorUserID != admin.UserID || e.Outcome != "SUCCESS" || e.ResourceID == "" {
			t.Errorf("audit entry %+v: want actor, SUCCESS and resource", e)
		}
	}
	if got := st.Audit[3].Metadata["fields"]; got != "active" {
		t.Errorf("city update fields = %q, want only the changed field", got)
	}
}

func TestValidationAndConflicts(t *testing.T) {
	ctx := context.Background()
	svc, st := newService()

	_, err := svc.CreateCountry(ctx, admin, application.CountryInput{Code: "Colombia", PhoneCode: "57", DefaultCurrency: "EUR"})
	want := map[string]string{
		"code": application.CodeInvalidCountryCode, "name": application.CodeRequired,
		"phoneCode": application.CodeInvalidPhoneCode, "defaultCurrency": application.CodeUnsupportedCurrency,
	}
	if got := fieldCodes(t, err); !reflect.DeepEqual(got, want) {
		t.Errorf("CreateCountry fields = %v, want %v", got, want)
	}

	_, err = svc.CreateDivision(ctx, admin, application.DivisionInput{CountryCode: "CO", Code: "MX-JAL", Name: "x", Type: "COUNTY"})
	want = map[string]string{"code": application.CodeInvalidDivisionCode, "type": application.CodeInvalidDivisionType}
	if got := fieldCodes(t, err); !reflect.DeepEqual(got, want) {
		t.Errorf("CreateDivision fields = %v, want %v", got, want)
	}
	_, err = svc.CreateDivision(ctx, admin, application.DivisionInput{CountryCode: "PE", Name: "Lima", Type: "REGION"})
	if got := fieldCodes(t, err); got["countryCode"] != application.CodeUnknownCountry {
		t.Errorf("unknown country: %v", got)
	}

	_, err = svc.CreateCity(ctx, admin, application.CityInput{Name: "x", TimeZone: "Mars/Olympus"})
	want = map[string]string{"divisionId": application.CodeRequired, "timeZone": application.CodeInvalidTimeZone}
	if got := fieldCodes(t, err); !reflect.DeepEqual(got, want) {
		t.Errorf("CreateCity fields = %v, want %v", got, want)
	}
	_, err = svc.CreateCity(ctx, admin, application.CityInput{DivisionID: uuid.Must(uuid.NewV7()), Name: "x", TimeZone: "UTC"})
	if got := fieldCodes(t, err); got["divisionId"] != application.CodeUnknownDivision {
		t.Errorf("unknown division: %v", got)
	}
	_, err = svc.UpdateDivision(ctx, admin, geotest.ValleID, application.DivisionPatch{Code: ptr("MX-JAL")})
	if got := fieldCodes(t, err); got["code"] != application.CodeInvalidDivisionCode {
		t.Errorf("division code of another country: %v", got)
	}

	// Duplicates: country code, division name (case-insensitive) and code, city name.
	if _, err := svc.CreateCountry(ctx, admin, application.CountryInput{Code: "CO", Name: "Colombia", PhoneCode: "+57", DefaultCurrency: "COP"}); !errors.Is(err, application.ErrConflict) {
		t.Errorf("duplicate country err = %v", err)
	}
	if _, err := svc.CreateDivision(ctx, admin, application.DivisionInput{CountryCode: "CO", Name: "VALLE DEL CAUCA", Type: "DEPARTMENT"}); !errors.Is(err, application.ErrConflict) {
		t.Errorf("duplicate division name err = %v", err)
	}
	if _, err := svc.UpdateDivision(ctx, admin, geotest.AntioquiaID, application.DivisionPatch{Code: ptr("CO-VAC")}); !errors.Is(err, application.ErrConflict) {
		t.Errorf("duplicate division code err = %v", err)
	}
	if _, err := svc.CreateCity(ctx, admin, application.CityInput{DivisionID: geotest.ValleID, Name: "cali", TimeZone: "America/Bogota"}); !errors.Is(err, application.ErrConflict) {
		t.Errorf("duplicate city err = %v", err)
	}

	// Unknown targets of updates: 404.
	if _, err := svc.UpdateCountry(ctx, admin, "PE", application.CountryPatch{Name: ptr("Perú")}); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("update unknown country err = %v", err)
	}
	if _, err := svc.UpdateCountry(ctx, admin, "co", application.CountryPatch{Name: ptr("x")}); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("update with lower-case path code err = %v", err)
	}
	if _, err := svc.UpdateCity(ctx, admin, uuid.Must(uuid.NewV7()), application.CityPatch{Name: ptr("x")}); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("update unknown city err = %v", err)
	}
	if _, err := svc.UpdateCity(ctx, admin, geotest.CaliID, application.CityPatch{TimeZone: ptr("Local")}); err == nil {
		t.Error("Local time zone accepted")
	}
	if len(st.Audit) != 0 {
		t.Errorf("failed writes audited: %v", st.AuditActions())
	}
}

func TestDeactivationBlockedByActiveBusinesses(t *testing.T) {
	ctx := context.Background()
	svc, st := newService()
	st.ActiveBusinesses[geotest.CaliID.String()] = 1
	st.ActiveBusinesses[geotest.ValleID.String()] = 1
	st.ActiveBusinesses["CO"] = 1

	if _, err := svc.UpdateCity(ctx, admin, geotest.CaliID, application.CityPatch{Active: ptr(false)}); !errors.Is(err, application.ErrInUse) {
		t.Errorf("city: err = %v, want ErrInUse", err)
	}
	if _, err := svc.UpdateDivision(ctx, admin, geotest.ValleID, application.DivisionPatch{Active: ptr(false)}); !errors.Is(err, application.ErrInUse) {
		t.Errorf("division: err = %v, want ErrInUse", err)
	}
	if _, err := svc.UpdateCountry(ctx, admin, "CO", application.CountryPatch{Active: ptr(false)}); !errors.Is(err, application.ErrInUse) {
		t.Errorf("country: err = %v, want ErrInUse", err)
	}
	// Other changes to an area in use are fine; so is deactivating an area without businesses.
	if _, err := svc.UpdateCity(ctx, admin, geotest.CaliID, application.CityPatch{Name: ptr("Santiago de Cali")}); err != nil {
		t.Errorf("rename city in use: %v", err)
	}
	if _, err := svc.UpdateDivision(ctx, admin, geotest.AntioquiaID, application.DivisionPatch{Active: ptr(false)}); err != nil {
		t.Errorf("deactivate unused division: %v", err)
	}
	if got := st.AuditActions(); len(got) != 2 {
		t.Errorf("audit = %v, want only the 2 successful updates", got)
	}
}

func TestPlaceOfCity(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	p, err := svc.PlaceOfCity(ctx, geotest.CaliID)
	if err != nil || p.DivisionID != geotest.ValleID || p.CountryCode != "CO" || p.TimeZone != "America/Bogota" || p.Currency != "COP" || !p.Active {
		t.Fatalf("PlaceOfCity = %+v, %v", p, err)
	}
	if _, err := svc.UpdateDivision(ctx, admin, geotest.ValleID, application.DivisionPatch{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if p, _ := svc.PlaceOfCity(ctx, geotest.CaliID); p.Active {
		t.Error("a city of an inactive division must not be Active")
	}
	if _, err := svc.PlaceOfCity(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, geocontract.ErrCityNotFound) {
		t.Errorf("unknown city: err = %v", err)
	}
}
