package application

// Public view of the catalog (T5.7): what customers can book, without authentication.

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
)

// Assignment is one professional performing one ACTIVE service.
type Assignment struct {
	BusinessID, ServiceID, ProfessionalID uuid.UUID
}

// PublicTx is the persistence of the public view.
type PublicTx interface {
	// ActiveAssignmentsIn returns the assignments of ACTIVE services in the businesses.
	ActiveAssignmentsIn(ctx context.Context, businessIDs []uuid.UUID) ([]Assignment, error)
}

// PublicDeps are the contracts the public view needs (set with SetPublicDeps).
type PublicDeps struct {
	Directory     businesses.PublicDirectory
	Professionals professionals.Bookable
}

// errNotConfigured: SetPublicDeps was not called by the composition root.
var errNotConfigured = errors.New("catalog: public view not configured")

// SetPublicDeps plugs the public view's contracts (composition root only, before serving).
func (s *Service) SetPublicDeps(d PublicDeps) { s.public = d }

// PublicService is a bookable service with the professionals who perform it.
type PublicService struct {
	domain.Service
	ProfessionalIDs []uuid.UUID
}

// PublicProfessional is a bookable professional with the services it performs.
type PublicProfessional struct {
	professionals.PublicProfessional
	ServiceIDs []uuid.UUID
}

// PublicCatalog is what customers see of a business's offer.
type PublicCatalog struct {
	Categories    []domain.Category // ACTIVE and used by a listed service
	Services      []PublicService
	Professionals []PublicProfessional
}

// BookableAmong implements the catalog contract Offerings: businesses with at least one ACTIVE
// service performed by at least one bookable professional.
func (s *Service) BookableAmong(ctx context.Context, businessIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{}
	if len(businessIDs) == 0 {
		return out, nil
	}
	if s.public.Professionals == nil {
		return nil, errNotConfigured
	}
	var as []Assignment
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		as, err = tx.ActiveAssignmentsIn(ctx, businessIDs)
		return err
	})
	if err != nil || len(as) == 0 {
		return out, err
	}
	withAssignments := uniqueBusinesses(as)
	bookable, err := s.public.Professionals.BookableProfessionals(ctx, withAssignments)
	if err != nil {
		return nil, err
	}
	ok := bookableSet(bookable)
	for _, a := range as {
		if ok[a.ProfessionalID] {
			out[a.BusinessID] = true
		}
	}
	return out, nil
}

// PublicCatalog returns the bookable offer of a business customers can see (ErrNotFound if the
// business is not public): ACTIVE services performed by at least one bookable professional, those
// professionals, and the ACTIVE categories in use. Ordered by sort order and name.
func (s *Service) PublicCatalog(ctx context.Context, slug string) (PublicCatalog, error) {
	if s.public.Directory == nil || s.public.Professionals == nil {
		return PublicCatalog{}, errNotConfigured
	}
	businessID, err := s.public.Directory.PublicBusinessID(ctx, slug)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return PublicCatalog{}, ErrNotFound
	}
	if err != nil {
		return PublicCatalog{}, err
	}
	bookable, err := s.public.Professionals.BookableProfessionals(ctx, []uuid.UUID{businessID})
	if err != nil {
		return PublicCatalog{}, err
	}
	pros := bookable[businessID]
	okPro := bookableSet(bookable)

	var (
		services   []domain.Service
		categories []domain.Category
		as         []Assignment
	)
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		if services, err = tx.ListServices(ctx, businessID); err != nil {
			return err
		}
		if categories, err = tx.ListCategories(ctx, businessID); err != nil {
			return err
		}
		as, err = tx.ActiveAssignmentsIn(ctx, []uuid.UUID{businessID})
		return err
	})
	if err != nil {
		return PublicCatalog{}, err
	}

	byService := map[uuid.UUID][]uuid.UUID{}
	byPro := map[uuid.UUID][]uuid.UUID{}
	for _, a := range as {
		if okPro[a.ProfessionalID] {
			byService[a.ServiceID] = append(byService[a.ServiceID], a.ProfessionalID)
			byPro[a.ProfessionalID] = append(byPro[a.ProfessionalID], a.ServiceID)
		}
	}
	activeCategory := map[uuid.UUID]bool{}
	for _, c := range categories {
		activeCategory[c.ID] = c.Status == domain.StatusActive
	}
	var out PublicCatalog
	used := map[uuid.UUID]bool{}
	listed := map[uuid.UUID]bool{}
	for _, svc := range services { // already ordered
		if svc.Status != domain.StatusActive || len(byService[svc.ID]) == 0 {
			continue
		}
		if !activeCategory[svc.CategoryID] {
			svc.CategoryID = uuid.Nil // an inactive category is not shown
		} else {
			used[svc.CategoryID] = true
		}
		listed[svc.ID] = true
		out.Services = append(out.Services, PublicService{Service: svc, ProfessionalIDs: ordered(byService[svc.ID], pros)})
	}
	for _, c := range categories {
		if used[c.ID] {
			out.Categories = append(out.Categories, c)
		}
	}
	serviceOrder := map[uuid.UUID]int{}
	for i, svc := range out.Services {
		serviceOrder[svc.ID] = i
	}
	for _, p := range pros { // already ordered
		var ids []uuid.UUID
		for _, id := range byPro[p.ID] {
			if listed[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		sort.Slice(ids, func(i, j int) bool { return serviceOrder[ids[i]] < serviceOrder[ids[j]] })
		out.Professionals = append(out.Professionals, PublicProfessional{PublicProfessional: p, ServiceIDs: ids})
	}
	return out, nil
}

func uniqueBusinesses(as []Assignment) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	for _, a := range as {
		if !seen[a.BusinessID] {
			seen[a.BusinessID] = true
			out = append(out, a.BusinessID)
		}
	}
	return out
}

func bookableSet(m map[uuid.UUID][]professionals.PublicProfessional) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, list := range m {
		for _, p := range list {
			out[p.ID] = true
		}
	}
	return out
}

// ordered returns ids in the order of the professionals list.
func ordered(ids []uuid.UUID, pros []professionals.PublicProfessional) []uuid.UUID {
	want := map[uuid.UUID]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, p := range pros {
		if want[p.ID] {
			out = append(out, p.ID)
		}
	}
	return out
}
