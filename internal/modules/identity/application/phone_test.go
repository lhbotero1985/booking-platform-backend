package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// TestPhoneAndCustomerProfile (T6.3): the phone customers need to book.
func TestPhoneAndCustomerProfile(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	id := e.store.User("ana@example.com").ID()
	p := contract.Principal{UserID: id}
	if _, err := e.svc.CustomerProfile(ctx, id); !errors.Is(err, contract.ErrPhoneRequired) {
		t.Errorf("no phone yet: %v", err)
	}
	var verr *application.ValidationError
	for _, bad := range []string{"", "12345", "2001234567", "+0573001234567", "abc"} {
		if _, err := e.svc.SetPhone(ctx, p, bad); !errors.As(err, &verr) || verr.Fields[0].Code != application.CodeInvalidPhone {
			t.Errorf("%q: %v", bad, err)
		}
	}
	before := len(e.store.AuditActions())
	u, err := e.svc.SetPhone(ctx, p, " (300) 123-4567 ")
	if err != nil || u.Phone() != "+573001234567" {
		t.Fatalf("set: %v %v", u, err)
	}
	if _, err := e.svc.SetPhone(ctx, p, "+57 300 123 4567"); err != nil { // same number: no new audit
		t.Fatal(err)
	}
	if got := e.store.AuditActions()[before:]; len(got) != 1 || got[0] != application.AuditPhoneChanged+":SUCCESS" {
		t.Errorf("audit = %v", got)
	}
	c, err := e.svc.CustomerProfile(ctx, id)
	if err != nil || c != (contract.Customer{UserID: id, FullName: "Ana Pérez", Phone: "+573001234567"}) {
		t.Errorf("profile = %+v %v", c, err)
	}
	if u, err := e.svc.SetPhone(ctx, p, "+1 415 555 0100"); err != nil || u.Phone() != "+14155550100" {
		t.Errorf("E.164 abroad: %v %v", u, err)
	}
	if _, err := e.svc.SetPhone(ctx, contract.Principal{UserID: uuid.Must(uuid.NewV7())}, "3001234567"); err == nil {
		t.Error("unknown account")
	}
	if _, err := e.svc.CustomerProfile(ctx, uuid.Must(uuid.NewV7())); err == nil {
		t.Error("unknown customer")
	}
	// A suspended account cannot book.
	e.store.User("ana@example.com").Suspend(e.clock.Now())
	if _, err := e.svc.CustomerProfile(ctx, id); !errors.Is(err, domain.ErrUserSuspended) {
		t.Errorf("suspended: %v", err)
	}
}
