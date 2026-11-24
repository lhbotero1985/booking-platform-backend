// Package apptest provides in-memory test doubles for the identity application ports. It is
// used by use-case, handler and contract tests; production code must not import it.
package apptest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// Store is an in-memory application.Store. Transactions are serialized and rolled back on
// error by restoring a snapshot.
type Store struct {
	mu       sync.Mutex
	users    map[uuid.UUID]*domain.User
	tokens   map[string]*evToken
	sessions map[string]*sessRec
	resets   map[string]*evToken
	// memberships holds ACTIVE memberships per user (revoked ones are removed).
	memberships map[uuid.UUID][]contract.Membership
	Enqueued    []uuid.UUID // verification emails scheduled (after commit)
	// ResetEnqueued and ChangedEnqueued record password reset / password changed emails.
	ResetEnqueued   []uuid.UUID
	ChangedEnqueued []uuid.UUID
	FailNext        error // if set, the next transaction fails with it
	// Audit holds committed audit entries; FailAudit makes AppendAudit fail.
	Audit     []application.AuditEntry
	FailAudit error
}

type evToken struct {
	userID    uuid.UUID
	expiresAt time.Time
	usedAt    *time.Time
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{users: map[uuid.UUID]*domain.User{}, tokens: map[string]*evToken{}, sessions: map[string]*sessRec{}, resets: map[string]*evToken{}, memberships: map[uuid.UUID][]contract.Membership{}}
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
	snapSessions := make(map[string]*sessRec, len(s.sessions))
	for k, v := range s.sessions {
		cp := *v
		snapSessions[k] = &cp
	}
	snapResets := make(map[string]*evToken, len(s.resets))
	for k, v := range s.resets {
		cp := *v
		snapResets[k] = &cp
	}
	tx := &storeTx{s: s}
	if err := fn(tx); err != nil {
		s.users, s.tokens, s.sessions, s.resets = snapUsers, snapTokens, snapSessions, snapResets
		return err
	}
	s.Enqueued = append(s.Enqueued, tx.enqueued...)
	s.ResetEnqueued = append(s.ResetEnqueued, tx.resetEnqueued...)
	s.ChangedEnqueued = append(s.ChangedEnqueued, tx.changedEnqueued...)
	s.Audit = append(s.Audit, tx.audit...)
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
	audit           []application.AuditEntry
	s               *Store
	enqueued        []uuid.UUID
	resetEnqueued   []uuid.UUID
	changedEnqueued []uuid.UUID
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

type sessRec struct {
	session   domain.Session
	revokedAt *time.Time
	userAgent string
}

// SessionCount returns the number of non-revoked sessions of userID.
func (s *Store) SessionCount(userID uuid.UUID) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.sessions {
		if r.session.UserID() == userID && r.revokedAt == nil {
			n++
		}
	}
	return n
}

// SessionHashes returns every stored session token hash (hex).
func (s *Store) SessionHashes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.sessions))
	for k := range s.sessions {
		out = append(out, k)
	}
	return out
}

// SetPlatformAdmin flags a user as platform administrator (test helper).
func (s *Store) SetPlatformAdmin(email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, u := range s.users {
		if u.Email().String() == email {
			s.users[id] = domain.RehydrateUser(u.ID(), u.Email(), u.FullName(), u.PasswordHash(), u.Status(), true, u.EmailVerifiedAt(), u.CreatedAt(), u.UpdatedAt())
		}
	}
}

func (t *storeTx) UpdatePasswordHash(_ context.Context, id uuid.UUID, hash domain.PasswordHash) error {
	u, ok := t.s.users[id]
	if !ok {
		return application.ErrNotFound
	}
	return u.ChangePasswordHash(hash, u.UpdatedAt())
}

func (t *storeTx) InsertMembership(_ context.Context, _, userID, businessID uuid.UUID, role contract.Role, _ time.Time) error {
	for _, m := range t.s.memberships[userID] {
		if m.BusinessID == businessID {
			return contract.ErrAlreadyMember
		}
	}
	t.s.memberships[userID] = append(t.s.memberships[userID], contract.Membership{BusinessID: businessID, Role: role})
	return nil
}

