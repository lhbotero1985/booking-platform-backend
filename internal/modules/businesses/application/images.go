package application

// Logo and cover of a business (2026-11-27, decision of the user): the OWNER uploads or removes
// them; the files go to the media store, the keys to the business row.

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
)

// ImageKind is the logo or the cover of a business.
type ImageKind string

// Kinds of business images.
const (
	ImageLogo  ImageKind = "logo"
	ImageCover ImageKind = "cover"
)

// Longest side of each kind once processed.
const (
	logoSide  = 512
	coverSide = 1600
)

// AuditBusinessImageUpdated and AuditBusinessImageRemoved are audited (metadata kind).
const (
	AuditBusinessImageUpdated = "business.image_updated"
	AuditBusinessImageRemoved = "business.image_removed"
)

// ErrMediaUnavailable means the service has no media store (misconfiguration).
var ErrMediaUnavailable = errors.New("businesses: media store not configured")

// SetMedia gives the service its media store. Call once, before serving.
func (s *Service) SetMedia(m media.Store) { s.media = m }

func (k ImageKind) folder() string {
	if k == ImageLogo {
		return media.FolderLogos
	}
	return media.FolderCovers
}

func (k ImageKind) side() int {
	if k == ImageLogo {
		return logoSide
	}
	return coverSide
}

func (k ImageKind) get(img domain.Images) string {
	if k == ImageLogo {
		return img.Logo
	}
	return img.Cover
}

func (k ImageKind) set(img *domain.Images, key string) {
	if k == ImageLogo {
		img.Logo = key
	} else {
		img.Cover = key
	}
}

// SetImage replaces the logo or the cover of a business (OWNER). The upload is read only after
// authorizing; errors of media (ErrNotMultipart, ErrTooLarge, ErrMissingFile, ErrUnsupported,
// ErrInvalid, ErrTooBig) are returned as they are.
func (s *Service) SetImage(ctx context.Context, p identity.Principal, businessID uuid.UUID, kind ImageKind, up media.Upload) (*domain.Business, error) {
	if _, err := s.authz.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return nil, err
	}
	if s.media == nil {
		return nil, ErrMediaUnavailable
	}
	data, err := up.Read()
	if err != nil {
		return nil, err
	}
	img, err := media.Process(data, kind.side())
	if err != nil {
		return nil, err
	}
	key := media.NewKey(kind.folder(), businessID, img)
	if err := s.media.Put(ctx, key, img.Data); err != nil {
		return nil, err
	}
	b, old, err := s.replaceImage(ctx, p, businessID, kind, key, AuditBusinessImageUpdated)
	if err != nil {
		_ = s.media.Delete(context.WithoutCancel(ctx), key) // nothing points to it
		return nil, err
	}
	s.dropImage(ctx, old)
	return b, nil
}

// RemoveImage removes the logo or the cover of a business (OWNER); removing a missing one is
// not an error.
func (s *Service) RemoveImage(ctx context.Context, p identity.Principal, businessID uuid.UUID, kind ImageKind) (*domain.Business, error) {
	if _, err := s.authz.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return nil, err
	}
	if s.media == nil {
		return nil, ErrMediaUnavailable
	}
	b, old, err := s.replaceImage(ctx, p, businessID, kind, "", AuditBusinessImageRemoved)
	if err != nil {
		return nil, err
	}
	s.dropImage(ctx, old)
	return b, nil
}

// replaceImage stores key as the image of that kind and returns the business and the old key.
func (s *Service) replaceImage(ctx context.Context, p identity.Principal, businessID uuid.UUID, kind ImageKind, key, action string) (*domain.Business, string, error) {
	var b *domain.Business
	var old string
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		img, err := tx.LockImages(ctx, businessID)
		if err != nil {
			return err
		}
		old = kind.get(img)
		if old == key { // removing an absent image
			b, err = tx.BusinessByID(ctx, businessID)
			return err
		}
		kind.set(&img, key)
		if err := tx.SetImages(ctx, businessID, img, s.clock.Now()); err != nil {
			return err
		}
		if b, err = tx.BusinessByID(ctx, businessID); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, action, b).Meta("kind", string(kind)))
	})
	return b, old, err
}

// dropImage deletes a file nothing points to any more. Best effort: a failure leaves an orphan
// file, never a broken image.
func (s *Service) dropImage(ctx context.Context, key string) {
	if key != "" {
		_ = s.media.Delete(context.WithoutCancel(ctx), key)
	}
}
