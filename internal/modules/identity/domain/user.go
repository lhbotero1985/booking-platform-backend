package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// UserStatus is the account lifecycle state.
type UserStatus string

// Account states (matches the users_status_valid CHECK).
const (
	UserPendingVerification UserStatus = "PENDING_VERIFICATION"
	UserActive              UserStatus = "ACTIVE"
	UserSuspended           UserStatus = "SUSPENDED"
)

// ParseUserStatus validates a stored status.
func ParseUserStatus(s string) (UserStatus, error) {
	switch st := UserStatus(s); st {
	case UserPendingVerification, UserActive, UserSuspended:
		return st, nil
	default:
		return "", ErrInvalidStatus
	}
}

// Role is a per-business role granted through a membership (ADR-0007). Customers have no
// membership; platform administrators are flagged on the user.
type Role string

// Business roles (matches the memberships_role_valid CHECK).
const (
	RoleOwner        Role = "OWNER"
	RoleManager      Role = "MANAGER"
	RoleProfessional Role = "PROFESSIONAL"
)

// ParseRole validates a stored role.
func ParseRole(s string) (Role, error) {
	switch r := Role(s); r {
	case RoleOwner, RoleManager, RoleProfessional:
		return r, nil
	default:
		return "", ErrInvalidRole
	}
}

// PasswordHash is an opaque, self-describing hash (PHC string). The domain never inspects it.
type PasswordHash string

// MaxFullNameLength matches the users_full_name_length CHECK.
const MaxFullNameLength = 120

// User is the account aggregate root.
type User struct {
	id              uuid.UUID
	email           Email
	fullName        string
	passwordHash    PasswordHash
	status          UserStatus
	isPlatformAdmin bool
	emailVerifiedAt *time.Time
	createdAt       time.Time
	updatedAt       time.Time
}

// NewUser registers a new account. It starts PENDING_VERIFICATION: the email must be verified
// before the first login (decision of 2026-11-19).
func NewUser(id uuid.UUID, email Email, fullName string, hash PasswordHash, now time.Time) (*User, error) {
	name, err := normalizeFullName(fullName)
	if err != nil {
		return nil, err
	}
	if email == "" {
		return nil, ErrInvalidEmail
	}
	if hash == "" {
		return nil, ErrEmptyPasswordHash
	}
	return &User{
		id: id, email: email, fullName: name, passwordHash: hash,
		status: UserPendingVerification, createdAt: now, updatedAt: now,
	}, nil
}

// RehydrateUser rebuilds a User from storage (no policy checks; storage CHECKs apply).
func RehydrateUser(id uuid.UUID, email Email, fullName string, hash PasswordHash, status UserStatus,
	isPlatformAdmin bool, emailVerifiedAt *time.Time, createdAt, updatedAt time.Time,
) *User {
	return &User{
		id: id, email: email, fullName: fullName, passwordHash: hash, status: status,
		isPlatformAdmin: isPlatformAdmin, emailVerifiedAt: emailVerifiedAt, createdAt: createdAt, updatedAt: updatedAt,
	}
}

// VerifyEmail confirms ownership of the email and activates a pending account. It is
// idempotent for already active accounts and refused for suspended ones.
func (u *User) VerifyEmail(now time.Time) error {
	switch u.status {
	case UserSuspended:
		return ErrUserSuspended
	case UserActive:
		return nil
	case UserPendingVerification:
	}
	u.status = UserActive
	t := now
	u.emailVerifiedAt = &t
	u.updatedAt = now
	return nil
}

// EmailVerifiedAt returns when the email was verified, or nil.
func (u *User) EmailVerifiedAt() *time.Time { return u.emailVerifiedAt }

func normalizeFullName(s string) (string, error) {
	name := strings.Join(strings.Fields(s), " ")
	if name == "" || utf8.RuneCountInString(name) > MaxFullNameLength {
		return "", ErrInvalidFullName
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", ErrInvalidFullName
		}
	}
	return name, nil
}

// CanAuthenticate reports whether the account may log in or use a session.
func (u *User) CanAuthenticate() error {
	switch u.status {
	case UserActive:
		return nil
	case UserSuspended:
		return ErrUserSuspended
	default:
		return ErrUserNotVerified
	}
}

// ChangePasswordHash replaces the stored hash (password change, reset or transparent rehash).
func (u *User) ChangePasswordHash(hash PasswordHash, now time.Time) error {
	if hash == "" {
		return ErrEmptyPasswordHash
	}
	u.passwordHash = hash
	u.updatedAt = now
	return nil
}

// Suspend blocks the account. Callers must also revoke its sessions (security.md §1).
func (u *User) Suspend(now time.Time) {
	u.status = UserSuspended
	u.updatedAt = now
}

// Reactivate lifts a suspension.
func (u *User) Reactivate(now time.Time) {
	u.status = UserActive
	u.updatedAt = now
}

// ID returns the user identifier.
func (u *User) ID() uuid.UUID { return u.id }

// Email returns the normalized email.
func (u *User) Email() Email { return u.email }

// FullName returns the normalized display name.
func (u *User) FullName() string { return u.fullName }

// PasswordHash returns the stored opaque hash.
func (u *User) PasswordHash() PasswordHash { return u.passwordHash }

// Status returns the account state.
func (u *User) Status() UserStatus { return u.status }

// IsPlatformAdmin reports whether the user administers the whole platform.
func (u *User) IsPlatformAdmin() bool { return u.isPlatformAdmin }

// CreatedAt returns the creation instant.
func (u *User) CreatedAt() time.Time { return u.createdAt }

// UpdatedAt returns the last modification instant.
func (u *User) UpdatedAt() time.Time { return u.updatedAt }