func (t *storeTx) ActiveMembers(_ context.Context, businessID uuid.UUID) ([]contract.Member, error) {
	order := map[contract.Role]int{contract.RoleOwner: 0, contract.RoleProfessional: 1}
	var out []contract.Member
	for userID, ms := range t.s.memberships {
		for _, m := range ms {
			if m.BusinessID == businessID {
				u := t.s.users[userID]
				member := contract.Member{UserID: userID, Role: m.Role}
				if u != nil {
					member.FullName, member.Email = u.FullName(), u.Email().String()
				}
				out = append(out, member)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if order[out[i].Role] != order[out[j].Role] {
			return order[out[i].Role] < order[out[j].Role]
		}
		return out[i].Email < out[j].Email
	})
	return out, nil
}

func (t *storeTx) ActiveMembersAmong(_ context.Context, businessID uuid.UUID, userIDs []uuid.UUID) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for _, id := range userIDs {
		for _, m := range t.s.memberships[id] {
			if m.BusinessID == businessID {
				out = append(out, id)
			}
		}
	}
	return out, nil
}

func (t *storeTx) ActiveRole(_ context.Context, businessID, userID uuid.UUID) (contract.Role, error) {
	for _, m := range t.s.memberships[userID] {
		if m.BusinessID == businessID {
			return m.Role, nil
		}
	}
	return "", contract.ErrMemberNotFound
}

func (t *storeTx) LockActiveOwners(_ context.Context, businessID uuid.UUID) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for userID, ms := range t.s.memberships {
		for _, m := range ms {
			if m.BusinessID == businessID && m.Role == contract.RoleOwner {
				out = append(out, userID)
			}
		}
	}
	return out, nil
}

func (t *storeTx) UpdateMembershipRole(_ context.Context, businessID, userID uuid.UUID, role contract.Role, _ time.Time) error {
	for i, m := range t.s.memberships[userID] {
		if m.BusinessID == businessID {
			t.s.memberships[userID][i].Role = role
		}
	}
	return nil
}

func (t *storeTx) RevokeMembership(_ context.Context, businessID, userID uuid.UUID, _ time.Time) error {
	kept := t.s.memberships[userID][:0]
	for _, m := range t.s.memberships[userID] {
		if m.BusinessID != businessID {
			kept = append(kept, m)
		}
	}
	t.s.memberships[userID] = kept
	return nil
}

func (t *storeTx) LockUser(_ context.Context, id uuid.UUID) error {
	if _, ok := t.s.users[id]; !ok {
		return application.ErrNotFound
	}
	return nil
}

func (t *storeTx) SetPlatformAdmin(_ context.Context, id uuid.UUID, admin bool) error {
	u, ok := t.s.users[id]
	if !ok {
		return application.ErrNotFound
	}
	t.s.users[id] = domain.RehydrateUser(u.ID(), u.Email(), u.FullName(), u.PasswordHash(), u.Status(), admin, u.EmailVerifiedAt(), u.CreatedAt(), u.UpdatedAt())
	return nil
}

func (t *storeTx) CreateSession(_ context.Context, sess *domain.Session, hash []byte, _ *netip.Addr, ua string) error {
	t.s.sessions[hex.EncodeToString(hash)] = &sessRec{session: *sess, userAgent: ua}
	return nil
}

func (t *storeTx) ActiveSessionByTokenHash(_ context.Context, hash []byte, now time.Time) (*domain.Session, *domain.User, error) {
	r, ok := t.s.sessions[hex.EncodeToString(hash)]
	if !ok || r.revokedAt != nil || !r.session.IsActive(now) {
		return nil, nil, application.ErrNotFound
	}
	u, ok := t.s.users[r.session.UserID()]
	if !ok {
		return nil, nil, application.ErrNotFound
	}
	sess, user := r.session, *u
	return &sess, &user, nil
}

func (t *storeTx) TouchSession(_ context.Context, id uuid.UUID, now, idle time.Time) error {
	for _, r := range t.s.sessions {
		if r.session.ID() == id && r.revokedAt == nil {
			s := r.session
			r.session = *domain.RehydrateSession(s.ID(), s.UserID(), s.ClientType(), s.CreatedAt(), now, idle, s.AbsoluteExpiresAt())
		}
	}
	return nil
}

