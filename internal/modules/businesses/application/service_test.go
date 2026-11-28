package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application/biztest"
	bizcontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	geography "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

var (
	admin    = identity.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	someone  = identity.Principal{UserID: uuid.Must(uuid.NewV7())}
	cali     = uuid.Must(uuid.NewV7())
	closed   = uuid.Must(uuid.NewV7())
	valle    = uuid.Must(uuid.NewV7())
	caliCity = geography.Place{CityID: cali, DivisionID: valle, CountryCode: "CO", TimeZone: "America/Bogota", Currency: "COP", Active: true}
)

type env struct {
	svc     *application.Service
	store   *biztest.Store
	members *biztest.Members
	owners  *biztest.Owners
	trials  *biztest.Trials
	places  biztest.Places
}

func newEnv() env {
	e := env{store: biztest.NewStore(), members: &biztest.Members{}, owners: &biztest.Owners{}, trials: &biztest.Trials{}, places: biztest.Places{
		cali:   caliCity,
		closed: {CityID: closed, DivisionID: valle, CountryCode: "CO", TimeZone: "America/Bogota", Currency: "COP", Active: false},
	}}
	e.svc = application.NewService(application.Deps{
		Store: e.store, Authz: biztest.Authorizer{}, Members: e.members, Owners: e.owners, Trials: e.trials, Places: e.places,
		Clock: clock.NewFixed(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)),
	})
	return e
}

func validBusiness() application.BusinessInput {
	lat, lng := 3.4516, -76.532
	return application.BusinessInput{
		Name: "Barbería El Paisa", ContactPhone: "300 123 4567", Description: "Cortes clásicos",
		CityID: cali, AddressLine: "Calle 5 # 38-25", Latitude: &lat, Longitude: &lng,
	}
}

func validInput() application.CreateInput {
	return application.CreateInput{BusinessInput: validBusiness(), OwnerEmail: "dueno@example.com"}
}

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

func TestCreateBusiness(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	b, err := e.svc.CreateBusiness(ctx, admin, validInput())
	if err != nil {
		t.Fatal(err)
	}
	if b.Status() != domain.StatusActive || !b.IsBookable() {
		t.Errorf("a business created by the administrator must start ACTIVE (no activation step), got %s", b.Status())
	}
	if b.Slug() != "barberia-el-paisa" || b.ContactPhone() != "+573001234567" || b.TimeZone() != "America/Bogota" ||
		b.Currency() != "COP" || b.Location().DivisionID != valle || b.Location().Coordinates == nil || b.ID().Version() != 7 {
		t.Errorf("business = %+v", b)
	}
	if b.Policy() != domain.DefaultBookingPolicy() || b.Reminders() != domain.DefaultReminders() {
		t.Errorf("defaults not applied: %+v %+v", b.Policy(), b.Reminders())
	}
	if len(e.members.Added) != 1 || e.members.Added[0].BusinessID != b.ID() || e.members.Added[0].Role != identity.RoleOwner {
		t.Errorf("owner membership = %+v", e.members.Added)
	}
	if len(e.trials.Started) != 1 || e.trials.Started[0] != b.ID() {
		t.Errorf("demo not started for the admin-created business: %v", e.trials.Started)
	}
	if got := e.store.AuditActions(); !reflect.DeepEqual(got, []string{application.AuditBusinessCreated}) {
		t.Errorf("audit = %v", got)
	}
	if a := e.store.Audit[0]; *a.ActorUserID != admin.UserID || *a.BusinessID != b.ID() || a.Metadata["city"] != cali.String() {
		t.Errorf("audit entry = %+v", a)
	}

	// Same name again: next free slug.
	b2, err := e.svc.CreateBusiness(ctx, admin, validInput())
	if err != nil || b2.Slug() != "barberia-el-paisa-2" {
		t.Fatalf("second business slug = %v, %v", b2, err)
	}
}

