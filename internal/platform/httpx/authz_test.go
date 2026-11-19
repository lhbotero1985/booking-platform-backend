package httpx_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

func TestWriteAuthzProblem(t *testing.T) {
	cases := []struct {
		err    error
		mapped bool
		status int
		code   string
	}{
		{authz.ErrNotFound, true, http.StatusNotFound, httpx.CodeNotFound},
		{fmt.Errorf("load appointment: %w", authz.ErrNotFound), true, http.StatusNotFound, httpx.CodeNotFound},
		{authz.ErrForbidden, true, http.StatusForbidden, httpx.CodeForbidden},
		{errors.New("db down"), false, 0, ""},
		{nil, false, 0, ""},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
		if got := httpx.WriteAuthzProblem(rec, r, c.err); got != c.mapped {
			t.Errorf("%v: mapped = %v", c.err, got)
			continue
		}
		if !c.mapped {
			if rec.Body.Len() != 0 {
				t.Errorf("%v: wrote a body", c.err)
			}
			continue
		}
		if rec.Code != c.status || rec.Header().Get("Content-Type") != httpx.ContentTypeProblem ||
			!strings.Contains(rec.Body.String(), `"code":"`+c.code+`"`) {
			t.Errorf("%v: %d %s", c.err, rec.Code, rec.Body.String())
		}
	}
}
