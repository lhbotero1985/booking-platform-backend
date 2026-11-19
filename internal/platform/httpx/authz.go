package httpx

import (
	"errors"
	"net/http"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// WriteAuthzProblem maps the authorization sentinels to problems and reports whether err was
// one of them. Handlers call it before their own error mapping:
//
//	if httpx.WriteAuthzProblem(w, r, err) { return }
//
// The 404 is byte-for-byte the router's "no such route" problem (no detail), so a resource of
// another business is indistinguishable from one that does not exist.
func WriteAuthzProblem(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, authz.ErrNotFound):
		WriteProblem(w, r, NewProblem(http.StatusNotFound, CodeNotFound, ""))
	case errors.Is(err, authz.ErrForbidden):
		WriteProblem(w, r, NewProblem(http.StatusForbidden, CodeForbidden, "you do not have permission to perform this action"))
	default:
		return false
	}
	return true
}
