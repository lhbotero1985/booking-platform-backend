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
func TestRegistrationRequiresPhone(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	var verr *application.ValidationError
	for phone, code := range map[string]string{"": application.CodeRequired, "  ": application.CodeRequired, "12345": application.CodeInvalidPhone} {
		err := e.svc.Register(ctx, application.RegisterInput{Email: "ana@example.com", Password: goodPassword, FullName: "Ana", Phone: phone})
		if !errors.As(err, &verr) || len(verr.Fields) != 1 || verr.Fields[0].Field != "phone" || verr.Fields[0].Code != code {
			t.Errorf("phone %q: %v", phone, err)
		}
	}
	if e.store.UserCount() != 0 {
		t.Fatal("an invalid registration created an account")
	}
	if err := e.svc.Register(ctx, application.RegisterInput{Email: "ana@example.com", Password: goodPassword, FullName: "Ana", Phone: "(300) 765-4321"}); err != nil {
		t.Fatal(err)
	}
	if got := e.store.User("ana@example.com").Phone(); got != "+573007654321" {
		t.Errorf("stored phone = %q", got)
	}
}

func TestPhoneAndCustomerProfile(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	id := e.store.User("ana@example.com").ID()
	p := contract.Principal{UserID: id}
	if got := e.store.User("ana@example.com").Phone(); got != "+573001234567" {
		t.Fatalf("phone from the registration = %q", got)
	}
	e.store.User("ana@example.com").WithPhone("") // an account created before phones were mandatory
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
	if err != nil || c != (contract.Customer{UserID: id, FullName: "Ana Pérez", Phone: "+573001234567", Email: "ana@example.com"}) {
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
