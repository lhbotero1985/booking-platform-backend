// Package application holds the businesses use cases (T4.3+) and their ports.
//
// Rules (enforced by depguard): may import domain, the identity and geography contracts and
// platform packages; no HTTP, database drivers or River.
package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
)

// Repository errors returned by Tx implementations.
var (
	ErrNotFound  = errors.New("businesses: not found")
	ErrSlugTaken = errors.New("businesses: slug already taken")
)

// Store runs a unit of work in a database transaction. fn receives a context carrying the
// transaction: passing it to another module's contract makes that module join it, so changes
// of both modules commit or roll back together.
type Store interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// ListFilter selects businesses for the administrator's list (keyset pagination by ID).
type ListFilter struct {
	Status domain.Status // "" = any
	CityID uuid.UUID     // Nil = any
	After  uuid.UUID     // Nil = from the start
	Limit  int
}

// Tx is the persistence available inside a transaction.
type Tx interface {
	// InsertBusiness returns ErrSlugTaken when the slug is in use.
	InsertBusiness(ctx context.Context, b *domain.Business) error
	UpdateBusiness(ctx context.Context, b *domain.Business) error
	// BusinessByID and BusinessBySlug return ErrNotFound when absent.
	BusinessByID(ctx context.Context, id uuid.UUID) (*domain.Business, error)
	BusinessBySlug(ctx context.Context, slug domain.Slug) (*domain.Business, error)
	SlugTaken(ctx context.Context, slug domain.Slug) (bool, error)
	ListBusinesses(ctx context.Context, f ListFilter) ([]*domain.Business, error)
	// SearchActive returns up to f.Limit ACTIVE businesses of f.CityID after the keyset.
	SearchActive(ctx context.Context, f SearchFilter) ([]SearchHit, error)
	AppendAudit(ctx context.Context, e *audit.Entry) error
}

// maxSlugAttempts bounds the "-2", "-3"… search for a free slug.
const maxSlugAttempts = 50

// AllocateSlug returns the first free slug for name: the derived slug, then "-2", "-3"…
// A concurrent insert can still take it between the check and the insert; InsertBusiness then
// returns ErrSlugTaken and the caller retries.
func AllocateSlug(ctx context.Context, tx Tx, name string) (domain.Slug, error) {
	base := domain.SlugFromName(name)
	for n := 1; n <= maxSlugAttempts; n++ {
		candidate := base
		if n > 1 {
			candidate = base.WithSuffix(n)
		}
		taken, err := tx.SlugTaken(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: no free slug for %q after %d attempts", ErrSlugTaken, base, maxSlugAttempts)
}
