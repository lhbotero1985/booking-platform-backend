// Package authz holds the authorization error sentinels shared by every module, so that the
// HTTP layer maps them uniformly (403 FORBIDDEN / 404 NOT_FOUND) without importing modules.
// Policies themselves live in each module's application layer (ADR-0007).
package authz

import "errors"

var (
	// ErrForbidden means the caller can see the resource (e.g. is a member of the business) but its
	// role does not allow the action. Mapped to 403.
	ErrForbidden = errors.New("authz: forbidden")
	// ErrNotFound means the resource does not exist *for this caller* (absent, or belonging to a
	// business the caller is not a member of). Mapped to 404 so existence is never revealed
	// across tenants (anti-IDOR).
	ErrNotFound = errors.New("authz: not found")
)