func TestCreateBusinessIsAtomicWithTheOwner(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	e.members.Err = identity.ErrAccountNotEligible
	_, err := e.svc.CreateBusiness(ctx, admin, validInput())
	if got := fieldCodes(t, err); got["ownerEmail"] != application.CodeAccountNotEligible {
		t.Fatalf("fields = %v", got)
	}
	boom := errors.New("membership store down")
	e.members.Err = boom
	if _, err := e.svc.CreateBusiness(ctx, admin, validInput()); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if e.store.Count() != 0 || len(e.store.Audit) != 0 {
		t.Errorf("business or audit persisted without its owner: %d businesses, audit %v", e.store.Count(), e.store.AuditActions())
	}
}

func TestCreateBusinessValidation(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	lat := 3.4
	_, err := e.svc.CreateBusiness(ctx, admin, application.CreateInput{BusinessInput: application.BusinessInput{
		Name: "x", ContactPhone: "123", AddressLine: "", Latitude: &lat, Description: "a\x07b",
	}})
	want := map[string]string{
		"name": application.CodeInvalidName, "addressLine": application.CodeRequired, "coordinates": application.CodeInvalidCoordinates,
		"ownerEmail": application.CodeRequired, "cityId": application.CodeRequired, "description": application.CodeInvalidDescription,
	}
	if got := fieldCodes(t, err); !reflect.DeepEqual(got, want) {
		t.Errorf("fields = %v, want %v", got, want)
	}

	in := validInput()
	in.CityID = uuid.Must(uuid.NewV7())
	if got := fieldCodes(t, mustFail(e.svc.CreateBusiness(ctx, admin, in))); got["cityId"] != application.CodeUnknownCity {
		t.Errorf("unknown city: %v", got)
	}
	in.CityID = closed
	if got := fieldCodes(t, mustFail(e.svc.CreateBusiness(ctx, admin, in))); got["cityId"] != application.CodeCityInactive {
		t.Errorf("inactive city: %v", got)
	}
	in = validInput()
	in.ContactPhone = "12"
	if got := fieldCodes(t, mustFail(e.svc.CreateBusiness(ctx, admin, in))); got["contactPhone"] != application.CodeInvalidPhone {
		t.Errorf("phone: %v", got)
	}
	bad := 95.0
	in = validInput()
	in.Latitude = &bad
	if got := fieldCodes(t, mustFail(e.svc.CreateBusiness(ctx, admin, in))); got["coordinates"] != application.CodeInvalidCoordinates {
		t.Errorf("coordinates: %v", got)
	}
	if e.store.Count() != 0 || len(e.members.Added) != 0 {
		t.Error("invalid input persisted something")
	}
}

func mustFail(_ *domain.Business, err error) error { return err }

func TestAdminOnly(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	b, err := e.svc.CreateBusiness(ctx, admin, validInput())
	if err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func(identity.Principal) error{
		"create":   func(p identity.Principal) error { return mustFail(e.svc.CreateBusiness(ctx, p, validInput())) },
		"get":      func(p identity.Principal) error { return mustFail(e.svc.GetBusiness(ctx, p, b.ID())) },
		"activate": func(p identity.Principal) error { return mustFail(e.svc.ActivateBusiness(ctx, p, b.ID())) },
		"suspend":  func(p identity.Principal) error { return mustFail(e.svc.SuspendBusiness(ctx, p, b.ID())) },
		"list": func(p identity.Principal) error {
			_, _, err := e.svc.ListBusinesses(ctx, p, "", uuid.Nil, uuid.Nil, 0)
			return err
		},
	} {
		if err := call(someone); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("%s as non-admin: %v", name, err)
		}
	}
	if e.store.Count() != 1 || len(e.store.Audit) != 1 {
		t.Errorf("denied calls changed state: %d %v", e.store.Count(), e.store.AuditActions())
	}
}

