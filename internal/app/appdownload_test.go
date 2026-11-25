package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppDownload(t *testing.T) {
	rec := httptest.NewRecorder()
	appDownload("")(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/app", nil))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") ||
		!strings.Contains(rec.Body.String(), "disponible muy pronto") {
		t.Errorf("pending page: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	rec = httptest.NewRecorder()
	appDownload("https://play.google.com/store/apps/details?id=example")(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/app", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://play.google.com/store/apps/details?id=example" {
		t.Errorf("store redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}
