package application

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	bizcontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	catalog "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/contract"
)

// Public search limits (T4.6).
const (
	MaxPublicLimit     = 50
	DefaultPublicLimit = 20
	minQueryLength     = 2
	maxQueryLength     = 60
	// maxScanRounds bounds how many candidate batches one page may examine when many active
	// businesses are not entitled (expired demos); the page then ends early with a cursor.
	maxScanRounds = 5
)

// Search field codes.
const (
	CodeInvalidQuery  = "INVALID_QUERY"
	CodeInvalidCursor = "INVALID_CURSOR"
	CodeInvalidLimit  = "INVALID_LIMIT"
)

// SearchFilter selects candidate businesses for the public search (ACTIVE, one city).
type SearchFilter struct {
	CityID    uuid.UUID
	NameLike  string // LIKE pattern with wildcards escaped; "" = any
	AfterName string // keyset: lower(name) of the last item seen ("" = from the start)
	AfterID   uuid.UUID
	Limit     int
}

// SearchHit is a candidate with its keyset sort key (lower(name) as PostgreSQL computes it).
type SearchHit struct {
	Business *domain.Business
	SortName string
}

// SearchQuery is a public search request.
type SearchQuery struct {
	CityID uuid.UUID
	Text   string
	Cursor string
	Limit  int
}

// SearchPublic lists the businesses customers can book in a city: ACTIVE, entitled (demo
// running or paid, ADR-0018) and with something to book (T5.7: an ACTIVE service performed by a
// bookable professional), ordered by name. next is the cursor of the following page ("" when
// there are no more). A city without such businesses (or unknown) gives an empty list.
func (s *Service) SearchPublic(ctx context.Context, q SearchQuery) (items []*domain.Business, next string, err error) {
	var verr ValidationError
	if q.CityID == uuid.Nil {
		verr.add("cityId", CodeRequired)
	}
	f := SearchFilter{CityID: q.CityID, Limit: q.Limit}
	if text := strings.TrimSpace(q.Text); text != "" {
		if n := utf8.RuneCountInString(text); n < minQueryLength || n > maxQueryLength {
			verr.add("q", CodeInvalidQuery)
		}
		f.NameLike = "%" + escapeLike(text) + "%"
	}
	if q.Cursor != "" {
		var ok bool
		if f.AfterName, f.AfterID, ok = decodeCursor(q.Cursor); !ok {
			verr.add("cursor", CodeInvalidCursor)
		}
	}
	switch {
	case q.Limit == 0:
		f.Limit = DefaultPublicLimit
	case q.Limit < 1 || q.Limit > MaxPublicLimit:
		verr.add("limit", CodeInvalidLimit)
	}
	if err := verr.orNil(); err != nil {
		return nil, "", err
	}

	want := f.Limit
	f.Limit = want + 1   // one extra visible item tells whether there is a next page
	var hits []SearchHit // visible, in order
	var last SearchHit   // last candidate examined
	for round := 0; round < maxScanRounds && len(hits) <= want; round++ {
		var batch []SearchHit
		err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
			batch, err = tx.SearchActive(ctx, f)
			return err
		})
		if err != nil {
			return nil, "", err
		}
		ids := make([]uuid.UUID, len(batch))
		for i, h := range batch {
			ids[i] = h.Business.ID()
		}
		entitled, err := s.entitlements.EntitledAmong(ctx, ids)
		if err != nil {
			return nil, "", err
		}
		offered, err := s.offered(ctx, ids)
		if err != nil {
			return nil, "", err
		}
		for _, h := range batch {
			last = h
			if entitled[h.Business.ID()] && offered(h.Business.ID()) {
				hits = append(hits, h)
				if len(hits) > want {
					break
				}
			}
		}
		if len(batch) < f.Limit && len(hits) <= want {
			return businesses(hits), "", nil // candidates exhausted
		}
		f.AfterName, f.AfterID = last.SortName, last.Business.ID()
	}
	if len(hits) > want {
		hits = hits[:want]
		return businesses(hits), encodeCursor(hits[want-1]), nil
	}
	// Scan budget spent with a partial page: continue after the last candidate examined.
	return businesses(hits), encodeCursor(last), nil
}

func businesses(hits []SearchHit) []*domain.Business {
	out := make([]*domain.Business, len(hits))
	for i, h := range hits {
		out[i] = h.Business
	}
	return out
}

// GetPublic returns a business customers can see, by slug; ErrNotFound if it does not exist,
// is not ACTIVE or is not entitled (no difference is revealed).
func (s *Service) GetPublic(ctx context.Context, rawSlug string) (*domain.Business, error) {
	slug, err := domain.ParseSlug(strings.ToLower(rawSlug))
	if err != nil {
		return nil, ErrNotFound
	}
	var b *domain.Business
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		b, err = tx.BusinessBySlug(ctx, slug)
		return err
	})
	if err != nil {
		return nil, err
	}
	if b.Status() != domain.StatusActive {
		return nil, ErrNotFound
	}
	ok, err := s.entitlements.Entitled(ctx, b.ID())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	return b, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Cursor: base64url of "sort name\x00id". Opaque to clients.
func encodeCursor(h SearchHit) string {
	return base64.RawURLEncoding.EncodeToString([]byte(h.SortName + "\x00" + h.Business.ID().String()))
}

