// Package postgres implements the businesses application.Store on PostgreSQL (sqlc + pgx).
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/infrastructure/sqlcgen"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
)

// Store implements application.Store. It also answers the geography module's question "does
// this area have active businesses?" (geography application.BusinessUsage), wired in the
// composition root.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore builds the store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// WithinTx implements application.Store.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, tx application.Tx) error) error {
	return db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(db.ContextWithTx(ctx, tx), &txStore{tx: tx, q: sqlcgen.New(tx)})
	})
}

// ActiveBusinessesInCountry counts ACTIVE businesses in a country.
func (s *Store) ActiveBusinessesInCountry(ctx context.Context, countryCode string) (int, error) {
	n, err := sqlcgen.New(s.pool).CountActiveBusinessesInCountry(ctx, countryCode)
	return int(n), err
}

// ActiveBusinessesInDivision counts ACTIVE businesses in an administrative division.
func (s *Store) ActiveBusinessesInDivision(ctx context.Context, divisionID uuid.UUID) (int, error) {
	n, err := sqlcgen.New(s.pool).CountActiveBusinessesInDivision(ctx, divisionID)
	return int(n), err
}

// ActiveBusinessesInCity counts ACTIVE businesses in a city.
func (s *Store) ActiveBusinessesInCity(ctx context.Context, cityID uuid.UUID) (int, error) {
	n, err := sqlcgen.New(s.pool).CountActiveBusinessesInCity(ctx, cityID)
	return int(n), err
}

type txStore struct {
	tx pgx.Tx
	q  *sqlcgen.Queries
}

