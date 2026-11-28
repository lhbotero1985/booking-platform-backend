package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/infrastructure/sqlcgen"
)

func toPortfolioImage(r sqlcgen.ProfessionalPortfolioImage) domain.PortfolioImage {
	return domain.PortfolioImage{
		ID: r.ID, BusinessID: r.BusinessID, ProfessionalID: r.ProfessionalID, Key: r.StorageKey, Caption: r.Caption,
		SortOrder: int(r.SortOrder), Width: int(r.Width), Height: int(r.Height), CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toPortfolio(rows []sqlcgen.ProfessionalPortfolioImage, err error) ([]domain.PortfolioImage, error) {
	if err != nil {
		return nil, err
	}
	out := make([]domain.PortfolioImage, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPortfolioImage(r))
	}
	return out, nil
}

func (t *txStore) InsertPortfolioImage(ctx context.Context, img domain.PortfolioImage) error {
	return t.q.InsertPortfolioImage(ctx, sqlcgen.InsertPortfolioImageParams{
		ID: img.ID, BusinessID: img.BusinessID, ProfessionalID: img.ProfessionalID, StorageKey: img.Key, Caption: img.Caption,
		SortOrder: int32(img.SortOrder), Width: int32(img.Width), Height: int32(img.Height), //nolint:gosec // validated ranges
		CreatedBy: img.CreatedBy, CreatedAt: img.CreatedAt, UpdatedAt: img.UpdatedAt,
	})
}

func (t *txStore) ListPortfolio(ctx context.Context, businessID, professionalID uuid.UUID) ([]domain.PortfolioImage, error) {
	return toPortfolio(t.q.ListPortfolio(ctx, sqlcgen.ListPortfolioParams{BusinessID: businessID, ProfessionalID: professionalID}))
}

func (t *txStore) PortfoliosOf(ctx context.Context, professionalIDs []uuid.UUID) ([]domain.PortfolioImage, error) {
	return toPortfolio(t.q.PortfoliosOf(ctx, professionalIDs))
}

func (t *txStore) PortfolioImageByID(ctx context.Context, businessID, professionalID, id uuid.UUID) (domain.PortfolioImage, error) {
	r, err := t.q.PortfolioImageByID(ctx, sqlcgen.PortfolioImageByIDParams{BusinessID: businessID, ProfessionalID: professionalID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PortfolioImage{}, application.ErrNotFound
	}
	return toPortfolioImage(r), err
}

func (t *txStore) UpdatePortfolioImage(ctx context.Context, img domain.PortfolioImage) error {
	return t.q.UpdatePortfolioImage(ctx, sqlcgen.UpdatePortfolioImageParams{
		BusinessID: img.BusinessID, ID: img.ID, Caption: img.Caption, SortOrder: int32(img.SortOrder), UpdatedAt: img.UpdatedAt, //nolint:gosec // validated range
	})
}

func (t *txStore) DeletePortfolioImage(ctx context.Context, businessID, id uuid.UUID) error {
	return t.q.DeletePortfolioImage(ctx, sqlcgen.DeletePortfolioImageParams{BusinessID: businessID, ID: id})
}
