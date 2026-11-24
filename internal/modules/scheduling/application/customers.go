package application

// Customers of a business (T6.4): the staff (OWNER and PROFESSIONAL) creates and edits them; a
// customer who books from the app is added or linked by its mobile number.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

// Customer errors and field codes.
var (
	// ErrCustomerNotFound means there is no such customer in the business.
	ErrCustomerNotFound = errors.New("scheduling: customer not found")
	// ErrCustomerPhoneTaken means another customer of the business has that mobile number.
	ErrCustomerPhoneTaken = errors.New("scheduling: customer phone taken")
)

// Field codes of ValidationError.
const (
	CodeRequired            = "REQUIRED"
	CodeInvalidName         = "INVALID_NAME"
	CodeInvalidPhone        = "INVALID_PHONE"
	CodeInvalidEmail        = "INVALID_EMAIL"
	CodeCustomerNotFound    = "CUSTOMER_NOT_FOUND"
	CodeProfessionalInvalid = "PROFESSIONAL_NOT_BOOKABLE"
)

// MaxListedCustomers bounds a customer listing.
const MaxListedCustomers = 50

// FieldError is one invalid field.
type FieldError struct{ Field, Code string }

// ValidationError lists the invalid fields of a request.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return "scheduling: invalid input" }

func (e *ValidationError) add(field, code string) {
	e.Fields = append(e.Fields, FieldError{field, code})
}

func (e *ValidationError) orNil() error {
	if len(e.Fields) == 0 {
		return nil
	}
	return e
}

// CustomerTx is the customer persistence available inside a transaction.
type CustomerTx interface {
	// InsertCustomer and UpdateCustomer return ErrCustomerPhoneTaken for a duplicate mobile.
	InsertCustomer(ctx context.Context, c domain.BusinessCustomer) error
	UpdateCustomer(ctx context.Context, c domain.BusinessCustomer) error
	// CustomerByID and CustomerByPhone return ErrCustomerNotFound when absent in the business.
	CustomerByID(ctx context.Context, businessID, id uuid.UUID) (domain.BusinessCustomer, error)
	CustomerByPhone(ctx context.Context, businessID uuid.UUID, phone string) (domain.BusinessCustomer, error)
	// ListCustomers filters by name or phone containing q ("" = all; LIKE wildcards escaped by
	// the caller), by name.
	ListCustomers(ctx context.Context, businessID uuid.UUID, q string, limit int) ([]domain.BusinessCustomer, error)
}

// CustomerInput creates a customer: name and mobile mandatory, email optional.
type CustomerInput struct{ FullName, Phone, Email string }

// CustomerPatch changes the given fields (nil = unchanged; an empty email clears it).
type CustomerPatch struct{ FullName, Phone, Email *string }

// ListCustomers returns the customers of the business matching q (name or phone), to its staff.
func (s *Service) ListCustomers(ctx context.Context, p identity.Principal, businessID uuid.UUID, q string) ([]domain.BusinessCustomer, error) {
	if _, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return nil, err
	}
	q = strings.TrimSpace(q)
	if len([]rune(q)) > 60 {
		return nil, &ValidationError{Fields: []FieldError{{"q", "TOO_LONG"}}}
	}
	if compact := strings.Map(dropPhoneSeparators, q); compact != "" && strings.IndexFunc(compact, notDigit) < 0 {
		q = compact // a phone search ignores spaces, dashes, dots, parentheses and the +
	}
	var out []domain.BusinessCustomer
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		out, err = tx.ListCustomers(ctx, businessID, escapeLike(q), MaxListedCustomers)
		return err
	})
	return out, err
}

func dropPhoneSeparators(r rune) rune {
	switch r {
	case ' ', '-', '.', '(', ')', '+':
		return -1
	}
	return r
}

func notDigit(r rune) bool { return r < '0' || r > '9' }

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// CreateCustomer adds a customer to the business (staff). ErrCustomerPhoneTaken if the mobile is
// already a customer of the business.
func (s *Service) CreateCustomer(ctx context.Context, p identity.Principal, businessID uuid.UUID, in CustomerInput) (domain.BusinessCustomer, error) {
	if _, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return domain.BusinessCustomer{}, err
	}
	c, err := s.newCustomer(businessID, p, in, "")
	if err != nil {
		return domain.BusinessCustomer{}, err
	}
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error { return tx.InsertCustomer(ctx, c) })
	if err != nil {
		return domain.BusinessCustomer{}, err
	}
	return c, nil
}

