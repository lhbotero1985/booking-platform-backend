// Package httpx contains HTTP plumbing shared by every module: RFC 9457 problem responses,
// JSON helpers, middleware and the router. It holds no business logic.
package httpx

import (
	"encoding/json"
	"net/http"
)

// ContentTypeProblem is the RFC 9457 media type.
const ContentTypeProblem = "application/problem+json"

// Stable machine-readable error codes for clients.
const (
	CodeNotFound         = "NOT_FOUND"
	CodeForbidden        = "FORBIDDEN"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeBodyTooLarge     = "BODY_TOO_LARGE"
	CodeInvalidRequest   = "INVALID_REQUEST"
	CodeForbiddenOrigin  = "FORBIDDEN_ORIGIN"
	CodeUnsupportedMedia = "UNSUPPORTED_MEDIA_TYPE"
	CodeInternal         = "INTERNAL_ERROR"
)

// FieldError describes a validation problem with one input field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Problem is an RFC 9457 problem details object with project extensions.
type Problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	Code      string       `json:"code"`
	RequestID string       `json:"requestId,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
	// Conflicts lists the records that block the request (e.g. appointments an absence would
	// break, T6.7).
	Conflicts any `json:"conflicts,omitempty"`
}

// NewProblem builds a problem with the standard title for status.
func NewProblem(status int, code, detail string) Problem {
	return Problem{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Code:   code,
		Detail: detail,
	}
}

// WriteProblem writes p as application/problem+json, filling the request ID and instance.
// Detail must never contain internal error messages.
func WriteProblem(w http.ResponseWriter, r *http.Request, p Problem) {
	if p.RequestID == "" {
		p.RequestID = RequestIDFrom(r.Context())
	}
	if p.Instance == "" && r.URL != nil {
		p.Instance = r.URL.Path
	}
	writeJSON(w, p.Status, ContentTypeProblem, p)
}

// WriteJSON writes v as application/json with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, "application/json", v)
}

func writeJSON(w http.ResponseWriter, status int, contentType string, v any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	// The status is already sent; an encoding error can only be a broken connection.
	_ = json.NewEncoder(w).Encode(v)
}
