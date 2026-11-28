package app

import (
	"io"
	"log/slog"
	"sort"
	"strings"
	"testing"
)

// TestEveryRouteIsDocumented: the routes registered by the modules (as in cmd/api) and the
// operations of api/openapi.yaml are the same set, so no endpoint ships undocumented and no
// documented endpoint is missing (ServeMux patterns and OpenAPI paths share the {param} syntax).
func TestEveryRouteIsDocumented(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	registered := map[string]bool{}
	for _, p := range newRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Deps{Modules: e.modules, Media: e.media.Handler()}, "").Patterns() {
		registered[strings.ReplaceAll(p, "...}", "}")] = true // ServeMux {key...} is OpenAPI {key}
	}
	model, err := c.doc.BuildV3Model()
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		for method := range item.GetOperations().FromOldest() {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}
	var undocumented, missing []string
	for r := range registered {
		if !documented[r] {
			undocumented = append(undocumented, r)
		}
	}
	for d := range documented {
		if !registered[d] {
			missing = append(missing, d)
		}
	}
	sort.Strings(undocumented)
	sort.Strings(missing)
	if len(undocumented) > 0 {
		t.Errorf("routes not in api/openapi.yaml: %v", undocumented)
	}
	if len(missing) > 0 {
		t.Errorf("documented operations without a route: %v", missing)
	}
	if len(registered) < 30 {
		t.Errorf("only %d routes registered: the composition is incomplete", len(registered))
	}
	t.Logf("%d routes, all documented", len(registered))
}
