package application

import (
	"errors"
	"strings"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// Field error codes exposed to clients in Problem.errors[].message (stable, translatable).
const (
	CodeRequired          = "REQUIRED"
	CodeInvalidEmail      = "INVALID_EMAIL"
	CodePasswordTooShort  = "PASSWORD_TOO_SHORT"
	CodePasswordTooLong   = "PASSWORD_TOO_LONG"
	CodePasswordTooCommon = "PASSWORD_TOO_COMMON"
	CodeInvalidFullName   = "INVALID_FULL_NAME"
)

// FieldError is a validation failure on one input field.
type FieldError struct {
	Field string
	Code  string
}

// ValidationError groups every invalid field of a request.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+":"+f.Code)
	}
	return "identity: validation failed (" + strings.Join(parts, ", ") + ")"
}

func (e *ValidationError) add(field, code string) {
	e.Fields = append(e.Fields, FieldError{field, code})
}

func (e *ValidationError) orNil() error {
	if len(e.Fields) == 0 {
		return nil
	}
	return e
}

// passwordCode maps domain password policy errors to field codes.
func passwordCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrPasswordTooShort):
		return CodePasswordTooShort
	case errors.Is(err, domain.ErrPasswordTooLong):
		return CodePasswordTooLong
	case errors.Is(err, domain.ErrPasswordTooCommon):
		return CodePasswordTooCommon
	default:
		return CodeRequired
	}
}