func TestActivateSuspendAndList(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	b, _ := e.svc.CreateBusiness(ctx, admin, validInput())

	got, err := e.svc.SuspendBusiness(ctx, admin, b.ID())
	if err != nil || got.Status() != domain.StatusSuspended {
		t.Fatalf("suspend: %v %v", got, err)
	}
	if _, err := e.svc.SuspendBusiness(ctx, admin, b.ID()); err != nil { // idempotent
		t.Fatal(err)
	}
	if got, err = e.svc.ActivateBusiness(ctx, admin, b.ID()); err != nil || got.Status() != domain.StatusActive {
		t.Fatalf("reactivate: %v %v", got, err)
	}
	if _, err := e.svc.ActivateBusiness(ctx, admin, b.ID()); err != nil { // idempotent
		t.Fatal(err)
	}
	want := []string{application.AuditBusinessCreated, application.AuditBusinessSuspended, application.AuditBusinessActivated}
	if got := e.store.AuditActions(); !reflect.DeepEqual(got, want) {
		t.Errorf("audit = %v, want %v (idempotent calls not audited)", got, want)
	}
	if _, err := e.svc.ActivateBusiness(ctx, admin, uuid.Must(uuid.NewV7())); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown business: %v", err)
	}

	// Suspended while its location became inactive: reactivation refused.
	if _, err := e.svc.SuspendBusiness(ctx, admin, b.ID()); err != nil {
		t.Fatal(err)
	}
	e.places[cali] = geography.Place{CityID: cali, DivisionID: valle, CountryCode: "CO", TimeZone: "America/Bogota", Currency: "COP", Active: false}
	if _, err := e.svc.ActivateBusiness(ctx, admin, b.ID()); !errors.Is(err, application.ErrLocationInactive) {
		t.Errorf("activation with inactive location: %v", err)
	}
	e.places[cali] = caliCity

	// Paging and filters.
	for i := 0; i < 4; i++ {
		if _, err := e.svc.CreateBusiness(ctx, admin, validInput()); err != nil {
			t.Fatal(err)
		}
	}
	page, next, err := e.svc.ListBusinesses(ctx, admin, "", uuid.Nil, uuid.Nil, 2)
	if err != nil || len(page) != 2 || next == uuid.Nil {
		t.Fatalf("page 1: %d %v %v", len(page), next, err)
	}
	seen := len(page)
	for next != uuid.Nil {
		page, next, err = e.svc.ListBusinesses(ctx, admin, "", uuid.Nil, next, 2)
		if err != nil {
			t.Fatal(err)
		}
		seen += len(page)
	}
	if seen != 5 {
		t.Errorf("paged through %d businesses, want 5", seen)
	}
	active, _, _ := e.svc.ListBusinesses(ctx, admin, "ACTIVE", uuid.Nil, uuid.Nil, 0)
	suspended, _, _ := e.svc.ListBusinesses(ctx, admin, "SUSPENDED", cali, uuid.Nil, 0)
	if len(active) != 4 || len(suspended) != 1 {
		t.Errorf("filters: active %d (want 4), suspended %d (want 1)", len(active), len(suspended))
	}
	if _, _, err := e.svc.ListBusinesses(ctx, admin, "DELETED", uuid.Nil, uuid.Nil, 0); fieldCodes(t, err)["status"] != application.CodeInvalidStatus {
		t.Errorf("invalid status filter: %v", err)
	}
}

func TestSelfServiceOnboarding(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	owner := identity.Principal{UserID: uuid.Must(uuid.NewV7())}

	b, err := e.svc.CreateOwnBusiness(ctx, owner, validBusiness())
	if err != nil {
		t.Fatal(err)
	}
	// ACTIVE at once (real bookings during the demo), demo started, OWNER = caller.
	if b.Status() != domain.StatusActive || !b.IsBookable() {
		t.Errorf("self-service business must be ACTIVE, got %s", b.Status())
	}
	if len(e.trials.Started) != 1 || e.trials.Started[0] != b.ID() {
		t.Errorf("demo: %v", e.trials.Started)
	}
	if len(e.members.Added) != 0 {
		t.Error("self-service must not use the admin AddMember path")
	}
	if a := e.store.Audit[len(e.store.Audit)-1]; a.Metadata["via"] != "self_service" || *a.ActorUserID != owner.UserID {
		t.Errorf("audit = %+v", a)
	}

	// P-11: a second business, with its own demo; the third goes beyond the limit (2) and
	// nothing is persisted.
	second, err := e.svc.CreateOwnBusiness(ctx, owner, validBusiness())
	if err != nil || second.ID() == b.ID() || second.Slug() == b.Slug() || len(e.trials.Started) != 2 || e.trials.Started[1] != second.ID() {
		t.Fatalf("second business: %v, demos %v", err, e.trials.Started)
	}
	before := e.store.Count()
	if _, err := e.svc.CreateOwnBusiness(ctx, owner, validBusiness()); !errors.Is(err, identity.ErrOwnedBusinessLimit) {
		t.Errorf("third self-service business: %v, want ErrOwnedBusinessLimit", err)
	}
	if e.store.Count() != before || len(e.trials.Started) != 2 {
		t.Error("business persisted beyond the limit")
	}
	// Validation and authentication.
	in := validBusiness()
	in.CityID = closed
	if got := fieldCodes(t, mustFail(e.svc.CreateOwnBusiness(ctx, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, in))); got["cityId"] != application.CodeCityInactive {
		t.Errorf("inactive city: %v", got)
	}
	if _, err := e.svc.CreateOwnBusiness(ctx, identity.Principal{}, validBusiness()); !errors.Is(err, application.ErrUnauthenticated) {
		t.Errorf("anonymous: %v", err)
	}
}

