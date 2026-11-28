package application

// Portfolio of a professional (2026-11-27, decisions of the user): up to 10 images of its best
// work, managed by the professional itself and by the OWNER; customers see them in the public
// catalog.

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
)

// portfolioSide is the longest side of a portfolio image once processed.
const portfolioSide = 1600

// Field codes and audited actions of the portfolio.
const (
	CodeInvalidCaption = "INVALID_CAPTION"

	AuditPortfolioAdded   = "professional.portfolio_image_added"
	AuditPortfolioUpdated = "professional.portfolio_image_updated"
	AuditPortfolioDeleted = "professional.portfolio_image_deleted"
)

var (
	// ErrPortfolioFull means the professional already shows domain.MaxPortfolioImages images.
	ErrPortfolioFull = errors.New("professionals: the portfolio already has the most images allowed")
	// ErrMediaUnavailable means the service has no media store (misconfiguration).
	ErrMediaUnavailable = errors.New("professionals: media store not configured")
)

// SetMedia gives the service its media store (composition root, before serving).
func (s *Service) SetMedia(m media.Store) { s.media = m }

// PortfolioPatch changes the given fields (nil = unchanged).
type PortfolioPatch struct {
	Caption   *string
	SortOrder *int
}

// manageable checks, before reading any upload, that the caller may manage the professional's
// portfolio: the OWNER any of the business, a PROFESSIONAL its own (another → authz.ErrForbidden).
func (s *Service) manageable(ctx context.Context, p identity.Principal, businessID, profID uuid.UUID) (identity.Role, error) {
	role, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return "", err
	}
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		prof, err := tx.ProfessionalByID(ctx, businessID, profID)
		if err != nil {
			return err
		}
		if role != identity.RoleOwner && prof.UserID != p.UserID {
			return authz.ErrForbidden
		}
		return nil
	})
	return role, err
}

// ListPortfolio returns the portfolio of a professional to the staff of the business, in order.
func (s *Service) ListPortfolio(ctx context.Context, p identity.Principal, businessID, profID uuid.UUID) ([]domain.PortfolioImage, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return nil, err
	}
	var out []domain.PortfolioImage
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if _, err := tx.ProfessionalByID(ctx, businessID, profID); err != nil {
			return err
		}
		var err error
		out, err = tx.ListPortfolio(ctx, businessID, profID)
		return err
	})
	return out, err
}

// AddPortfolioImage adds an image (form field "caption" optional) at the end of the portfolio.
// The upload is read only after authorizing; errors of media are returned as they are;
// ErrPortfolioFull beyond domain.MaxPortfolioImages.
func (s *Service) AddPortfolioImage(ctx context.Context, p identity.Principal, businessID, profID uuid.UUID, up media.Upload) (domain.PortfolioImage, error) {
	if _, err := s.manageable(ctx, p, businessID, profID); err != nil {
		return domain.PortfolioImage{}, err
	}
	if s.media == nil {
		return domain.PortfolioImage{}, ErrMediaUnavailable
	}
	data, err := up.Read()
	if err != nil {
		return domain.PortfolioImage{}, err
	}
	caption, err := domain.ParseCaption(up.Field("caption"))
	if err != nil {
		return domain.PortfolioImage{}, &ValidationError{Fields: []FieldError{{"caption", CodeInvalidCaption}}}
	}
	img, err := media.Process(data, portfolioSide)
	if err != nil {
		return domain.PortfolioImage{}, err
	}
	key := media.NewKey(media.FolderPortfolio, businessID, img)
	if err := s.media.Put(ctx, key, img.Data); err != nil {
		return domain.PortfolioImage{}, err
	}
	var out domain.PortfolioImage
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		// The professional's row lock serializes two uploads: the limit holds.
		prof, err := tx.LockProfessional(ctx, businessID, profID)
		if err != nil {
			return err
		}
		current, err := tx.ListPortfolio(ctx, businessID, profID)
		if err != nil {
			return err
		}
		if len(current) >= domain.MaxPortfolioImages {
			return ErrPortfolioFull
		}
		next := 0
		for _, c := range current {
			next = max(next, c.SortOrder+1)
		}
		now := s.clock.Now()
		out = domain.PortfolioImage{
			ID: uuid.Must(uuid.NewV7()), BusinessID: businessID, ProfessionalID: profID, Key: key, Caption: caption,
			SortOrder: min(next, 1000), Width: img.Width, Height: img.Height, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.InsertPortfolioImage(ctx, out); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.portfolioAudit(ctx, p, AuditPortfolioAdded, prof, out))
	})
	if err != nil {
		_ = s.media.Delete(context.WithoutCancel(ctx), key) // nothing points to it
		return domain.PortfolioImage{}, err
	}
	return out, nil
}

// UpdatePortfolioImage changes the caption or the position of an image.
func (s *Service) UpdatePortfolioImage(ctx context.Context, p identity.Principal, businessID, profID, id uuid.UUID, in PortfolioPatch) (domain.PortfolioImage, error) {
	if _, err := s.manageable(ctx, p, businessID, profID); err != nil {
		return domain.PortfolioImage{}, err
	}
	var verr ValidationError
	var caption string
	if in.Caption != nil {
		var err error
		if caption, err = domain.ParseCaption(*in.Caption); err != nil {
			verr.add("caption", CodeInvalidCaption)
		}
	}
	if in.SortOrder != nil && (*in.SortOrder < 0 || *in.SortOrder > 1000) {
		verr.add("sortOrder", CodeInvalidSortOrder)
	}
	if err := verr.orNil(); err != nil {
		return domain.PortfolioImage{}, err
	}
	var out domain.PortfolioImage
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		prof, err := tx.LockProfessional(ctx, businessID, profID)
		if err != nil {
			return err
		}
		if out, err = tx.PortfolioImageByID(ctx, businessID, profID, id); err != nil {
			return err
		}
		if in.Caption != nil {
			out.Caption = caption
		}
		if in.SortOrder != nil {
			out.SortOrder = *in.SortOrder
		}
		out.UpdatedAt = s.clock.Now()
		if err := tx.UpdatePortfolioImage(ctx, out); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.portfolioAudit(ctx, p, AuditPortfolioUpdated, prof, out))
	})
	return out, err
}

// DeletePortfolioImage removes an image and its file.
func (s *Service) DeletePortfolioImage(ctx context.Context, p identity.Principal, businessID, profID, id uuid.UUID) error {
	if _, err := s.manageable(ctx, p, businessID, profID); err != nil {
		return err
	}
	if s.media == nil {
		return ErrMediaUnavailable
	}
	var key string
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		prof, err := tx.LockProfessional(ctx, businessID, profID)
		if err != nil {
			return err
		}
		img, err := tx.PortfolioImageByID(ctx, businessID, profID, id)
		if err != nil {
			return err
		}
		key = img.Key
		if err := tx.DeletePortfolioImage(ctx, businessID, id); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.portfolioAudit(ctx, p, AuditPortfolioDeleted, prof, img))
	})
	if err != nil {
		return err
	}
	_ = s.media.Delete(context.WithoutCancel(ctx), key) // best effort: an orphan file, never a broken image
	return nil
}

func (s *Service) portfolioAudit(ctx context.Context, p identity.Principal, action string, prof domain.Professional, img domain.PortfolioImage) *audit.Entry {
	return s.audit(ctx, p, action, prof).Meta("image", img.ID.String()).Meta("self", strconv.FormatBool(prof.UserID == p.UserID))
}
