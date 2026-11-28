package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
)

// slugTx is a Tx whose only behavior is a set of taken slugs.
type slugTx struct {
	taken map[domain.Slug]bool
	err   error
}

func (t slugTx) SlugTaken(_ context.Context, s domain.Slug) (bool, error) { return t.taken[s], t.err }
func (slugTx) InsertBusiness(context.Context, *domain.Business) error     { return nil }
func (slugTx) UpdateBusiness(context.Context, *domain.Business) error     { return nil }
func (slugTx) BusinessByID(context.Context, uuid.UUID) (*domain.Business, error) {
	return nil, application.ErrNotFound
}
func (slugTx) BusinessBySlug(context.Context, domain.Slug) (*domain.Business, error) {
	return nil, application.ErrNotFound
}
func (slugTx) AppendAudit(context.Context, *audit.Entry) error { return nil }
func (slugTx) ClaimCreationKey(context.Context, uuid.UUID, string, string, time.Time) (bool, error) {
	return true, nil
}
func (slugTx) CreationKeyOf(context.Context, uuid.UUID, string) (string, uuid.UUID, error) {
	return "", uuid.Nil, application.ErrNotFound
}
func (slugTx) SetCreationKeyBusiness(context.Context, uuid.UUID, string, uuid.UUID) error { return nil }
func (slugTx) LockImages(context.Context, uuid.UUID) (domain.Images, error) {
	return domain.Images{}, application.ErrNotFound
}
func (slugTx) SetImages(context.Context, uuid.UUID, domain.Images, time.Time) error { return nil }
func (slugTx) ListBusinesses(context.Context, application.ListFilter) ([]*domain.Business, error) {
	return nil, nil
}
func (slugTx) SearchActive(context.Context, application.SearchFilter) ([]application.SearchHit, error) {
	return nil, nil
}

func TestAllocateSlug(t *testing.T) {
	ctx := context.Background()
	tx := slugTx{taken: map[domain.Slug]bool{}}
	if s, err := application.AllocateSlug(ctx, tx, "Barbería El Paisa"); err != nil || s != "barberia-el-paisa" {
		t.Fatalf("free slug = %q, %v", s, err)
	}
	tx.taken["barberia-el-paisa"] = true
	tx.taken["barberia-el-paisa-2"] = true
	if s, err := application.AllocateSlug(ctx, tx, "Barbería El Paisa"); err != nil || s != "barberia-el-paisa-3" {
		t.Fatalf("with collisions = %q, %v", s, err)
	}

	// Exhausted: every candidate taken.
	full := slugTx{taken: map[domain.Slug]bool{"cali": true}}
	for n := 2; n <= 50; n++ {
		full.taken[domain.Slug("cali").WithSuffix(n)] = true
	}
	if _, err := application.AllocateSlug(ctx, full, "Cali"); !errors.Is(err, application.ErrSlugTaken) {
		t.Errorf("exhausted: err = %v, want ErrSlugTaken", err)
	}
	boom := errors.New("db down")
	if _, err := application.AllocateSlug(ctx, slugTx{err: boom}, "Cali"); !errors.Is(err, boom) {
		t.Errorf("store error not propagated: %v", err)
	}
}
