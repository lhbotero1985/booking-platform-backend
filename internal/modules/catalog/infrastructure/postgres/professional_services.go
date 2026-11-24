package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/infrastructure/sqlcgen"
)

func (t *txStore) ServicesOfProfessional(ctx context.Context, businessID, professionalID uuid.UUID) ([]domain.Service, error) {
	rows, err := t.q.ServicesOfProfessional(ctx, sqlcgen.ServicesOfProfessionalParams{BusinessID: businessID, ProfessionalID: professionalID})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Service, 0, len(rows))
	for _, r := range rows {
		s, err := serviceToDomain(r)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (t *txStore) ExistingServices(ctx context.Context, businessID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	found, err := t.q.ServicesByIDs(ctx, sqlcgen.ServicesByIDsParams{BusinessID: businessID, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}

func (t *txStore) ReplaceProfessionalServices(ctx context.Context, businessID, professionalID uuid.UUID, serviceIDs []uuid.UUID) error {
	if err := t.q.LockProfessionalServices(ctx, professionalID.String()); err != nil {
		return err
	}
	if err := t.q.DeleteProfessionalServices(ctx, sqlcgen.DeleteProfessionalServicesParams{BusinessID: businessID, ProfessionalID: professionalID}); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, id := range serviceIDs {
		if err := t.q.InsertProfessionalService(ctx, sqlcgen.InsertProfessionalServiceParams{
			BusinessID: businessID, ServiceID: id, ProfessionalID: professionalID, CreatedAt: now,
		}); err != nil {
			return err
		}
	}
	return nil
}
