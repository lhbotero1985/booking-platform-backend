package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

func TestCustomers(t *testing.T) {
	ctx := context.Background()
	e := newBookingEnv(t)
	owner, barber, outsider := identity.Principal{UserID: uuid.Must(uuid.NewV7())}, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, identity.Principal{UserID: uuid.Must(uuid.NewV7())}
	e.roles[[2]uuid.UUID{e.biz, owner.UserID}] = identity.RoleOwner
	e.roles[[2]uuid.UUID{e.biz, barber.UserID}] = identity.RoleProfessional

	var verr *application.ValidationError
	for name, in := range map[string]application.CustomerInput{
		"no name":   {Phone: "3001234567"},
		"no phone":  {FullName: "Carlos"},
		"bad phone": {FullName: "Carlos", Phone: "12345"},
		"long name": {FullName: string(make([]rune, 121)), Phone: "3001234567"},
	} {
		if _, err := e.svc.CreateCustomer(ctx, owner, e.biz, in); !errors.As(err, &verr) {
			t.Errorf("%s: %v", name, err)
		}
	}
	carlos, err := e.svc.CreateCustomer(ctx, barber, e.biz, application.CustomerInput{FullName: "  Carlos   Pérez ", Phone: "300 555 0101"})
	if err != nil || carlos.FullName != "Carlos Pérez" || carlos.Phone != "+573005550101" || carlos.UserID != uuid.Nil || carlos.CreatedBy != barber.UserID {
		t.Fatalf("barber creates a customer: %+v %v", carlos, err)
	}
	if _, err := e.svc.CreateCustomer(ctx, owner, e.biz, application.CustomerInput{FullName: "Otro", Phone: "+573005550101"}); !errors.Is(err, application.ErrCustomerPhoneTaken) {
		t.Errorf("same phone: %v", err)
	}
	if _, err := e.svc.CreateCustomer(ctx, outsider, e.biz, application.CustomerInput{FullName: "X", Phone: "3009990000"}); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("outsider: %v", err)
	}
	diana, _ := e.svc.CreateCustomer(ctx, owner, e.biz, application.CustomerInput{FullName: "Diana 100%_", Phone: "3015550202"})

	for q, want := range map[string]int{"": 2, "carlos": 1, "PÉREZ": 1, "300 555": 1, "+57 301": 1, "%": 1, "_": 1, "100%_": 1, "nadie": 0} { // "%" and "_" match literally (Diana), not everything {
		if got, err := e.svc.ListCustomers(ctx, owner, e.biz, q); err != nil || len(got) != want {
			t.Errorf("q=%q: %d %v", q, len(got), err)
		}
	}
	if _, err := e.svc.ListCustomers(ctx, owner, e.biz, string(make([]rune, 61))); !errors.As(err, &verr) {
		t.Errorf("long query: %v", err)
	}

	name := "Carlos P."
	upd, err := e.svc.UpdateCustomer(ctx, owner, e.biz, carlos.ID, application.CustomerPatch{FullName: &name})
	if err != nil || upd.FullName != "Carlos P." || upd.Phone != carlos.Phone {
		t.Errorf("update: %+v %v", upd, err)
	}
	taken := diana.Phone
	if _, err := e.svc.UpdateCustomer(ctx, owner, e.biz, carlos.ID, application.CustomerPatch{Phone: &taken}); !errors.Is(err, application.ErrCustomerPhoneTaken) {
		t.Errorf("update to a taken phone: %v", err)
	}
	if _, err := e.svc.UpdateCustomer(ctx, owner, e.biz, uuid.Must(uuid.NewV7()), application.CustomerPatch{FullName: &name}); !errors.Is(err, application.ErrCustomerNotFound) {
		t.Errorf("unknown customer: %v", err)
	}
	bad := "x"
	if _, err := e.svc.UpdateCustomer(ctx, owner, e.biz, carlos.ID, application.CustomerPatch{Phone: &bad}); !errors.As(err, &verr) {
		t.Errorf("bad phone: %v", err)
	}
}

