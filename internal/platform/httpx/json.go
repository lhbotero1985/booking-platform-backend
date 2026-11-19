package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// DecodeJSON decodes a single JSON object from the request body into dst. Unknown fields and
// trailing data are rejected. On failure it writes the problem response and returns false.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(dst)
	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
		err = errors.New("body must contain a single JSON object")
	}
	if err == nil {
		return true
	}

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteProblem(w, r, NewProblem(http.StatusRequestEntityTooLarge, CodeBodyTooLarge, "request body too large"))
		return false
	}
	WriteProblem(w, r, NewProblem(http.StatusBadRequest, CodeInvalidRequest, "malformed JSON body"))
	return false
}
