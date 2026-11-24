//go:build integration

package app

import (
	"errors"
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
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestProfessionalAbsencesOnPostgres (T5.4): absences in UTC, the barber manages his own, the
// exclusion constraint rejects overlaps (also on update and under concurrent inserts) and the
// schema keeps rows inside the professional's business.
func TestProfessionalAbsencesOnPostgres(t *testing.T) {
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
	b, err := biz.CreateOwnBusiness(ctx, owner, bizapp.BusinessInput{Name: "Barbería Ausencias", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 1"})
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
	day := func(d, h int) time.Time { return time.Date(2031, 3, d, h, 0, 0, 0, time.UTC) }

	cot := time.FixedZone("COT", -5*3600)
	sick, err := profs.CreateAbsence(ctx, barber, b.ID(), juan.ID, profapp.AbsenceInput{
		Start: time.Date(2031, 3, 2, 8, 0, 0, 0, cot), End: time.Date(2031, 3, 3, 19, 0, 0, 0, cot), Reason: "Incapacidad",
	})
	if err != nil || !sick.Start.Equal(day(2, 13)) {
		t.Fatalf("barber sick leave: %+v %v", sick, err)
	}
	vac, err := profs.CreateAbsence(ctx, owner, b.ID(), juan.ID, profapp.AbsenceInput{Start: day(10, 0), End: day(17, 0), Reason: "Vacaciones"})
	if err != nil {
		t.Fatal(err)
	}
	// Overlap rejected by the database; touching allowed; other professionals unaffected.
	if _, err := profs.CreateAbsence(ctx, owner, b.ID(), juan.ID, profapp.AbsenceInput{Start: day(3, 0), End: day(4, 0)}); !errors.Is(err, profapp.ErrAbsenceOverlap) {
		t.Errorf("overlap: %v", err)
	}
	if _, err := profs.CreateAbsence(ctx, owner, b.ID(), juan.ID, profapp.AbsenceInput{Start: day(17, 0), End: day(18, 0)}); err != nil {
		t.Errorf("touching: %v", err)
	}
	if _, err := profs.CreateAbsence(ctx, owner, b.ID(), ana.ID, profapp.AbsenceInput{Start: day(10, 0), End: day(17, 0)}); err != nil {
		t.Errorf("another professional, same period: %v", err)
	}
	early := day(3, 12)
	if _, err := profs.UpdateAbsence(ctx, barber, b.ID(), juan.ID, vac.ID, profapp.AbsencePatch{Start: &early}); !errors.Is(err, profapp.ErrAbsenceOverlap) {
		t.Errorf("update overlap: %v", err)
	}
	if _, err := profs.CreateAbsence(ctx, barber, b.ID(), ana.ID, profapp.AbsenceInput{Start: day(20, 0), End: day(21, 0)}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("barber for the owner: %v", err)
	}
	from := day(1, 0)
	list, err := profs.ListAbsences(ctx, barber, b.ID(), juan.ID, &from, nil)
	if err != nil || len(list) != 3 || list[0].ID != sick.ID || list[0].Reason != "Incapacidad" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if err := profs.DeleteAbsence(ctx, barber, b.ID(), juan.ID, sick.ID); err != nil {
		t.Errorf("delete own: %v", err)
	}
	for action, want := range map[string]int{"professional.absence_created": 4, "professional.absence_deleted": 1} {
		if n := count(t, pool, `SELECT count(*) FROM audit_log WHERE business_id = $1 AND action = $2`, b.ID(), action); n != want {
			t.Errorf("%s entries = %d, want %d", action, n, want)
		}
	}

	// Schema: composite FK, period checks.
	otherOwner, _ := activeUser(t, pool, "ACTIVE")
	b2, err := biz.CreateOwnBusiness(ctx, identitycontract.Principal{UserID: otherOwner}, bizapp.BusinessInput{Name: "Otra Barbería", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 2"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		constraint string
		biz        uuid.UUID
		start, end time.Time
	}{
		{"professional_absences_professional_fk", b2.ID(), day(25, 0), day(26, 0)},
		{"professional_absences_period_valid", b.ID(), day(26, 0), day(25, 0)},
		{"professional_absences_period_max", b.ID(), day(1, 0).AddDate(1, 0, 0), day(1, 0).AddDate(1, 0, 367)},
	} {
		_, err := pool.Exec(ctx, `INSERT INTO professional_absences (id, business_id, professional_id, starts_at, ends_at, created_by, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, now(), now())`,
			uuid.Must(uuid.NewV7()), tc.biz, juan.ID, tc.start, tc.end, ownerID)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != tc.constraint {
			t.Errorf("%s: %v", tc.constraint, err)
		}
	}

	// Concurrency: N overlapping absences at once → exactly one is stored.
	const n = 8
	var wg sync.WaitGroup
	results := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := day(22, i) // all overlap 22nd 08:00–23rd 00:00
			_, err := profs.CreateAbsence(ctx, owner, b.ID(), juan.ID, profapp.AbsenceInput{Start: start, End: day(23, 0)})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok, overlaps := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, profapp.ErrAbsenceOverlap):
			overlaps++
		default:
			t.Errorf("concurrent create: %v", err)
		}
	}
	if ok != 1 || overlaps != n-1 {
		t.Errorf("concurrent creates: %d stored, %d overlaps", ok, overlaps)
	}
	if rows := count(t, pool, `SELECT count(*) FROM professional_absences WHERE professional_id = $1 AND starts_at >= $2 AND starts_at < $3`, juan.ID, day(22, 0), day(23, 0)); rows != 1 {
		t.Errorf("rows on the 22nd = %d", rows)
	}
}
