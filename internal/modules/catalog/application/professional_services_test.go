package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

func names(list []domain.Service) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.Name
	}
	return out
}

func TestProfessionalServices(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	juan := uuid.Must(uuid.NewV7())
	e.profs.Add(e.biz, juan)
	mk := func(name string, order int) domain.Service {
		s, err := e.svc.CreateService(ctx, e.owner, e.biz, application.ServiceInput{Name: name, DurationMinutes: ptr(30), PriceMinor: ptr(int64(2_000_000)), SortOrder: ptr(order)})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	corte, barba, tinte := mk("Corte", 0), mk("Corte + barba", 1), mk("Tinte", 2)

	// None assigned yet.
	if list, err := e.svc.ProfessionalServices(ctx, e.barber, e.biz, juan); err != nil || len(list) != 0 {
		t.Fatalf("initial: %v %v", list, err)
	}
	// The OWNER assigns "Corte" and "Corte + barba" (duplicates ignored), not "Tinte".
	list, err := e.svc.ReplaceProfessionalServices(ctx, e.owner, e.biz, juan, []uuid.UUID{barba.ID, corte.ID, barba.ID})
	if err != nil || len(list) != 2 || list[0].Name != "Corte" || list[1].Name != "Corte + barba" {
		t.Fatalf("assign: %v %v", names(list), err)
	}
	if last := e.store.Audit[len(e.store.Audit)-1]; last.Action != application.AuditProfessionalServicesReplaced || last.Metadata["services"] != "2" {
		t.Errorf("audit = %+v", last)
	}
	// Same set in another order: no-op, not audited.
	before := len(e.store.Audit)
	if _, err := e.svc.ReplaceProfessionalServices(ctx, e.owner, e.biz, juan, []uuid.UUID{corte.ID, barba.ID}); err != nil || len(e.store.Audit) != before {
		t.Errorf("no-op audited: %v", err)
	}
	// The barber reads but cannot change it.
	if got, err := e.svc.ProfessionalServices(ctx, e.barber, e.biz, juan); err != nil || len(got) != 2 {
		t.Errorf("barber reads: %v %v", names(got), err)
	}
	if _, err := e.svc.ReplaceProfessionalServices(ctx, e.barber, e.biz, juan, []uuid.UUID{tinte.ID}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("barber assigns: %v", err)
	}
	// Unknown services or services of another business are reported by position.
	other := uuid.Must(uuid.NewV7())
	admin := identity.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	foreign, err := e.svc.CreateService(ctx, admin, other, application.ServiceInput{Name: "Ajeno", DurationMinutes: ptr(30), PriceMinor: ptr(int64(0))})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.ReplaceProfessionalServices(ctx, e.owner, e.biz, juan, []uuid.UUID{tinte.ID, foreign.ID, uuid.Must(uuid.NewV7())})
	if got := fieldCodes(t, err); len(got) != 2 || got["serviceIds[1]"] != application.CodeServiceNotFound || got["serviceIds[2]"] != application.CodeServiceNotFound {
		t.Errorf("fields = %v", got)
	}
	tooMany := make([]uuid.UUID, application.MaxServicesPerPro+1)
	if got := fieldCodes(t, mustList(e.svc.ReplaceProfessionalServices(ctx, e.owner, e.biz, juan, tooMany))); got["serviceIds"] != application.CodeTooManyServices {
		t.Errorf("too many: %v", got)
	}
	// Unknown professional (or of another business): 404.
	if _, err := e.svc.ReplaceProfessionalServices(ctx, e.owner, e.biz, uuid.Must(uuid.NewV7()), nil); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown professional: %v", err)
	}
	if _, err := e.svc.ProfessionalServices(ctx, admin, other, juan); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("professional through another business: %v", err)
	}
	// A stranger: 404.
	if _, err := e.svc.ProfessionalServices(ctx, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, e.biz, juan); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
	// Clearing the list.
	if list, err := e.svc.ReplaceProfessionalServices(ctx, e.owner, e.biz, juan, nil); err != nil || len(list) != 0 {
		t.Errorf("clear: %v %v", names(list), err)
	}
}

func mustList(_ []domain.Service, err error) error { return err }