func decodeCursor(c string) (name string, id uuid.UUID, ok bool) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil || len(raw) > 4*domain.MaxNameLength+40 {
		return "", uuid.Nil, false
	}
	name, rawID, found := strings.Cut(string(raw), "\x00")
	if !found || name == "" || !utf8.ValidString(name) {
		return "", uuid.Nil, false
	}
	id, err = uuid.Parse(rawID)
	if err != nil {
		return "", uuid.Nil, false
	}
	return name, id, true
}

// SetOfferings plugs the catalog's Offerings into the public search (composition root only,
// before serving: catalog itself depends on this service). Without it every entitled business
// is listed.
func (s *Service) SetOfferings(o catalog.Offerings) { s.offerings = o }

func (s *Service) offered(ctx context.Context, ids []uuid.UUID) (func(uuid.UUID) bool, error) {
	if s.offerings == nil {
		return func(uuid.UUID) bool { return true }, nil
	}
	m, err := s.offerings.BookableAmong(ctx, ids)
	if err != nil {
		return nil, err
	}
	return func(id uuid.UUID) bool { return m[id] }, nil
}

// PublicBusinessID implements contract.PublicDirectory.
func (s *Service) PublicBusinessID(ctx context.Context, slug string) (uuid.UUID, error) {
	b, err := s.GetPublic(ctx, slug)
	if errors.Is(err, ErrNotFound) {
		return uuid.Nil, bizcontract.ErrBusinessNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	return b.ID(), nil
}

// PublicBookingPolicy implements the businesses contract PublicBooking (T6.2): the booking
// policy and time zone of a business customers can see.
func (s *Service) PublicBookingPolicy(ctx context.Context, slug string) (bizcontract.BookingPolicy, error) {
	b, err := s.GetPublic(ctx, slug)
	if errors.Is(err, ErrNotFound) {
		return bizcontract.BookingPolicy{}, bizcontract.ErrBusinessNotFound
	}
	if err != nil {
		return bizcontract.BookingPolicy{}, err
	}
	return bookingPolicyOf(b), nil
}

// PublicBookingPolicyOf implements the businesses contract PublicBooking by id (T6.3): same
// visibility rules as the public detail (ACTIVE and entitled).
func (s *Service) PublicBookingPolicyOf(ctx context.Context, businessID uuid.UUID) (bizcontract.BookingPolicy, error) {
	var b *domain.Business
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		b, err = tx.BusinessByID(ctx, businessID)
		return err
	})
	if err == nil && b.Status() != domain.StatusActive {
		err = ErrNotFound
	}
	if err == nil {
		var ok bool
		if ok, err = s.entitlements.Entitled(ctx, b.ID()); err == nil && !ok {
			err = ErrNotFound
		}
	}
	if errors.Is(err, ErrNotFound) {
		return bizcontract.BookingPolicy{}, bizcontract.ErrBusinessNotFound
	}
	if err != nil {
		return bizcontract.BookingPolicy{}, err
	}
	return bookingPolicyOf(b), nil
}

// BookingPolicyOf implements the businesses contract PublicBooking for the staff (T6.4): any
// existing business, visible or not.
func (s *Service) BookingPolicyOf(ctx context.Context, businessID uuid.UUID) (bizcontract.BookingPolicy, error) {
	var b *domain.Business
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		b, err = tx.BusinessByID(ctx, businessID)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return bizcontract.BookingPolicy{}, bizcontract.ErrBusinessNotFound
	}
	if err != nil {
		return bizcontract.BookingPolicy{}, err
	}
	return bookingPolicyOf(b), nil
}

// BusinessSummaries implements the businesses contract PublicBooking (T6.5).
func (s *Service) BusinessSummaries(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bizcontract.BusinessSummary, error) {
	out := map[uuid.UUID]bizcontract.BusinessSummary{}
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		for _, id := range ids {
			if _, done := out[id]; done {
				continue
			}
			b, err := tx.BusinessByID(ctx, id)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			sum := bizcontract.BusinessSummary{
				ID: b.ID(), Name: b.Name(), Slug: string(b.Slug()), TimeZone: b.TimeZone(),
				AddressLine: b.Location().AddressLine, ContactPhone: b.ContactPhone(),
			}
			if c := b.Location().Coordinates; c != nil {
				lat, lng := c.Lat, c.Lng
				sum.Latitude, sum.Longitude = &lat, &lng
			}
			out[id] = sum
		}
		return nil
	})
	return out, err
}

func bookingPolicyOf(b *domain.Business) bizcontract.BookingPolicy {
	p, r := b.Policy(), b.Reminders()
	return bizcontract.BookingPolicy{
		BusinessID: b.ID(), TimeZone: b.TimeZone(), SlotIntervalMinutes: p.SlotIntervalMinutes,
		BufferMinutes: p.BufferMinutes, MinLeadMinutes: p.MinLeadMinutes, MaxAdvanceDays: p.MaxAdvanceDays,
		CancellationCutoffHours: p.CancellationCutoffHours, RescheduleCutoffHours: p.RescheduleCutoffHours,
		MaxCustomerReschedules: p.MaxCustomerReschedules, RemindersEnabled: r.Enabled, ReminderHoursBefore: r.HoursBefore,
	}
}
