package security_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/pb33f/libopenapi"

	"github.com/lhbotero1985/booking-platform-backend/api"
)

type route struct{ method, path, body string }

var placeholder = regexp.MustCompile(`\{[^}]+\}`)

// contractAuthenticatedRoutes (T6.3) returns every operation of api/openapi.yaml that declares a
// security requirement, so the credential matrix covers routes added later automatically. The
// hand-written authenticatedRoutes keep their realistic bodies; the other operations get a
// placeholder id and "{}" (authentication is checked before the body).
func contractAuthenticatedRoutes(t *testing.T) []route {
	t.Helper()
	doc, err := libopenapi.NewDocument(api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	model, err := doc.BuildV3Model()
	if err != nil {
		t.Fatal(err)
	}
	covered := func(method, template string) bool {
		pattern := "^" + placeholder.ReplaceAllString(template, `[^/]+`) + "$"
		for _, r := range authenticatedRoutes {
			if r.method == method && regexp.MustCompile(pattern).MatchString(r.path) {
				return true
			}
		}
		return false
	}
	out := make([]route, 0, len(authenticatedRoutes))
	for _, r := range authenticatedRoutes {
		out = append(out, route(r))
	}
	total := 0
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		for method, op := range item.GetOperations().FromOldest() {
			if len(op.Security) == 0 {
				continue // public
			}
			total++
			method = strings.ToUpper(method)
			if covered(method, path) {
				continue
			}
			concrete := strings.NewReplacer("{code}", "CO").Replace(path)
			concrete = placeholder.ReplaceAllString(concrete, "01a20aae-7e5d-73d7-b7d3-a11c5afb9655")
			body := ""
			if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
				body = "{}"
			}
			out = append(out, route{method, concrete, body})
		}
	}
	if total < 48 {
		t.Fatalf("only %d authenticated operations in the contract", total)
	}
	t.Logf("%d authenticated operations in the contract, %d requests in the matrix", total, len(out))
	return out
}