// UpdateCustomer changes a customer of the business (staff).
func (s *Service) UpdateCustomer(ctx context.Context, p identity.Principal, businessID, id uuid.UUID, patch CustomerPatch) (domain.BusinessCustomer, error) {
	if _, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return domain.BusinessCustomer{}, err
	}
	verr := &ValidationError{}
	var name, phone, email string
	if patch.FullName != nil {
		name = parseName(verr, "fullName", *patch.FullName)
	}
	if patch.Phone != nil {
		phone = parsePhone(verr, "phone", *patch.Phone)
	}
	if patch.Email != nil {
		email = parseEmail(verr, "email", *patch.Email)
	}
	if err := verr.orNil(); err != nil {
		return domain.BusinessCustomer{}, err
	}
	var out domain.BusinessCustomer
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		c, err := tx.CustomerByID(ctx, businessID, id)
		if err != nil {
			return err
		}
		if patch.FullName != nil {
			c.FullName = name
		}
		if patch.Phone != nil {
			c.Phone = phone
		}
		if patch.Email != nil {
			c.Email = email
		}
		c.UpdatedAt = s.d.Clock.Now()
		out = c
		return tx.UpdateCustomer(ctx, c)
	})
	return out, err
}

// newCustomer validates a customer input; prefix locates the fields (e.g. "customer.").
func (s *Service) newCustomer(businessID uuid.UUID, p identity.Principal, in CustomerInput, prefix string) (domain.BusinessCustomer, error) {
	verr := &ValidationError{}
	name := parseName(verr, prefix+"fullName", in.FullName)
	phone := parsePhone(verr, prefix+"phone", in.Phone)
	email := parseEmail(verr, prefix+"email", in.Email)
	if err := verr.orNil(); err != nil {
		return domain.BusinessCustomer{}, err
	}
	now := s.d.Clock.Now()
	return domain.BusinessCustomer{
		ID: uuid.Must(uuid.NewV7()), BusinessID: businessID, FullName: name, Phone: phone, Email: email,
		CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func parseName(verr *ValidationError, field, raw string) string {
	if strings.TrimSpace(raw) == "" {
		verr.add(field, CodeRequired)
		return ""
	}
	name, err := domain.ParseCustomerName(raw)
	if err != nil {
		verr.add(field, CodeInvalidName)
	}
	return name
}

func parsePhone(verr *ValidationError, field, raw string) string {
	if strings.TrimSpace(raw) == "" {
		verr.add(field, CodeRequired)
		return ""
	}
	phone, err := domain.ParsePhone(raw)
	if err != nil {
		verr.add(field, CodeInvalidPhone)
	}
	return phone
}

func parseEmail(verr *ValidationError, field, raw string) string {
	email, err := domain.ParseEmail(raw)
	if err != nil {
		verr.add(field, CodeInvalidEmail)
	}
	return email
}

// customerForBooking returns the business's record of an account that books from the app,
// creating it, or linking an existing record with the same mobile and no account yet. If that
// mobile already belongs to another account in the business, the appointment keeps no record.
func customerForBooking(ctx context.Context, tx Tx, businessID uuid.UUID, c identity.Customer, now time.Time) (uuid.UUID, error) {
	existing, err := tx.CustomerByPhone(ctx, businessID, c.Phone)
	switch {
	case errors.Is(err, ErrCustomerNotFound):
		rec := domain.BusinessCustomer{
			ID: uuid.Must(uuid.NewV7()), BusinessID: businessID, FullName: c.FullName, Phone: c.Phone,
			Email: strings.ToLower(c.Email), UserID: c.UserID, CreatedBy: c.UserID, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.InsertCustomer(ctx, rec); err != nil {
			return uuid.Nil, err
		}
		return rec.ID, nil
	case err != nil:
		return uuid.Nil, err
	case existing.UserID == c.UserID:
		return existing.ID, nil
	case existing.UserID == uuid.Nil:
		existing.UserID, existing.UpdatedAt = c.UserID, now
		if existing.Email == "" {
			existing.Email = strings.ToLower(c.Email)
		}
		return existing.ID, tx.UpdateCustomer(ctx, existing)
	default:
		return uuid.Nil, nil
	}
}
