package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// TestAgenda: who sees what, the views and the pagination.
func TestAgenda(t *testing.T) {
	ctx := context.Background()
	e := newBookingEnv(t)
	newP := func() identity.Principal { return identity.Principal{UserID: uuid.Must(uuid.NewV7())} }
	owner, juanUser, anaUser, nobody := newP(), newP(), newP(), newP()
	e.roles[[2]uuid.UUID{e.biz, owner.UserID}] = identity.RoleOwner
	e.roles[[2]uuid.UUID{e.biz, juanUser.UserID}] = identity.RoleProfessional
	e.roles[[2]uuid.UUID{e.biz, anaUser.UserID}] = identity.RoleProfessional
	e.roles[[2]uuid.UUID{e.biz, nobody.UserID}] = identity.RoleProfessional // member without a profile
	anaPro := uuid.Must(uuid.NewV7())
	e.ag.owners = map[uuid.UUID]uuid.UUID{e.pro: juanUser.UserID, anaPro: anaUser.UserID}
	e.ag.names = map[uuid.UUID]string{e.pro: "Juan", anaPro: "Ana"}
	// Juan: Tuesday 09:00 and 10:00 (customers from the app); Ana: Tuesday 09:00 (seeded).
	for i, k := range []string{"agenda-1", "agenda-2"} {
		if _, err := e.svc.Book(ctx, e.ana, e.in(e.day(9+i, 0), k)); err != nil {
			t.Fatal(err)
		}
	}
	seed := domain.Appointment{
		ID: uuid.Must(uuid.NewV7()), BusinessID: e.biz, ProfessionalID: anaPro, Start: e.day(9, 0).UTC(), End: e.day(9, 30).UTC(),
		Status: domain.StatusConfirmed, Customer: domain.Customer{Name: "Walk-in", Phone: "+573001234567"}, Service: domain.ServiceSnapshot{Name: "Corte"},
	}
	e.store.Appointments[seed.ID] = seed

	tuesday, _ := domain.ParseLocalDate("2030-03-05")
	q := func(view domain.View, pro uuid.UUID) application.AgendaQuery {
		return application.AgendaQuery{View: view, Date: tuesday, ProfessionalID: pro}
	}
	for name, c := range map[string]struct {
		p    identity.Principal
		in   application.AgendaQuery
		want int
		err  error
	}{
		"owner sees everyone":      {owner, q(domain.ViewDay, uuid.Nil), 3, nil},
		"owner filters one":        {owner, q(domain.ViewDay, anaPro), 1, nil},
		"barber sees his own":      {juanUser, q(domain.ViewDay, uuid.Nil), 2, nil},
		"barber asks for his own":  {juanUser, q(domain.ViewDay, e.pro), 2, nil},
		"barber asks for another":  {juanUser, q(domain.ViewDay, anaPro), 0, authz.ErrForbidden},
		"member without a profile": {nobody, q(domain.ViewDay, uuid.Nil), 0, nil},
		"outsider":                 {newP(), q(domain.ViewDay, uuid.Nil), 0, authz.ErrNotFound},
		"week":                     {owner, q(domain.ViewWeek, uuid.Nil), 3, nil},
		"month":                    {owner, q(domain.ViewMonth, uuid.Nil), 3, nil},
		"year":                     {owner, q(domain.ViewYear, uuid.Nil), 3, nil},
		"another day":              {owner, application.AgendaQuery{View: domain.ViewDay, Date: tuesday.AddDays(1)}, 0, nil},
		"the next week":            {owner, application.AgendaQuery{View: domain.ViewWeek, Date: tuesday.AddDays(7)}, 0, nil},
	} {
		page, err := e.svc.Agenda(ctx, c.p, e.biz, c.in)
		if !errors.Is(err, c.err) || (err == nil && len(page.Items) != c.want) {
			t.Errorf("%s: %d %v, want %d %v", name, len(page.Items), err, c.want, c.err)
		}
	}
	var verr *application.ValidationError
	if _, err := e.svc.Agenda(ctx, owner, e.biz, q(domain.ViewDay, uuid.Must(uuid.NewV7()))); !errors.As(err, &verr) {
		t.Errorf("unknown professional: %v", err)
	}
	if _, err := e.svc.Agenda(ctx, owner, e.biz, application.AgendaQuery{Date: tuesday, Cursor: "basura"}); !errors.As(err, &verr) {
		t.Errorf("bad cursor: %v", err)
	}
	if _, err := e.svc.Agenda(ctx, owner, e.biz, application.AgendaQuery{Date: tuesday, Limit: 501}); !errors.As(err, &verr) {
		t.Errorf("limit too big: %v", err)
	}

	// Pagination: one per page, in order, names attached, the whole day once.
	page, err := e.svc.Agenda(ctx, owner, e.biz, application.AgendaQuery{View: domain.ViewDay, Date: tuesday, Limit: 1})
	var seen []string
	for err == nil {
		for _, it := range page.Items {
			seen = append(seen, it.Start.In(page.Location).Format("15:04")+" "+it.ProfessionalName)
		}
		if page.NextCursor == "" {
			break
		}
		page, err = e.svc.Agenda(ctx, owner, e.biz, application.AgendaQuery{View: domain.ViewDay, Date: tuesday, Limit: 1, Cursor: page.NextCursor})
	}
	if err != nil || len(seen) != 3 || seen[2] != "10:00 Juan" || seen[0][:5] != "09:00" || seen[1][:5] != "09:00" {
		t.Errorf("pages = %v %v", seen, err)
	}
	// No date: today in the business's calendar (Friday 1 March).
	if page, err := e.svc.Agenda(ctx, owner, e.biz, application.AgendaQuery{}); err != nil || page.From.String() != "2030-03-01" || len(page.Items) != 0 {
		t.Errorf("default day: %s %d %v", page.From, len(page.Items), err)
	}
}