// TestSelfServiceIdempotency (P-11): the optional Idempotency-Key returns the same business for
// a repeated request and refuses it with other data.
func TestSelfServiceIdempotency(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	owner := identity.Principal{UserID: uuid.Must(uuid.NewV7())}
	in := validBusiness()
	in.IdempotencyKey = "portal-create-0001"
	b, err := e.svc.CreateOwnBusiness(ctx, owner, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.svc.CreateOwnBusiness(ctx, owner, in)
	if err != nil || again.ID() != b.ID() || e.store.Count() != 1 || len(e.trials.Started) != 1 {
		t.Errorf("replay: %v %v, %d businesses, demos %v", again, err, e.store.Count(), e.trials.Started)
	}
	other := in
	other.Name = "Otra Barbería"
	if _, err := e.svc.CreateOwnBusiness(ctx, owner, other); !errors.Is(err, application.ErrIdempotencyKeyReused) {
		t.Errorf("same key, other data: %v", err)
	}
	// The key belongs to the account: another one creates its own business with it.
	if c, err := e.svc.CreateOwnBusiness(ctx, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, in); err != nil || c.ID() == b.ID() {
		t.Errorf("another account, same key: %v %v", c, err)
	}
	// A request that fails does not keep the key: after fixing it, the same key creates.
	bad := validBusiness()
	bad.CityID, bad.IdempotencyKey = closed, "portal-create-0002"
	if _, err := e.svc.CreateOwnBusiness(ctx, owner, bad); err == nil {
		t.Fatal("inactive city accepted")
	}
	fixed := validBusiness()
	fixed.IdempotencyKey = "portal-create-0002"
	if _, err := e.svc.CreateOwnBusiness(ctx, owner, fixed); err != nil {
		t.Errorf("same key after a failed request: %v", err)
	}
	for _, k := range []string{"short", "con espacios 123", strings.Repeat("a", 101)} {
		in := validBusiness()
		in.IdempotencyKey = k
		if _, err := e.svc.CreateOwnBusiness(ctx, owner, in); !errors.Is(err, application.ErrInvalidIdempotencyKey) {
			t.Errorf("key %q: %v", k, err)
		}
	}
}

func TestCreationIsAtomicWithTheDemo(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	boom := errors.New("subscriptions down")
	e.trials.Err = boom
	if _, err := e.svc.CreateOwnBusiness(ctx, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, validBusiness()); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, err := e.svc.CreateBusiness(ctx, admin, validInput()); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if e.store.Count() != 0 || len(e.store.Audit) != 0 {
		t.Errorf("business persisted without its demo: %d %v", e.store.Count(), e.store.AuditActions())
	}
}

func TestCurrencyOf(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	b, err := e.svc.CreateOwnBusiness(ctx, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, validBusiness())
	if err != nil {
		t.Fatal(err)
	}
	if c, err := e.svc.CurrencyOf(ctx, b.ID()); err != nil || c != "COP" {
		t.Errorf("currency = %q %v", c, err)
	}
	if _, err := e.svc.CurrencyOf(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, bizcontract.ErrBusinessNotFound) {
		t.Errorf("unknown business: %v", err)
	}
}
