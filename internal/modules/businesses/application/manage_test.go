package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application/biztest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	subscriptions "github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type manageEnv struct {
	svc                             *application.Service
	store                           *biztest.Store
	biz                             *domain.Business
	owner, manager, professional, x identity.Principal
}

var (
	antioquia = uuid.Must(uuid.NewV7())
	medellin  = uuid.Must(uuid.NewV7())
	lima      = uuid.Must(uuid.NewV7())
)

func newManageEnv(t *testing.T) manageEnv {
	t.Helper()
	e := manageEnv{
		store: biztest.NewStore(), owner: identity.Principal{UserID: uuid.Must(uuid.NewV7())},
		manager: identity.Principal{UserID: uuid.Must(uuid.NewV7())}, professional: identity.Principal{UserID: uuid.Must(uuid.NewV7())},
		x: identity.Principal{UserID: uuid.Must(uuid.NewV7())},
	}
	places := biztest.Places{
		cali:     caliCity,
		closed:   {CityID: closed, DivisionID: valle, CountryCode: "CO", TimeZone: "America/Bogota", Currency: "COP", Active: false},
		medellin: {CityID: medellin, DivisionID: antioquia, CountryCode: "CO", TimeZone: "America/Bogota", Currency: "COP", Active: true},
		lima:     {CityID: lima, DivisionID: uuid.Must(uuid.NewV7()), CountryCode: "PE", TimeZone: "America/Lima", Currency: "USD", Active: true},
	}
	authz := biztest.Authorizer{Memberships: map[uuid.UUID][]identity.Membership{}}
	end := time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)
	e.svc = application.NewService(application.Deps{
		Store: e.store, Authz: authz, Members: &biztest.Members{}, Owners: &biztest.Owners{}, Trials: &biztest.Trials{},
		Summaries: biztest.Summaries{Summary: subscriptions.Summary{Status: "TRIALING", TrialEndsAt: &end, Entitled: true}},
		Places:    places, Clock: clock.NewFixed(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)),
	})
	var err error
	if e.biz, err = e.svc.CreateOwnBusiness(context.Background(), e.owner, validBusiness()); err != nil {
		t.Fatal(err)
	}
	id := e.biz.ID()
	authz.Memberships[e.owner.UserID] = []identity.Membership{{BusinessID: id, Role: identity.RoleOwner}}
	authz.Memberships[e.manager.UserID] = []identity.Membership{{BusinessID: id, Role: identity.RoleManager}}
	authz.Memberships[e.professional.UserID] = []identity.Membership{{BusinessID: id, Role: identity.RoleProfessional}}
	authz.Memberships[e.x.UserID] = []identity.Membership{{BusinessID: uuid.Must(uuid.NewV7()), Role: identity.RoleOwner}}
	return e
}

func ptr[T any](v T) *T { return &v }

func TestGetManagedByRole(t *testing.T) {
	ctx := context.Background()
	e := newManageEnv(t)
	for name, p := range map[string]identity.Principal{"owner": e.owner, "manager": e.manager, "professional": e.professional, "admin": admin} {
		m, err := e.svc.GetManaged(ctx, p, e.biz.ID())
		if err != nil || m.Business.ID() != e.biz.ID() || m.Subscription.Status != "TRIALING" || !m.Subscription.Entitled {
			t.Errorf("%s: %+v %v", name, m, err)
		}
	}
	// Owner of another business / nobody: 404 (no hint that the business exists).
	for name, p := range map[string]identity.Principal{"other owner": e.x, "no membership": someone} {
		if _, err := e.svc.GetManaged(ctx, p, e.biz.ID()); !errors.Is(err, authz.ErrNotFound) {
			t.Errorf("%s: %v, want ErrNotFound", name, err)
		}
	}
	if _, err := e.svc.GetManaged(ctx, admin, uuid.Must(uuid.NewV7())); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("admin on a nonexistent business: %v", err)
	}
}

