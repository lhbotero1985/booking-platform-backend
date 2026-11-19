package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ClientType identifies the kind of client that owns a session (security.md §1).
type ClientType string

// Client types (matches the sessions_client_type_valid CHECK).
const (
	ClientMobile ClientType = "MOBILE"
	ClientWeb    ClientType = "WEB"
)

// ErrInvalidClientType is returned for unknown client types.
var ErrInvalidClientType = errors.New("identity: invalid client type")

// ParseClientType validates a client type.
func ParseClientType(s string) (ClientType, error) {
	switch c := ClientType(s); c {
	case ClientMobile, ClientWeb:
		return c, nil
	default:
		return "", ErrInvalidClientType
	}
}

// SessionPolicy bounds a session's lifetime.
type SessionPolicy struct {
	Idle     time.Duration // expires after this much inactivity
	Absolute time.Duration // expires this long after creation, regardless of activity
}

// PolicyFor returns the lifetime policy (docs/architecture/security.md §1). Platform
// administrators get the strictest policy on any client.
func PolicyFor(c ClientType, platformAdmin bool) SessionPolicy {
	switch {
	case platformAdmin:
		return SessionPolicy{Idle: time.Hour, Absolute: 12 * time.Hour}
	case c == ClientWeb:
		return SessionPolicy{Idle: 12 * time.Hour, Absolute: 7 * 24 * time.Hour}
	default:
		return SessionPolicy{Idle: 30 * 24 * time.Hour, Absolute: 90 * 24 * time.Hour}
	}
}

// TouchInterval throttles last_seen updates to at most one write per interval.
const TouchInterval = 5 * time.Minute

// Session is an authenticated login. The token that identifies it is never part of the
// aggregate: only its hash is stored by the infrastructure.
type Session struct {
	id                uuid.UUID
	userID            uuid.UUID
	clientType        ClientType
	createdAt         time.Time
	lastSeenAt        time.Time
	idleExpiresAt     time.Time
	absoluteExpiresAt time.Time
}

// NewSession opens a session at now under policy.
func NewSession(id, userID uuid.UUID, c ClientType, policy SessionPolicy, now time.Time) *Session {
	abs := now.Add(policy.Absolute)
	return &Session{
		id: id, userID: userID, clientType: c, createdAt: now, lastSeenAt: now,
		idleExpiresAt: minTime(now.Add(policy.Idle), abs), absoluteExpiresAt: abs,
	}
}

// RehydrateSession rebuilds a stored session.
func RehydrateSession(id, userID uuid.UUID, c ClientType, createdAt, lastSeenAt, idleExpiresAt, absoluteExpiresAt time.Time) *Session {
	return &Session{
		id: id, userID: userID, clientType: c, createdAt: createdAt, lastSeenAt: lastSeenAt,
		idleExpiresAt: idleExpiresAt, absoluteExpiresAt: absoluteExpiresAt,
	}
}

// IsActive reports whether the session is within both expiries at now.
func (s *Session) IsActive(now time.Time) bool {
	return now.Before(s.idleExpiresAt) && now.Before(s.absoluteExpiresAt)
}

// Touch slides the idle expiry (never beyond the absolute one). It returns false when the last
// touch is more recent than TouchInterval, so callers can skip the write.
func (s *Session) Touch(now time.Time, policy SessionPolicy) bool {
	if now.Sub(s.lastSeenAt) < TouchInterval {
		return false
	}
	s.lastSeenAt = now
	s.idleExpiresAt = minTime(now.Add(policy.Idle), s.absoluteExpiresAt)
	return true
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// ID returns the session identifier.
func (s *Session) ID() uuid.UUID { return s.id }

// UserID returns the owner.
func (s *Session) UserID() uuid.UUID { return s.userID }

// ClientType returns the client kind.
func (s *Session) ClientType() ClientType { return s.clientType }

// CreatedAt returns the login instant.
func (s *Session) CreatedAt() time.Time { return s.createdAt }

// LastSeenAt returns the last recorded activity.
func (s *Session) LastSeenAt() time.Time { return s.lastSeenAt }

// IdleExpiresAt returns the inactivity deadline.
func (s *Session) IdleExpiresAt() time.Time { return s.idleExpiresAt }

// AbsoluteExpiresAt returns the hard deadline.
func (s *Session) AbsoluteExpiresAt() time.Time { return s.absoluteExpiresAt }
