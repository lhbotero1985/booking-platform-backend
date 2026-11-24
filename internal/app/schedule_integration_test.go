//go:build integration

package app

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	idapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	idpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/postgres"
	profapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestProfessionalScheduleOnPostgres (T5.3): round trip of a weekly schedule, the barber edits
// only his own, the schema keeps rows inside the professional's business, and concurrent
// replacements never mix two weeks.
func TestProfessionalScheduleOnPostgres(t *testing.T) {
	ctx := t.Context()
	pool := migratedPool(t)
	identity := idapp.NewService(idapp.Deps{
		Store: idpg.NewStore(pool, nil), Hasher: &apptest.Hasher{}, Mailer: &apptest.Mailer{}, Clock: clock.System{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	biz := NewBusinessesService(pool, identity, NewGeographyService(pool, identity), NewSubscriptionsService(pool, identity))
	profs := NewProfessionalsService(pool, identity)

	ownerID, ownerEmail := activeUser(t, pool, "ACTIVE")
	barberID, barberEmail := activeUser(t, pool, "ACTIVE")
	owner, barber := identitycontract.Principal{UserID: ownerID}, identitycontract.Principal{UserID: barberID}
	b, err := biz.CreateOwnBusiness(ctx, owner, bizapp.BusinessInput{Name: "Barbería Horarios", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 1"})
	if err != nil {
		t.Fatal(err)
	}
	juan, err := profs.Create(ctx, owner, b.ID(), profapp.CreateInput{Email: barberEmail, DisplayName: "Juan"})
	if err != nil {
		t.Fatal(err)
	}
	ana, err := profs.Create(ctx, owner, b.ID(), profapp.CreateInput{Email: ownerEmail, DisplayName: "Ana"})
	if err != nil {
		t.Fatal(err)
	}

	// Round trip, including the day's edges.
	in := []domain.DayInput{
		{Weekday: "MONDAY", WorkingHours: []domain.RangeInput{{Start: "14:00", End: "19:30"}, {Start: "09:00", End: "13:00"}}, Breaks: []domain.RangeInput{{Start: "11:00", End: "11:15"}}},
		{Weekday: "SUNDAY", WorkingHours: []domain.RangeInput{{Start: "00:00", End: "23:59"}}},
	}
	want, err := profs.ReplaceSchedule(ctx, owner, b.ID(), juan.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := profs.GetSchedule(ctx, barber, b.ID(), juan.ID)
	if err != nil || !got.Equal(want) || got.Days[time.Monday].WorkingHours[0].Start.String() != "09:00" || got.Days[time.Sunday].WorkingHours[0].End.String() != "23:59" {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if n := count(t, pool, `SELECT count(*) FROM professional_working_hours WHERE business_id = $1 AND professional_id = $2`, b.ID(), juan.ID); n != 3 {
		t.Errorf("working hour rows = %d", n)
	}
	// The barber adjusts his own week, not the OWNER's.
	if _, err := profs.ReplaceSchedule(ctx, barber, b.ID(), juan.ID, in[:1]); err != nil {
		t.Errorf("barber own: %v", err)
	}
	if _, err := profs.ReplaceSchedule(ctx, barber, b.ID(), ana.ID, in); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("barber edits the owner's: %v", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM audit_log WHERE business_id = $1 AND action = 'professional.schedule_replaced'`, b.ID()); n != 2 {
		t.Errorf("schedule_replaced entries = %d", n)
	}

	// Schema: rows cannot point to a professional of another business, nor be inverted ranges.
	otherOwner, _ := activeUser(t, pool, "ACTIVE")
	b2, err := biz.CreateOwnBusiness(ctx, identitycontract.Principal{UserID: otherOwner}, bizapp.BusinessInput{Name: "Otra Barbería", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 2"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		table, constraint string
		biz               uuid.UUID
		start, end        string
	}{
		{"professional_working_hours", "professional_working_hours_professional_fk", b2.ID(), "09:00", "10:00"},
		{"professional_breaks", "professional_breaks_professional_fk", b2.ID(), "09:00", "10:00"},
		{"professional_working_hours", "professional_working_hours_range_valid", b.ID(), "10:00", "09:00"},
		{"professional_breaks", "professional_breaks_range_valid", b.ID(), "10:00", "10:00"},
	} {
		_, err := pool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (id, business_id, professional_id, weekday, start_time, end_time) VALUES ($1, $2, $3, 1, $4, $5)`, tc.table),
			uuid.Must(uuid.NewV7()), tc.biz, juan.ID, tc.start, tc.end)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != tc.constraint {
			t.Errorf("%s: %v, want %s", tc.table, err, tc.constraint)
		}
	}

	// Concurrency: N different weeks replaced at once; the result is exactly one of them.
	const n = 8
	weeks := make([]domain.WeeklySchedule, n)
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		start := fmt.Sprintf("%02d:00", 6+i)
		days := []domain.DayInput{
			{Weekday: "WEDNESDAY", WorkingHours: []domain.RangeInput{{Start: start, End: "20:00"}}, Breaks: []domain.RangeInput{{Start: "19:00", End: "19:30"}}},
			{Weekday: "THURSDAY", WorkingHours: []domain.RangeInput{{Start: start, End: "21:00"}}},
		}
		weeks[i], _ = domain.NewWeeklySchedule(days)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := profs.ReplaceSchedule(ctx, owner, b.ID(), juan.ID, days)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent replace: %v", err)
		}
	}
	final, err := profs.GetSchedule(ctx, owner, b.ID(), juan.ID)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, w := range weeks {
		if final.Equal(w) {
			matches++
		}
	}
	if matches != 1 {
		t.Errorf("final schedule is not one of the submitted weeks (mixed rows?): %+v", final.Days)
	}
	if rows := count(t, pool, `SELECT count(*) FROM professional_working_hours WHERE professional_id = $1`, juan.ID); rows != 2 {
		t.Errorf("working hour rows after the race = %d, want 2", rows)
	}

	// Removing the barber from the business ends his access to the schedule.
	if err := identity.RemoveMember(ctx, owner, b.ID(), barberID); err != nil {
		t.Fatal(err)
	}
	if _, err := profs.ReplaceSchedule(ctx, barber, b.ID(), juan.ID, nil); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("removed barber: %v", err)
	}
}
