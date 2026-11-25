// Package postgres implements the notifications persistence with SQLC (T7.1).
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/sqlcgen"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/worker"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
)

// JobInserter inserts River jobs inside a transaction.
type JobInserter interface {
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Store is the notifications store.
type Store struct {
	pool *pgxpool.Pool
	jobs JobInserter
}

// NewStore builds the store.
func NewStore(pool *pgxpool.Pool, jobs JobInserter) *Store { return &Store{pool: pool, jobs: jobs} }

// WithinTx implements application.Store; it joins the transaction ctx carries.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, tx application.Tx) error) error {
	return db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(db.ContextWithTx(ctx, tx), &txStore{tx: tx, q: sqlcgen.New(tx), jobs: s.jobs})
	})
}

// Message implements application.Store.
func (s *Store) Message(ctx context.Context, id uuid.UUID) (domain.Message, error) {
	r, err := sqlcgen.New(s.pool).MessageByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Message{}, application.ErrMessageNotFound
	}
	if err != nil {
		return domain.Message{}, err
	}
	return toMessage(r), nil
}

// FinishAttempt implements application.Store.
func (s *Store) FinishAttempt(ctx context.Context, m domain.Message, attempted bool) error {
	var n int32
	if attempted {
		n = 1
	}
	var sent *time.Time
	if !m.SentAt.IsZero() {
		sent = &m.SentAt
	}
	return sqlcgen.New(s.pool).FinishMessage(ctx, sqlcgen.FinishMessageParams{
		ID: m.ID, Status: string(m.Status), SkipReason: text(string(m.SkipReason)), ErrorCode: text(m.ErrorCode),
		ProviderMessageID: text(m.ProviderMessageID), SentAt: sent, Attempted: n, UpdatedAt: m.UpdatedAt,
	})
}

// AppointmentMessages implements application.Store.
func (s *Store) AppointmentMessages(ctx context.Context, businessID, appointmentID uuid.UUID) ([]domain.Message, error) {
	rows, err := sqlcgen.New(s.pool).AppointmentMessages(ctx, sqlcgen.AppointmentMessagesParams{BusinessID: businessID, AppointmentID: appointmentID})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, toMessage(r))
	}
	return out, nil
}

type txStore struct {
	tx   pgx.Tx
	q    *sqlcgen.Queries
	jobs JobInserter
}

func (t *txStore) InsertMessage(ctx context.Context, m domain.Message) (bool, error) {
	ids, err := t.q.InsertMessage(ctx, sqlcgen.InsertMessageParams{
		ID: m.ID, BusinessID: m.BusinessID, AppointmentID: m.AppointmentID, EventID: m.EventID, Kind: string(m.Kind),
		Channel: string(m.Channel), RecipientRole: string(m.RecipientRole), Recipient: m.Recipient,
		ProfessionalID: m.ProfessionalID, StartAt: m.Start, CreatedAt: m.CreatedAt,
	})
	return len(ids) == 1, err
}

func (t *txStore) EnqueueSend(ctx context.Context, messageID uuid.UUID) error {
	_, err := t.jobs.InsertTx(ctx, t.tx, worker.SendMessageArgs{MessageID: messageID}, nil)
	return err
}

func toMessage(r sqlcgen.NotificationMessage) domain.Message {
	m := domain.Message{
		ID: r.ID, BusinessID: r.BusinessID, AppointmentID: r.AppointmentID, EventID: r.EventID, Kind: domain.Kind(r.Kind),
		Channel: domain.Channel(r.Channel), RecipientRole: domain.Role(r.RecipientRole), Recipient: r.Recipient,
		ProfessionalID: r.ProfessionalID, Start: r.StartAt.UTC(), Status: domain.Status(r.Status), Attempts: int(r.Attempts),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.SkipReason != nil {
		m.SkipReason = domain.SkipReason(*r.SkipReason)
	}
	if r.ErrorCode != nil {
		m.ErrorCode = *r.ErrorCode
	}
	if r.ProviderMessageID != nil {
		m.ProviderMessageID = *r.ProviderMessageID
	}
	if r.SentAt != nil {
		m.SentAt = r.SentAt.UTC()
	}
	return m
}

func text(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