func (t *storeTx) RevokeSession(_ context.Context, id, userID uuid.UUID, now time.Time) error {
	for _, r := range t.s.sessions {
		if r.session.ID() == id && r.session.UserID() == userID && r.revokedAt == nil {
			n := now
			r.revokedAt = &n
		}
	}
	return nil
}

func (t *storeTx) RevokeAllUserSessions(_ context.Context, userID uuid.UUID, now time.Time) (int64, error) {
	var n int64
	for _, r := range t.s.sessions {
		if r.session.UserID() == userID && r.revokedAt == nil {
			ts := now
			r.revokedAt = &ts
			n++
		}
	}
	return n, nil
}

// ActiveResetTokenCount returns unused password reset tokens for userID.
func (s *Store) ActiveResetTokenCount(userID uuid.UUID) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, t := range s.resets {
		if t.userID == userID && t.usedAt == nil {
			n++
		}
	}
	return n
}

func (t *storeTx) RevokeOtherUserSessions(_ context.Context, userID, keep uuid.UUID, now time.Time) (int64, error) {
	var n int64
	for _, r := range t.s.sessions {
		if r.session.UserID() == userID && r.session.ID() != keep && r.revokedAt == nil {
			ts := now
			r.revokedAt = &ts
			n++
		}
	}
	return n, nil
}

func (t *storeTx) InvalidatePasswordResetTokens(_ context.Context, id uuid.UUID, now time.Time) error {
	for _, tok := range t.s.resets {
		if tok.userID == id && tok.usedAt == nil {
			n := now
			tok.usedAt = &n
		}
	}
	return nil
}

func (t *storeTx) CreatePasswordResetToken(_ context.Context, hash []byte, id uuid.UUID, _, expiresAt time.Time) error {
	t.s.resets[hex.EncodeToString(hash)] = &evToken{userID: id, expiresAt: expiresAt}
	return nil
}

func (t *storeTx) ConsumePasswordResetToken(_ context.Context, hash []byte, now time.Time) (uuid.UUID, error) {
	tok, ok := t.s.resets[hex.EncodeToString(hash)]
	if !ok || tok.usedAt != nil || !tok.expiresAt.After(now) {
		return uuid.Nil, application.ErrNotFound
	}
	n := now
	tok.usedAt = &n
	return tok.userID, nil
}

func (t *storeTx) EnqueuePasswordResetEmail(_ context.Context, id uuid.UUID) error {
	t.resetEnqueued = append(t.resetEnqueued, id)
	return nil
}

func (t *storeTx) EnqueuePasswordChangedEmail(_ context.Context, id uuid.UUID) error {
	t.changedEnqueued = append(t.changedEnqueued, id)
	return nil
}

// GrantMembership gives userID an active role in businessID (test helper).
func (s *Store) GrantMembership(userID, businessID uuid.UUID, role contract.Role) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memberships[userID] = append(s.memberships[userID], contract.Membership{BusinessID: businessID, Role: role})
}

// RevokeMembership removes userID's membership in businessID (test helper).
func (s *Store) RevokeMembership(userID, businessID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.memberships[userID][:0]
	for _, m := range s.memberships[userID] {
		if m.BusinessID != businessID {
			kept = append(kept, m)
		}
	}
	s.memberships[userID] = kept
}

func (t *storeTx) ActiveMembershipsByUser(_ context.Context, userID uuid.UUID) ([]contract.Membership, error) {
	return append([]contract.Membership{}, t.s.memberships[userID]...), nil
}

func (t *storeTx) AppendAudit(_ context.Context, e application.AuditEntry) error {
	if t.s.FailAudit != nil {
		return t.s.FailAudit
	}
	t.audit = append(t.audit, e)
	return nil
}

// AuditActions returns "action:OUTCOME" for every committed entry, in order (test helper).
func (s *Store) AuditActions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.Audit))
	for i, e := range s.Audit {
		out[i] = e.Action + ":" + string(e.Outcome)
	}
	return out
}

// LastAudit returns the last committed entry (test helper).
func (s *Store) LastAudit() application.AuditEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Audit) == 0 {
		return application.AuditEntry{}
	}
	return s.Audit[len(s.Audit)-1]
}
