package contract

import (
	"context"

	"github.com/google/uuid"
)

// Accounts reads what other modules may know about an account to write to it (T7.6: the
// appointment emails of a professional or a customer with an account).
type Accounts interface {
	// AccountEmail returns the email of an account that can sign in, or "" when the account does
	// not exist or is disabled. No authorization (background jobs).
	AccountEmail(ctx context.Context, userID uuid.UUID) (string, error)
}
