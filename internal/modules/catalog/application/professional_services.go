package application

// Services each professional performs (T5.6): the OWNER assigns them; the staff reads them.

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
)

// AuditProfessionalServicesReplaced is audited when a professional's services change.
const AuditProfessionalServicesReplaced = "catalog.professional_services_replaced"

// Field error codes and limits.
const (
	CodeServiceNotFound = "SERVICE_NOT_FOUND"
	CodeTooManyServices = "TOO_MANY_SERVICES"
	MaxServicesPerPro   = 200
)

// ProfessionalServicesTx is the persistence of the professional ↔ service assignment.
type ProfessionalServicesTx interface {
	// ServicesOfProfessional returns the services assigned to a professional, by sort order and name.
	ServicesOfProfessional(ctx context.Context, businessID, professionalID uuid.UUID) ([]domain.Service, error)
	// ExistingServices returns which of ids are services of the business.
	ExistingServices(ctx context.Context, businessID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error)
	// ReplaceProfessionalServices replaces the whole list; concurrent replacements of the same
	// professional run one after the other.
	ReplaceProfessionalServices(ctx context.Context, businessID, professionalID uuid.UUID, serviceIDs []uuid.UUID) error
}

// ProfessionalServices returns the services a professional performs (staff of its business).
func (s *Service) ProfessionalServices(ctx context.Context, p identity.Principal, businessID, professionalID uuid.UUID) ([]domain.Service, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return nil, err
	}
	if err := s.requireProfessional(ctx, businessID, professionalID); err != nil {
		return nil, err
	}
	var out []domain.Service
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		out, err = tx.ServicesOfProfessional(ctx, businessID, professionalID)
		return err
	})
	return out, err
}

// ReplaceProfessionalServices sets the services a professional performs (OWNER). Every id must
// be a service of the business (active or not); duplicates are ignored. An identical list is a
// no-op; changes are audited.
func (s *Service) ReplaceProfessionalServices(ctx context.Context, p identity.Principal, businessID, professionalID uuid.UUID, serviceIDs []uuid.UUID) ([]domain.Service, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return nil, err
	}
	if len(serviceIDs) > MaxServicesPerPro {
		return nil, &ValidationError{Fields: []FieldError{{"serviceIds", CodeTooManyServices}}}
	}
	if err := s.requireProfessional(ctx, businessID, professionalID); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(serviceIDs))
	seen := map[uuid.UUID]bool{}
	for _, id := range serviceIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	var out []domain.Service
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		exists, err := tx.ExistingServices(ctx, businessID, ids)
		if err != nil {
			return err
		}
		var verr ValidationError
		for i, id := range serviceIDs {
			if !exists[id] {
				verr.add(fmt.Sprintf("serviceIds[%d]", i), CodeServiceNotFound)
			}
		}
		if err := verr.orNil(); err != nil {
			return err
		}
		current, err := tx.ServicesOfProfessional(ctx, businessID, professionalID)
		if err != nil {
			return err
		}
		if sameServices(current, seen) {
			out = current
			return nil
		}
		if err := tx.ReplaceProfessionalServices(ctx, businessID, professionalID, ids); err != nil {
			return err
		}
		if out, err = tx.ServicesOfProfessional(ctx, businessID, professionalID); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditProfessionalServicesReplaced, businessID, "professional", professionalID).
			Meta("services", strconv.Itoa(len(ids))))
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) requireProfessional(ctx context.Context, businessID, professionalID uuid.UUID) error {
	ok, err := s.professionals.IsProfessionalOf(ctx, businessID, professionalID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func sameServices(current []domain.Service, want map[uuid.UUID]bool) bool {
	if len(current) != len(want) {
		return false
	}
	for _, s := range current {
		if !want[s.ID] {
			return false
		}
	}
	return true
}