func (t *txStore) InsertBusiness(ctx context.Context, b *domain.Business) error {
	l, p, r := b.Location(), b.Policy(), b.Reminders()
	lat, lng := coords(l.Coordinates)
	err := t.q.InsertBusiness(ctx, sqlcgen.InsertBusinessParams{
		ID: b.ID(), Name: b.Name(), Slug: string(b.Slug()), Status: string(b.Status()),
		ContactPhone: b.ContactPhone(), Description: b.Description(), CountryCode: l.CountryCode,
		DivisionID: l.DivisionID, CityID: l.CityID, AddressLine: l.AddressLine, Latitude: lat, Longitude: lng,
		TimeZone: b.TimeZone(), Currency: b.Currency(),
		SlotIntervalMinutes: int16(p.SlotIntervalMinutes), BufferMinutes: int16(p.BufferMinutes), //nolint:gosec // validated ranges
		MinLeadMinutes: int32(p.MinLeadMinutes), MaxAdvanceDays: int16(p.MaxAdvanceDays), //nolint:gosec // validated ranges
		CancellationCutoffHours: int16(p.CancellationCutoffHours), RescheduleCutoffHours: int16(p.RescheduleCutoffHours), //nolint:gosec // validated ranges
		RequireCustomerConfirmation: p.RequireCustomerConfirmation, RemindersEnabled: r.Enabled,
		ReminderHoursBefore: int16(r.HoursBefore), CreatedAt: b.CreatedAt(), //nolint:gosec // validated range
		MaxCustomerReschedules: int16(p.MaxCustomerReschedules), //nolint:gosec // validated range
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "businesses_slug_unique" {
		return application.ErrSlugTaken
	}
	return err
}

func (t *txStore) UpdateBusiness(ctx context.Context, b *domain.Business) error {
	l, p, r := b.Location(), b.Policy(), b.Reminders()
	lat, lng := coords(l.Coordinates)
	return t.q.UpdateBusiness(ctx, sqlcgen.UpdateBusinessParams{
		ID: b.ID(), Name: b.Name(), Status: string(b.Status()), ContactPhone: b.ContactPhone(),
		Description: b.Description(), DivisionID: l.DivisionID, CityID: l.CityID, AddressLine: l.AddressLine,
		Latitude: lat, Longitude: lng, TimeZone: b.TimeZone(),
		SlotIntervalMinutes: int16(p.SlotIntervalMinutes), BufferMinutes: int16(p.BufferMinutes), //nolint:gosec // validated ranges
		MinLeadMinutes: int32(p.MinLeadMinutes), MaxAdvanceDays: int16(p.MaxAdvanceDays), //nolint:gosec // validated ranges
		CancellationCutoffHours: int16(p.CancellationCutoffHours), RescheduleCutoffHours: int16(p.RescheduleCutoffHours), //nolint:gosec // validated ranges
		RequireCustomerConfirmation: p.RequireCustomerConfirmation, RemindersEnabled: r.Enabled,
		ReminderHoursBefore: int16(r.HoursBefore), UpdatedAt: b.UpdatedAt(), //nolint:gosec // validated range
		MaxCustomerReschedules: int16(p.MaxCustomerReschedules), //nolint:gosec // validated range
	})
}

func (t *txStore) BusinessByID(ctx context.Context, id uuid.UUID) (*domain.Business, error) {
	r, err := t.q.BusinessByID(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	return toDomain(row(r))
}

func (t *txStore) BusinessBySlug(ctx context.Context, slug domain.Slug) (*domain.Business, error) {
	r, err := t.q.BusinessBySlug(ctx, string(slug))
	if err != nil {
		return nil, notFound(err)
	}
	return toDomain(row(r))
}

func (t *txStore) SlugTaken(ctx context.Context, slug domain.Slug) (bool, error) {
	return t.q.SlugTaken(ctx, string(slug))
}

func (t *txStore) ListBusinesses(ctx context.Context, f application.ListFilter) ([]*domain.Business, error) {
	p := sqlcgen.ListBusinessesParams{MaxRows: int32(f.Limit)} //nolint:gosec // bounded by the caller
	if f.Status != "" {
		s := string(f.Status)
		p.Status = &s
	}
	if f.CityID != uuid.Nil {
		p.CityID = &f.CityID
	}
	if f.After != uuid.Nil {
		p.After = &f.After
	}
	rows, err := t.q.ListBusinesses(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Business, 0, len(rows))
	for _, r := range rows {
		b, err := toDomain(row(r))
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (t *txStore) SearchActive(ctx context.Context, f application.SearchFilter) ([]application.SearchHit, error) {
	p := sqlcgen.SearchActiveBusinessesParams{CityID: f.CityID, MaxRows: int32(f.Limit)} //nolint:gosec // bounded by the caller
	if f.NameLike != "" {
		p.NameLike = &f.NameLike
	}
	if f.AfterName != "" {
		p.AfterName, p.AfterID = &f.AfterName, &f.AfterID
	}
	rows, err := t.q.SearchActiveBusinesses(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]application.SearchHit, 0, len(rows))
	for _, r := range rows {
		b, err := toDomain(sqlcgen.BusinessByIDRow{
			ID: r.ID, Name: r.Name, Slug: r.Slug, Status: r.Status, ContactPhone: r.ContactPhone, Description: r.Description,
			CountryCode: r.CountryCode, DivisionID: r.DivisionID, CityID: r.CityID, AddressLine: r.AddressLine,
			Latitude: r.Latitude, Longitude: r.Longitude, TimeZone: r.TimeZone, Currency: r.Currency,
			SlotIntervalMinutes: r.SlotIntervalMinutes, BufferMinutes: r.BufferMinutes, MinLeadMinutes: r.MinLeadMinutes,
			MaxAdvanceDays: r.MaxAdvanceDays, CancellationCutoffHours: r.CancellationCutoffHours,
			RescheduleCutoffHours: r.RescheduleCutoffHours, RequireCustomerConfirmation: r.RequireCustomerConfirmation,
			RemindersEnabled: r.RemindersEnabled, ReminderHoursBefore: r.ReminderHoursBefore,
			MaxCustomerReschedules: r.MaxCustomerReschedules, LogoKey: r.LogoKey, CoverKey: r.CoverKey,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, application.SearchHit{Business: b, SortName: r.SortName})
	}
	return out, nil
}

func (t *txStore) AppendAudit(ctx context.Context, e *audit.Entry) error {
	return audit.Append(ctx, t.tx, e)
}

func (t *txStore) ClaimCreationKey(ctx context.Context, userID uuid.UUID, key, fingerprint string, now time.Time) (bool, error) {
	rows, err := t.q.ClaimCreationKey(ctx, sqlcgen.ClaimCreationKeyParams{UserID: userID, Key: key, Fingerprint: fingerprint, CreatedAt: now})
	return len(rows) == 1, err
}

func (t *txStore) CreationKeyOf(ctx context.Context, userID uuid.UUID, key string) (string, uuid.UUID, error) {
	r, err := t.q.CreationKeyOf(ctx, sqlcgen.CreationKeyOfParams{UserID: userID, Key: key})
	if err != nil {
		return "", uuid.Nil, notFound(err)
	}
	if r.BusinessID == nil {
		return r.Fingerprint, uuid.Nil, nil
	}
	return r.Fingerprint, *r.BusinessID, nil
}

func (t *txStore) SetCreationKeyBusiness(ctx context.Context, userID uuid.UUID, key string, businessID uuid.UUID) error {
	return t.q.SetCreationKeyBusiness(ctx, sqlcgen.SetCreationKeyBusinessParams{UserID: userID, Key: key, BusinessID: &businessID})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ref(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// LockImages implements application.Tx: the business row, locked, with its image keys.
func (t *txStore) LockImages(ctx context.Context, id uuid.UUID) (domain.Images, error) {
	r, err := t.q.LockBusinessImages(ctx, id)
	if err != nil {
		return domain.Images{}, notFound(err)
	}
	return domain.Images{Logo: deref(r.LogoKey), Cover: deref(r.CoverKey)}, nil
}

// SetImages implements application.Tx.
func (t *txStore) SetImages(ctx context.Context, id uuid.UUID, img domain.Images, now time.Time) error {
	if err := t.q.SetBusinessLogo(ctx, sqlcgen.SetBusinessLogoParams{ID: id, LogoKey: ref(img.Logo), UpdatedAt: now}); err != nil {
		return err
	}
	return t.q.SetBusinessCover(ctx, sqlcgen.SetBusinessCoverParams{ID: id, CoverKey: ref(img.Cover), UpdatedAt: now})
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	}
	return err
}

func coords(c *domain.Coordinates) (lat, lng *float64) {
	if c == nil {
		return nil, nil
	}
	return &c.Lat, &c.Lng
}

// row converts both generated row types (identical columns) to one shape.
func row[T sqlcgen.BusinessByIDRow | sqlcgen.BusinessBySlugRow | sqlcgen.ListBusinessesRow](r T) sqlcgen.BusinessByIDRow {
	return sqlcgen.BusinessByIDRow(r)
}

func toDomain(r sqlcgen.BusinessByIDRow) (*domain.Business, error) {
	status, err := domain.ParseStatus(r.Status)
	if err != nil {
		return nil, fmt.Errorf("business %s: %w", r.ID, err)
	}
	var c *domain.Coordinates
	if r.Latitude != nil && r.Longitude != nil {
		c = &domain.Coordinates{Lat: *r.Latitude, Lng: *r.Longitude}
	}
	b, err := domain.RehydrateBusiness(domain.NewBusinessParams{
		ID: r.ID, Name: r.Name, Slug: domain.Slug(r.Slug), Status: status, ContactPhone: r.ContactPhone,
		Description: r.Description,
		Location: domain.Location{
			CountryCode: r.CountryCode, DivisionID: r.DivisionID, CityID: r.CityID,
			AddressLine: r.AddressLine, Coordinates: c,
		},
		TimeZone: r.TimeZone, Currency: r.Currency,
		Policy: domain.BookingPolicy{
			SlotIntervalMinutes: int(r.SlotIntervalMinutes), BufferMinutes: int(r.BufferMinutes),
			MinLeadMinutes: int(r.MinLeadMinutes), MaxAdvanceDays: int(r.MaxAdvanceDays),
			CancellationCutoffHours: int(r.CancellationCutoffHours), RescheduleCutoffHours: int(r.RescheduleCutoffHours),
			RequireCustomerConfirmation: r.RequireCustomerConfirmation, MaxCustomerReschedules: int(r.MaxCustomerReschedules),
		},
		Reminders: domain.Reminders{Enabled: r.RemindersEnabled, HoursBefore: int(r.ReminderHoursBefore)},
		Images:    domain.Images{Logo: deref(r.LogoKey), Cover: deref(r.CoverKey)},
		Now:       r.CreatedAt,
	}, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("business %s: stored row is invalid: %w", r.ID, err)
	}
	return b, nil
}
