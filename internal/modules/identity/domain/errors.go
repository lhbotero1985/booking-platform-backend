package domain

import "errors"

// Domain errors. The application layer maps them to stable API codes; messages are safe to log.
var (
	ErrInvalidEmail      = errors.New("identity: invalid email")
	ErrPasswordTooShort  = errors.New("identity: password too short")
	ErrPasswordTooLong   = errors.New("identity: password too long")
	ErrPasswordTooCommon = errors.New("identity: password too common")
	ErrInvalidFullName   = errors.New("identity: invalid full name")
	ErrInvalidStatus     = errors.New("identity: invalid user status")
	ErrInvalidRole       = errors.New("identity: invalid role")
	ErrEmptyPasswordHash = errors.New("identity: empty password hash")
	ErrUserSuspended     = errors.New("identity: user suspended")
	ErrUserNotVerified   = errors.New("identity: user not verified")
)
