package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// unpublished is a business customers cannot see: only the staff's policy is available.
type unpublished struct{ fakeBiz }

func (unpublished) PublicBookingPolicy(context.Context, string) (businesses.BookingPolicy, error) {
	return businesses.BookingPolicy{}, businesses.ErrBusinessNotFound
}

func (unpublished) PublicBookingPolicyOf(context.Context, uuid.UUID) (businesses.BookingPolicy, error) {
	return businesses.BookingPolicy{}, businesses.ErrBusinessNotFound
}

func starts(slots []domain.Slot) []time.Time {
	out := make([]time.Time, 0, len(slots))
	for _, s := range slots {
		out = append(out, s.Start)
	}
	return out
}

// TestStaffAvailability (P-9): the staff sees the times it can book — today from now on without
// the minimum notice, unpublished or not, within the horizon — and each of them is accepted by a
// manual booking; a professional only asks for its own agenda.
func TestStaffAvailability(t *testing.T) {
	ctx := context.Background()
	e := newBookingEnv(t)
	// Working 09:00–14:00 every day; "now" is Friday 2030-03-01 12:00 in Bogotá, 60 min notice.
	e.ag.agenda = professionals.DayAgenda{WorkingHours: []professionals.MinuteRange{{Start: 540, End: 840}}}
	newP := func() identity.Principal { return identity.Principal{UserID: uuid.Must(uuid.NewV7())} }
	owner, juanUser, anaUser, outsider := newP(), newP(), newP(), newP()
	e.roles[[2]uuid.UUID{e.biz, owner.UserID}] = identity.RoleOwner
	e.roles[[2]uuid.UUID{e.biz, juanUser.UserID}] = identity.RoleProfessional
	e.roles[[2]uuid.UUID{e.biz, anaUser.UserID}] = identity.RoleProfessional
	anaPro := uuid.Must(uuid.NewV7())
	e.ag.owners = map[uuid.UUID]uuid.UUID{e.pro: juanUser.UserID, anaPro: anaUser.UserID}
	bog := e.day(0, 0).Location()
	today := time.Date(2030, 3, 1, 0, 0, 0, 0, bog)
	at := func(h, m int) time.Time { return time.Date(2030, 3, 1, h, m, 0, 0, bog) }
	q := func(pro uuid.UUID, date string) application.StaffAvailabilityQuery {
		return application.StaffAvailabilityQuery{ServiceID: e.svcID.String(), ProfessionalID: pro.String(), Date: date}
	}

	public, err := e.svc.PublicAvailability(ctx, "barberia", e.svcID, e.pro, domain.LocalDateOf(today, bog))
	if err != nil || len(public.Slots) == 0 || !public.Slots[0].Start.Equal(at(13, 0)) {
		t.Fatalf("public today: %v %v", starts(public.Slots), err)
	}
	staff, err := e.svc.StaffAvailability(ctx, owner, e.biz, q(e.pro, "2030-03-01"))
	if err != nil || len(staff.Slots) == 0 || !staff.Slots[0].Start.Equal(at(12, 0)) || staff.DurationMinutes != 30 {
		t.Fatalf("staff today: %v %v", starts(staff.Slots), err)
	}
	if last := staff.Slots[len(staff.Slots)-1]; !last.Start.Equal(at(13, 30)) || len(staff.Slots) != len(public.Slots)+6 {
		t.Errorf("staff today: %v (public %v)", starts(staff.Slots), starts(public.Slots))
	}
	// "Now" is bookable by the staff, not by a customer.
	if _, err := e.svc.Book(ctx, e.ana, e.in(at(12, 0), "ahora-cliente-1")); !errors.Is(err, application.ErrSlotUnavailable) {
		t.Errorf("customer books now: %v", err)
	}
	walkIn, err := e.svc.BookManual(ctx, owner, e.biz, application.ManualInput{
		ProfessionalID: e.pro, ServiceID: e.svcID, Customer: &application.CustomerInput{FullName: "Sin cita", Phone: "3007778899"},
		Start: staff.Slots[0].Start, Confirmed: true, IdempotencyKey: "ahora-mismo-0001",
	})
	if err != nil {
		t.Fatalf("manual booking of the first staff time: %v", err)
	}

	// The booked appointment blocks its times, unless it is the one being moved.
	after, _ := e.svc.StaffAvailability(ctx, owner, e.biz, q(e.pro, "2030-03-01"))
	if len(after.Slots) == 0 || after.Slots[0].Start.Equal(at(12, 0)) {
		t.Errorf("booked time still offered: %v", starts(after.Slots))
	}
	moving := q(e.pro, "2030-03-01")
	moving.ExcludeAppointmentID = walkIn.ID.String()
	if got, err := e.svc.StaffAvailability(ctx, owner, e.biz, moving); err != nil || len(got.Slots) != len(staff.Slots) {
		t.Errorf("excluding the appointment being moved: %v %v", starts(got.Slots), err)
	}

	for name, c := range map[string]struct {
		p    identity.Principal
		in   application.StaffAvailabilityQuery
		want error
	}{
		"barber, his own":          {juanUser, q(e.pro, "2030-03-01"), nil},
		"barber, another":          {juanUser, q(anaPro, "2030-03-01"), authz.ErrForbidden},
		"owner, any professional":  {owner, q(anaPro, "2030-03-01"), nil},
		"outsider":                 {outsider, q(e.pro, "2030-03-01"), authz.ErrNotFound},
		"outsider, bad parameters": {outsider, application.StaffAvailabilityQuery{Date: "x"}, authz.ErrNotFound},
		"yesterday":                {owner, q(e.pro, "2030-02-28"), domain.ErrOutsideBookingWindow},
		"beyond the horizon":       {owner, q(e.pro, "2030-05-01"), domain.ErrOutsideBookingWindow},
		"last day of the horizon":  {owner, q(e.pro, "2030-04-30"), nil},
	} {
		if _, err := e.svc.StaffAvailability(ctx, c.p, e.biz, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	var verr *application.ValidationError
	for name, in := range map[string]application.StaffAvailabilityQuery{
		"bad service":      {ServiceID: "x", ProfessionalID: e.pro.String(), Date: "2030-03-01"},
		"bad date":         {ServiceID: e.svcID.String(), ProfessionalID: e.pro.String(), Date: "01/03/2030"},
		"bad exclusion":    {ServiceID: e.svcID.String(), ProfessionalID: e.pro.String(), Date: "2030-03-01", ExcludeAppointmentID: "x"},
		"no professional":  {ServiceID: e.svcID.String(), Date: "2030-03-01"},
		"nothing at all":   {},
		"bad professional": {ServiceID: e.svcID.String(), ProfessionalID: "juan", Date: "2030-03-01"},
	} {
		if _, err := e.svc.StaffAvailability(ctx, owner, e.biz, in); !errors.As(err, &verr) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// An unpublished business: customers see nothing, the staff the same times.
	pol := businesses.BookingPolicy{BusinessID: e.biz, TimeZone: "America/Bogota", SlotIntervalMinutes: 10, BufferMinutes: 10, MinLeadMinutes: 60, MaxAdvanceDays: 60}
	hidden := application.NewService(application.Deps{Store: e.store, Authz: e.roles, Businesses: unpublished{fakeBiz{pol: pol}}, Catalog: e.cat, Professionals: e.ag, Bookable: e.ag, Clock: e.clk})
	if _, err := hidden.PublicAvailability(ctx, "barberia", e.svcID, e.pro, domain.LocalDateOf(today, bog)); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("public of an unpublished business: %v", err)
	}
	if got, err := hidden.StaffAvailability(ctx, owner, e.biz, q(e.pro, "2030-03-01")); err != nil || len(got.Slots) != len(after.Slots) {
		t.Errorf("staff of an unpublished business: %v %v", starts(got.Slots), err)
	}
}

// TestCustomerAppointments (P-10): upcoming ascending and past newest first, the OWNER sees all,
// a PROFESSIONAL only its agenda's; another business's customer is not found.
func TestCustomerAppointments(t *testing.T) {
	ctx := context.Background()
	e := newBookingEnv(t)
	newP := func() identity.Principal { return identity.Principal{UserID: uuid.Must(uuid.NewV7())} }
	owner, juanUser, anaUser, nobody, outsider := newP(), newP(), newP(), newP(), newP()
	e.roles[[2]uuid.UUID{e.biz, owner.UserID}] = identity.RoleOwner
	e.roles[[2]uuid.UUID{e.biz, juanUser.UserID}] = identity.RoleProfessional
	e.roles[[2]uuid.UUID{e.biz, anaUser.UserID}] = identity.RoleProfessional
	e.roles[[2]uuid.UUID{e.biz, nobody.UserID}] = identity.RoleProfessional // member without a profile
	anaPro := uuid.Must(uuid.NewV7())
	e.ag.owners = map[uuid.UUID]uuid.UUID{e.pro: juanUser.UserID, anaPro: anaUser.UserID}
	e.ag.names = map[uuid.UUID]string{e.pro: "Juan", anaPro: "Ana"}

	carlos, err := e.svc.CreateCustomer(ctx, owner, e.biz, application.CustomerInput{FullName: "Carlos", Phone: "3005550101"})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := e.svc.CreateCustomer(ctx, owner, e.biz, application.CustomerInput{FullName: "Diana", Phone: "3005550202"})
	book := func(pro, customer uuid.UUID, start time.Time, key string) {
		t.Helper()
		if _, err := e.svc.BookManual(ctx, owner, e.biz, application.ManualInput{ProfessionalID: pro, ServiceID: e.svcID, CustomerID: customer, Start: start, IdempotencyKey: key}); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
	}
	book(e.pro, carlos.ID, e.day(9, 0), "carlos-juan-0001")
	book(anaPro, carlos.ID, e.day(10, 0), "carlos-ana-00001")
	book(e.pro, other.ID, e.day(11, 0), "diana-juan-00001")
	// Two past appointments of Carlos with Juan, and one "of Carlos" in another business.
	for _, days := range []int{3, 10} {
		start := e.clk.Now().AddDate(0, 0, -days)
		a := domain.Appointment{ID: uuid.Must(uuid.NewV7()), BusinessID: e.biz, ProfessionalID: e.pro, Start: start, End: start.Add(30 * time.Minute),
			Status: domain.StatusCompleted, Customer: domain.Customer{CustomerID: carlos.ID, Name: "Carlos"}, Service: domain.ServiceSnapshot{Name: "Corte"}}
		e.store.Appointments[a.ID] = a
	}
	foreign := domain.Appointment{ID: uuid.Must(uuid.NewV7()), BusinessID: uuid.Must(uuid.NewV7()), ProfessionalID: e.pro, Start: e.day(12, 0), End: e.day(12, 30),
		Status: domain.StatusConfirmed, Customer: domain.Customer{CustomerID: carlos.ID}}
	e.store.Appointments[foreign.ID] = foreign

	upcoming := application.CustomerAppointmentsQuery{Upcoming: true}
	past := application.CustomerAppointmentsQuery{}
	for name, c := range map[string]struct {
		p    identity.Principal
		q    application.CustomerAppointmentsQuery
		want int
	}{
		"owner, upcoming":          {owner, upcoming, 2},
		"owner, past":              {owner, past, 2},
		"Juan, upcoming":           {juanUser, upcoming, 1},
		"Juan, past":               {juanUser, past, 2},
		"Ana, upcoming":            {anaUser, upcoming, 1},
		"Ana, past":                {anaUser, past, 0},
		"member without a profile": {nobody, upcoming, 0},
	} {
		page, err := e.svc.CustomerAppointments(ctx, c.p, e.biz, carlos.ID, c.q)
		if err != nil || len(page.Items) != c.want {
			t.Errorf("%s: %d %v, want %d", name, len(page.Items), err, c.want)
		}
	}

	// Order, names and pagination.
	page, err := e.svc.CustomerAppointments(ctx, owner, e.biz, carlos.ID, application.CustomerAppointmentsQuery{Upcoming: true, Limit: 1})
	if err != nil || len(page.Items) != 1 || !page.Items[0].Start.Equal(e.day(9, 0)) || page.Items[0].ProfessionalName != "Juan" || page.NextCursor == "" || page.Location.String() != "America/Bogota" {
		t.Fatalf("first page: %+v %v", page, err)
	}
	page, err = e.svc.CustomerAppointments(ctx, owner, e.biz, carlos.ID, application.CustomerAppointmentsQuery{Upcoming: true, Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(page.Items) != 1 || !page.Items[0].Start.Equal(e.day(10, 0)) || page.Items[0].ProfessionalName != "Ana" || page.NextCursor != "" {
		t.Fatalf("second and last page: %+v %v", page, err)
	}
	page, _ = e.svc.CustomerAppointments(ctx, owner, e.biz, carlos.ID, past)
	if len(page.Items) != 2 || !page.Items[0].Start.After(page.Items[1].Start) {
		t.Errorf("past is not newest first: %+v", page.Items)
	}

	if _, err := e.svc.CustomerAppointments(ctx, owner, e.biz, uuid.Must(uuid.NewV7()), upcoming); !errors.Is(err, application.ErrCustomerNotFound) {
		t.Errorf("unknown customer: %v", err)
	}
	if _, err := e.svc.CustomerAppointments(ctx, outsider, e.biz, carlos.ID, upcoming); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("outsider: %v", err)
	}
	var verr *application.ValidationError
	if _, err := e.svc.CustomerAppointments(ctx, owner, e.biz, carlos.ID, application.CustomerAppointmentsQuery{Cursor: "basura"}); !errors.As(err, &verr) {
		t.Errorf("bad cursor: %v", err)
	}
	if _, err := e.svc.CustomerAppointments(ctx, owner, e.biz, carlos.ID, application.CustomerAppointmentsQuery{Limit: 501}); !errors.As(err, &verr) {
		t.Errorf("limit too big: %v", err)
	}
}
