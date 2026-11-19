// Package apptest provides in-memory test doubles for the identity application ports. It is
// used by use-case, handler and contract tests; production code must not import it.
package apptest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// Store is an in-memory application.Store. Transactions are serialized and rolled back on
// error by restoring a snapshot.
type Store struct {
	mu       sync.Mutex
	users    map[uuid.UUID]*domain.User
	tokens   map[string]*evToken
	Enqueued []uuid.UUID // verification emails scheduled (after commit)
	FailNext error       // if set, the next transaction fails with it
}

type evToken struct {
	userID    uuid.UUID
	expiresAt time.Time
	usedAt    *time.Time
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{users: map[uuid.UUID]*domain.User{}, tokens: map[string]*evToken{}}
}

// WithinTx implements application.Store.
func (s *Store) WithinTx(_ context.Context, fn func(tx application.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.FailNext; err != nil {
		s.FailNext = nil
		return err
	}
	snapUsers := make(map[uuid.UUID]*domain.User, len(s.users))
	for k, v := range s.users {
		cp := *v
		snapUsers[k] = &cp
	}
	snapTokens := make(map[string]*evToken, len(s.tokens))
	for k, v := range s.tokens {
		cp := *v
		snapTokens[k] = &cp
	}
	tx := &storeTx{s: s}
	if err := fn(tx); err != nil {
		s.users, s.tokens = snapUsers, snapTokens
		return err
	}
	s.Enqueued = append(s.Enqueued, tx.enqueued...)
	return nil
}

// User returns a stored user (test helper).
func (s *Store) User(email string) *domain.User {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Email().String() == email {
			return u
		}
	}
	return nil
}

// UserCount returns the number of stored users.
func (s *Store) UserCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users)
}

// ActiveTokenCount returns unused verification tokens for userID.
func (s *Store) ActiveTokenCount(userID uuid.UUID) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, t := range s.tokens {
		if t.userID == userID && t.usedAt == nil {
			n++
		}
	}
	return n
}

// StoredTokenHashes returns every stored token hash (hex).
func (s *Store) StoredTokenHashes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.tokens))
	for k := range s.tokens {
		out = append(out, k)
	}
	return out
}

type storeTx struct {
	s        *Store
	enqueued []uuid.UUID
}

func (t *storeTx) CreateUser(_ context.Context, u *domain.User) error {
	for _, x := range t.s.users {
		if x.Email() == u.Email() {
			return application.ErrEmailTaken
		}
	}
	cp := *u
	t.s.users[u.ID()] = &cp
	return nil
}

func (t *storeTx) UserByEmail(_ context.Context, email domain.Email) (*domain.User, error) {
	for _, u := range t.s.users {
		if u.Email() == email {
			cp := *u
			return &cp, nil
		}
	}
	return nil, application.ErrNotFound
}

func (t *storeTx) UserByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	u, ok := t.s.users[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (t *storeTx) MarkEmailVerified(_ context.Context, id uuid.UUID, now time.Time) error {
	if u, ok := t.s.users[id]; ok {
		return u.VerifyEmail(now)
	}
	return application.ErrNotFound
}

func (t *storeTx) InvalidateEmailVerificationTokens(_ context.Context, id uuid.UUID, now time.Time) error {
	for _, tok := range t.s.tokens {
		if tok.userID == id && tok.usedAt == nil {
			n := now
			tok.usedAt = &n
		}
	}
	return nil
}

func (t *storeTx) CreateEmailVerificationToken(_ context.Context, hash []byte, id uuid.UUID, _, expiresAt time.Time) error {
	t.s.tokens[hex.EncodeToString(hash)] = &evToken{userID: id, expiresAt: expiresAt}
	return nil
}

func (t *storeTx) ConsumeEmailVerificationToken(_ context.Context, hash []byte, now time.Time) (uuid.UUID, error) {
	tok, ok := t.s.tokens[hex.EncodeToString(hash)]
	if !ok || tok.usedAt != nil || !tok.expiresAt.After(now) {
		return uuid.Nil, application.ErrNotFound
	}
	n := now
	tok.usedAt = &n
	return tok.userID, nil
}

func (t *storeTx) EnqueueVerificationEmail(_ context.Context, id uuid.UUID) error {
	t.enqueued = append(t.enqueued, id)
	return nil
}

// Hasher is a fast, deterministic PasswordHasher for tests (NOT secure).
type Hasher struct {
	mu          sync.Mutex
	HashCalls   int
	VerifyCalls int
	DummyCalls  int
}

// Hash implements application.PasswordHasher.
func (h *Hasher) Hash(_ context.Context, p domain.Password) (domain.PasswordHash, error) {
	h.mu.Lock()
	h.HashCalls++
	h.mu.Unlock()
	sum := sha256.Sum256(p.Reveal())
	return domain.PasswordHash("$argon2id$test$" + hex.EncodeToString(sum[:])), nil
}

// Verify implements application.PasswordHasher.
func (h *Hasher) Verify(ctx context.Context, p domain.Password, hash domain.PasswordHash) (bool, bool, error) {
	h.mu.Lock()
	h.VerifyCalls++
	h.mu.Unlock()
	want, _ := h.Hash(ctx, p)
	return want == hash, false, nil
}

// VerifyDummy implements application.PasswordHasher.
func (h *Hasher) VerifyDummy(context.Context, domain.Password) {
	h.mu.Lock()
	h.DummyCalls++
	h.mu.Unlock()
}

// Mail is a captured email.
type Mail struct{ To, Subject, Body string }

// Mailer records sent emails.
type Mailer struct {
	mu   sync.Mutex
	Sent []Mail
	Err  error
}

// Send implements application.Mailer.
func (m *Mailer) Send(_ context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.Sent = append(m.Sent, Mail{to, subject, body})
	return nil
}

// Last returns the last email sent, or zero.
func (m *Mailer) Last() Mail {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Sent) == 0 {
		return Mail{}
	}
	return m.Sent[len(m.Sent)-1]
}

// FixedRandom returns deterministic entropy that changes on every read (counter-based).
type FixedRandom struct {
	mu sync.Mutex
	n  byte
}

func (r *FixedRandom) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	copy(p, bytes.Repeat([]byte{r.n}, len(p)))
	return len(p), nil
}