func TestUpdateBusinessRoles(t *testing.T) {
	ctx := context.Background()
	e := newManageEnv(t)
	in := application.UpdateInput{Name: ptr("Barbería Renovada")}
	for name, tc := range map[string]struct {
		p    identity.Principal
		want error
	}{
		"manager":      {e.manager, authz.ErrForbidden},
		"professional": {e.professional, authz.ErrForbidden},
		"other owner":  {e.x, authz.ErrNotFound},
		"nobody":       {someone, authz.ErrNotFound},
	} {
		if _, err := e.svc.UpdateBusiness(ctx, tc.p, e.biz.ID(), in); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
	}
	before := len(e.store.Audit)
	for i, p := range []identity.Principal{e.owner, admin} {
		in.Name = ptr([]string{"Barbería del Dueño", "Barbería del Admin"}[i])
		if b, err := e.svc.UpdateBusiness(ctx, p, e.biz.ID(), in); err != nil || b.Name() != *in.Name {
			t.Fatalf("update as %v: %v", p.UserID, err)
		}
	}
	if len(e.store.Audit)-before != 2 {
		t.Errorf("updates audited = %d, want 2", len(e.store.Audit)-before)
	}
	last := e.store.Audit[len(e.store.Audit)-1]
	if last.Action != application.AuditBusinessUpdated || *last.ActorUserID != admin.UserID || last.Metadata["fields"] != "name" {
		t.Errorf("audit = %+v (admin acts with its own identity)", last)
	}
}

func TestUpdateBusinessFields(t *testing.T) {
	ctx := context.Background()
	e := newManageEnv(t)
	id := e.biz.ID()

	b, err := e.svc.UpdateBusiness(ctx, e.owner, id, application.UpdateInput{
		ContactPhone: ptr("(602) 486 1234"), Description: ptr("Nueva descripción"), CityID: &medellin, AddressLine: ptr("Cra 70 # 1-1"),
		Coordinates: &application.CoordinatesPatch{Remove: true},
		Policy:      &application.PolicyPatch{SlotIntervalMinutes: ptr(15), RequireCustomerConfirmation: ptr(false)},
		Reminders:   &application.RemindersPatch{HoursBefore: ptr(2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.ContactPhone() != "+576024861234" || b.Location().CityID != medellin || b.Location().DivisionID != antioquia ||
		b.Location().Coordinates != nil || b.Policy().SlotIntervalMinutes != 15 || b.Policy().RequireCustomerConfirmation ||
		b.Policy().MinLeadMinutes != 60 || b.Reminders().HoursBefore != 2 || !b.Reminders().Enabled {
		t.Errorf("after update: %+v", b)
	}
	if got := e.store.Audit[len(e.store.Audit)-1].Metadata["fields"]; got != "contactPhone,description,location,bookingPolicy,reminders" {
		t.Errorf("audited fields = %q", got)
	}

	// Nothing changes: no audit entry.
	before := len(e.store.Audit)
	if _, err := e.svc.UpdateBusiness(ctx, e.owner, id, application.UpdateInput{CityID: &medellin, Name: ptr(b.Name())}); err != nil {
		t.Fatal(err)
	}
	if len(e.store.Audit) != before {
		t.Error("a no-op update was audited")
	}

	// Validation: every invalid field reported; nothing changes.
	_, err = e.svc.UpdateBusiness(ctx, e.owner, id, application.UpdateInput{
		Name: ptr("x"), ContactPhone: ptr("12"), AddressLine: ptr(""), Description: ptr("a\x07"),
		Coordinates: &application.CoordinatesPatch{Lat: 100},
		Policy:      &application.PolicyPatch{BufferMinutes: ptr(61)},
		Reminders:   &application.RemindersPatch{HoursBefore: ptr(0)},
	})
	want := map[string]string{
		"name": application.CodeInvalidName, "contactPhone": application.CodeInvalidPhone, "addressLine": application.CodeRequired,
		"description": application.CodeInvalidDescription, "coordinates": application.CodeInvalidCoordinates,
		"bookingPolicy.bufferMinutes": application.CodeInvalidPolicy, "reminders.hoursBefore": application.CodeInvalidReminder,
	}
	if got := fieldCodes(t, err); !reflect.DeepEqual(got, want) {
		t.Errorf("fields = %v, want %v", got, want)
	}
	for city, code := range map[uuid.UUID]string{
		lima: application.CodeCityOtherCountry, closed: application.CodeCityInactive, uuid.Must(uuid.NewV7()): application.CodeUnknownCity,
	} {
		c := city
		if got := fieldCodes(t, mustFail(e.svc.UpdateBusiness(ctx, e.owner, id, application.UpdateInput{CityID: &c}))); got["cityId"] != code {
			t.Errorf("city %s: %v, want %s", city, got, code)
		}
	}
	if len(e.store.Audit) != before {
		t.Error("rejected updates were audited")
	}
}
