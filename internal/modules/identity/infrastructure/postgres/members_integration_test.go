//go:build integration

package postgres_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestBusinessAlwaysKeepsAnOwner: two OWNERs remove each other at the same time; the OWNER
// rows are locked, so exactly one removal succeeds and the business keeps an OWNER.
func TestBusinessAlwaysKeepsAnOwner(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	biz := uuid.Must(uuid.NewV7())
	if err := pgtest.InsertBusiness(ctx, pool, biz); err != nil {
		t.Fatal(err)
	}
	admin := contract.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	emails := []string{e.activeUser(t), e.activeUser(t)}
	ids := make([]uuid.UUID, 2)
	for i, email := range emails {
		var err error
		if ids[i], err = e.svc.AddMember(ctx, admin, biz, email, contract.RoleOwner); err != nil {
			t.Fatal(err)
		}
	}
	// Several OWNERs are allowed now (migration 00008).
	if ms, err := e.svc.ListMembers(ctx, admin, biz); err != nil || len(ms) != 2 {
		t.Fatalf("members = %+v %v", ms, err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Each OWNER removes the other.
			errs[i] = e.svc.RemoveMember(ctx, contract.Principal{UserID: ids[i]}, biz, ids[1-i])
		}(i)
	}
	wg.Wait()
	ok, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, contract.ErrLastOwner), errors.Is(err, contract.ErrNotFound):
			// ErrNotFound: the loser was itself removed before its authorization check.
			refused++
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	var owners int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE business_id = $1 AND role = 'OWNER' AND status = 'ACTIVE'`, biz).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if ok != 1 || refused != 1 || owners != 1 {
		t.Errorf("concurrent removals: %d ok, %d refused, %d owners left; want 1, 1, 1", ok, refused, owners)
	}

	// A removed member is reactivated with the new role (same row, unique per user and business).
	removed := ids[0]
	if owners == 1 {
		var stillOwner uuid.UUID
		_ = pool.QueryRow(ctx, `SELECT user_id FROM memberships WHERE business_id = $1 AND role = 'OWNER' AND status = 'ACTIVE'`, biz).Scan(&stillOwner)
		if stillOwner == ids[0] {
			removed = ids[1]
		}
	}
	var email string
	_ = pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, removed).Scan(&email)
	if _, err := e.svc.AddMember(ctx, admin, biz, email, contract.RoleProfessional); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	var rows int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE business_id = $1 AND user_id = $2`, biz, removed).Scan(&rows)
	if rows != 1 {
		t.Errorf("membership rows for the re-added user = %d, want 1 (reactivated)", rows)
	}
	if _, err := e.svc.AddMember(ctx, admin, biz, email, contract.RoleProfessional); !errors.Is(err, contract.ErrAlreadyMember) {
		t.Errorf("adding an active member again: %v", err)
	}
}