func TestBookManual(t *testing.T) {
	ctx := context.Background()
	e := newBookingEnv(t)
	owner, juanUser, anaUser := identity.Principal{UserID: uuid.Must(uuid.NewV7())}, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, identity.Principal{UserID: uuid.Must(uuid.NewV7())}
	e.roles[[2]uuid.UUID{e.biz, owner.UserID}] = identity.RoleOwner
	e.roles[[2]uuid.UUID{e.biz, juanUser.UserID}] = identity.RoleProfessional
	e.roles[[2]uuid.UUID{e.biz, anaUser.UserID}] = identity.RoleProfessional
	e.ag.owners = map[uuid.UUID]uuid.UUID{e.pro: juanUser.UserID} // e.pro is Juan's agenda
	carlos, err := e.svc.CreateCustomer(ctx, owner, e.biz, application.CustomerInput{FullName: "Carlos", Phone: "3005550101"})
	if err != nil {
		t.Fatal(err)
	}
	in := func(start time.Time, key string, confirmed bool) application.ManualInput {
		return application.ManualInput{ProfessionalID: e.pro, ServiceID: e.svcID, CustomerID: carlos.ID, Start: start, Confirmed: confirmed, IdempotencyKey: key}
	}
	// "now" is Friday noon in Bogota; the staff books without the 60-min notice the customers need.
	e.clk.Advance(time.Date(2030, 3, 5, 14, 5, 0, 0, time.UTC).Sub(e.clk.Now())) // Tuesday 09:05 Bogota
	a, err := e.svc.BookManual(ctx, owner, e.biz, in(e.day(9, 10), "manual-owner-1", true))
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != domain.StatusConfirmed || a.Customer.CustomerID != carlos.ID || a.Customer.Name != "Carlos" || a.CreatedBy != owner.UserID {
		t.Errorf("manual appointment = %+v", a)
	}
	if again, err := e.svc.BookManual(ctx, owner, e.biz, in(e.day(9, 10), "manual-owner-1", true)); err != nil || again.ID != a.ID {
		t.Errorf("replay: %v %v", again.ID, err)
	}
	// Juan books his own agenda (pending); Ana cannot book Juan's.
	if p, err := e.svc.BookManual(ctx, juanUser, e.biz, in(e.day(10, 0), "manual-juan-1", false)); err != nil || p.Status != domain.StatusPendingConfirmation {
		t.Errorf("barber in his agenda: %+v %v", p, err)
	}
	if _, err := e.svc.BookManual(ctx, anaUser, e.biz, in(e.day(11, 0), "manual-ana-1", true)); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("barber in another agenda: %v", err)
	}
	var verr *application.ValidationError
	for name, c := range map[string]struct {
		in   application.ManualInput
		want any
	}{
		"overlaps":         {in(e.day(9, 20), "manual-owner-2", true), application.ErrSlotUnavailable},
		"in the past":      {in(e.day(9, 0), "manual-owner-3", true), application.ErrSlotUnavailable},
		"unknown customer": {application.ManualInput{ProfessionalID: e.pro, ServiceID: e.svcID, CustomerID: uuid.Must(uuid.NewV7()), Start: e.day(11, 0), IdempotencyKey: "manual-owner-4"}, &verr},
		"missing fields":   {application.ManualInput{IdempotencyKey: "manual-owner-5"}, &verr},
		"bad key":          {in(e.day(11, 0), "x", true), application.ErrInvalidIdempotencyKey},
	} {
		_, err := e.svc.BookManual(ctx, owner, e.biz, c.in)
		if target, ok := c.want.(**application.ValidationError); ok {
			if !errors.As(err, target) {
				t.Errorf("%s: %v", name, err)
			}
		} else if !errors.Is(err, c.want.(error)) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if _, err := e.svc.BookManual(ctx, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, e.biz, in(e.day(11, 0), "manual-intruso", true)); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("outsider: %v", err)
	}
	if len(e.store.Appointments) != 2 {
		t.Errorf("appointments = %d", len(e.store.Appointments))
	}
}

// TestAppBookingJoinsTheCustomerList: an account that books from the app becomes (or is linked
// to) the business's customer with the same mobile.
func TestAppBookingJoinsTheCustomerList(t *testing.T) {
	ctx := context.Background()
	e := newBookingEnv(t)
	owner := identity.Principal{UserID: uuid.Must(uuid.NewV7())}
	e.roles[[2]uuid.UUID{e.biz, owner.UserID}] = identity.RoleOwner
	// Bob was created by the staff with the same mobile as his account.
	bobRec, _ := e.svc.CreateCustomer(ctx, owner, e.biz, application.CustomerInput{FullName: "Bob (mostrador)", Phone: "+573004445566"})
	ana, err := e.svc.Book(ctx, e.ana, e.in(e.day(9, 0), "ana-app-1"))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := e.svc.Book(ctx, e.bob, e.in(e.day(10, 0), "bob-app-1"))
	if err != nil {
		t.Fatal(err)
	}
	list, _ := e.svc.ListCustomers(ctx, owner, e.biz, "")
	if len(list) != 2 || bob.Customer.CustomerID != bobRec.ID || ana.Customer.CustomerID == uuid.Nil {
		t.Fatalf("customers %+v, bob → %v, ana → %v", list, bob.Customer.CustomerID, ana.Customer.CustomerID)
	}
	for _, c := range list {
		if c.UserID == uuid.Nil {
			t.Errorf("%s not linked to the account", c.FullName)
		}
	}
	if _, err := e.svc.Book(ctx, e.ana, e.in(e.day(11, 0), "ana-app-2")); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.svc.ListCustomers(ctx, owner, e.biz, ""); len(list) != 2 {
		t.Errorf("a second booking duplicated the customer: %d", len(list))
	}
}