// TestMyAppointments: a customer sees its own appointments of every business, upcoming and past;
// anyone else's is not found.
func TestMyAppointments(t *testing.T) {
	ctx := context.Background()
	e := newBookingEnv(t)
	e.ag.owners = map[uuid.UUID]uuid.UUID{e.pro: uuid.Must(uuid.NewV7())}
	e.ag.names = map[uuid.UUID]string{e.pro: "Juan"}
	a1, err := e.svc.Book(ctx, e.ana, e.in(e.day(10, 0), "mis-citas-1"))
	if err != nil {
		t.Fatal(err)
	}
	a2, err := e.svc.Book(ctx, e.ana, e.in(e.day(9, 0), "mis-citas-2"))
	if err != nil {
		t.Fatal(err)
	}
	b1, err := e.svc.Book(ctx, e.bob, e.in(e.day(11, 0), "sus-citas-1"))
	if err != nil {
		t.Fatal(err)
	}
	past := domain.Appointment{
		ID: uuid.Must(uuid.NewV7()), BusinessID: e.biz, ProfessionalID: e.pro, Start: time.Date(2030, 2, 1, 14, 0, 0, 0, time.UTC),
		Status: domain.StatusCompleted, Customer: domain.Customer{UserID: e.ana.UserID, Name: "Ana"}, Service: domain.ServiceSnapshot{Name: "Corte"},
	}
	e.store.Appointments[past.ID] = past

	up, next, err := e.svc.MyAppointments(ctx, e.ana, application.MyAppointmentsQuery{Upcoming: true})
	if err != nil || len(up) != 2 || up[0].ID != a2.ID || up[1].ID != a1.ID || next != "" {
		t.Fatalf("upcoming: %+v %q %v", up, next, err)
	}
	if up[0].Business.Name != "Barbería Centro" || up[0].ProfessionalName != "Juan" {
		t.Errorf("names: %+v", up[0])
	}
	if p, _, err := e.svc.MyAppointments(ctx, e.ana, application.MyAppointmentsQuery{}); err != nil || len(p) != 1 || p[0].ID != past.ID {
		t.Errorf("past: %+v %v", p, err)
	}
	first, next, _ := e.svc.MyAppointments(ctx, e.ana, application.MyAppointmentsQuery{Upcoming: true, Limit: 1})
	rest, _, _ := e.svc.MyAppointments(ctx, e.ana, application.MyAppointmentsQuery{Upcoming: true, Limit: 1, Cursor: next})
	if len(first) != 1 || len(rest) != 1 || first[0].ID != a2.ID || rest[0].ID != a1.ID {
		t.Errorf("pages: %v %v", first, rest)
	}
	if d, err := e.svc.MyAppointmentDetail(ctx, e.ana, a1.ID); err != nil || d.ID != a1.ID {
		t.Errorf("own detail: %v", err)
	}
	for name, id := range map[string]uuid.UUID{"another customer": b1.ID, "missing": uuid.Must(uuid.NewV7())} {
		if _, err := e.svc.MyAppointmentDetail(ctx, e.ana, id); !errors.Is(err, application.ErrAppointmentNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