// TestBookManualQuickWay: the barber types the customer in the booking (name and mobile, email
// optional); an existing customer with that mobile is reused, otherwise created.
func TestBookManualQuickWay(t *testing.T) {
	ctx := context.Background()
	e := newBookingEnv(t)
	juanUser := identity.Principal{UserID: uuid.Must(uuid.NewV7())}
	e.roles[[2]uuid.UUID{e.biz, juanUser.UserID}] = identity.RoleProfessional
	e.ag.owners = map[uuid.UUID]uuid.UUID{e.pro: juanUser.UserID}
	quick := func(start time.Time, key string, c application.CustomerInput) application.ManualInput {
		return application.ManualInput{ProfessionalID: e.pro, ServiceID: e.svcID, Customer: &c, Start: start, Confirmed: true, IdempotencyKey: key}
	}

	// New person, with email.
	a, err := e.svc.BookManual(ctx, juanUser, e.biz, quick(e.day(9, 0), "rapida-1", application.CustomerInput{FullName: " Laura  Gómez ", Phone: "311 555 0303", Email: " Laura@Example.COM "}))
	if err != nil {
		t.Fatal(err)
	}
	laura, ok := e.store.Customers[a.Customer.CustomerID]
	if !ok || laura.FullName != "Laura Gómez" || laura.Phone != "+573115550303" || laura.Email != "laura@example.com" || a.Customer.Name != "Laura Gómez" {
		t.Fatalf("customer created by the booking: %+v / %+v", laura, a.Customer)
	}
	// Same mobile without email, then with: reused, never duplicated; the email is completed
	// only when missing.
	noMail, _ := e.svc.CreateCustomer(ctx, juanUser, e.biz, application.CustomerInput{FullName: "Pedro", Phone: "3125550404"})
	b, err := e.svc.BookManual(ctx, juanUser, e.biz, quick(e.day(10, 0), "rapida-2", application.CustomerInput{FullName: "Pedro Pérez", Phone: "+57 312 555 0404", Email: "pedro@example.com"}))
	if err != nil || b.Customer.CustomerID != noMail.ID {
		t.Fatalf("existing mobile: %+v %v", b.Customer, err)
	}
	if p := e.store.Customers[noMail.ID]; p.Email != "pedro@example.com" || p.FullName != "Pedro" {
		t.Errorf("existing customer after the booking: %+v", p)
	}
	if _, err := e.svc.BookManual(ctx, juanUser, e.biz, quick(e.day(11, 0), "rapida-3", application.CustomerInput{FullName: "Laura", Phone: "3115550303", Email: "otra@example.com"})); err != nil {
		t.Fatal(err)
	}
	if l := e.store.Customers[laura.ID]; l.Email != "laura@example.com" {
		t.Errorf("an existing email was overwritten: %q", l.Email)
	}
	if len(e.store.Customers) != 2 {
		t.Errorf("customers = %d", len(e.store.Customers))
	}

	var verr *application.ValidationError
	both := quick(e.day(11, 30), "rapida-4", application.CustomerInput{FullName: "X", Phone: "3135550505"})
	both.CustomerID = laura.ID
	for name, c := range map[string]struct {
		in    application.ManualInput
		field string
	}{
		"customer and customerId": {both, "customer"},
		"no customer at all":      {application.ManualInput{ProfessionalID: e.pro, ServiceID: e.svcID, Start: e.day(11, 30), IdempotencyKey: "rapida-5"}, "customer"},
		"no mobile":               {quick(e.day(11, 30), "rapida-6", application.CustomerInput{FullName: "Sin celular"}), "customer.phone"},
		"bad email":               {quick(e.day(11, 30), "rapida-7", application.CustomerInput{FullName: "Mal", Phone: "3135550505", Email: "no-es-correo"}), "customer.email"},
		"no name":                 {quick(e.day(11, 30), "rapida-8", application.CustomerInput{Phone: "3135550505"}), "customer.fullName"},
	} {
		_, err := e.svc.BookManual(ctx, juanUser, e.biz, c.in)
		if !errors.As(err, &verr) || verr.Fields[0].Field != c.field {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A failed booking does not leave a customer behind (same transaction).
	if _, err := e.svc.BookManual(ctx, juanUser, e.biz, quick(e.day(9, 10), "rapida-9", application.CustomerInput{FullName: "Tarde", Phone: "3145550606"})); !errors.Is(err, application.ErrSlotUnavailable) {
		t.Fatalf("overlap: %v", err)
	}
	if len(e.store.Customers) != 2 {
		t.Errorf("a failed booking created a customer: %d", len(e.store.Customers))
	}
}
