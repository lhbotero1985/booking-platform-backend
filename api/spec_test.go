package api_test

import (
	"strings"
	"testing"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"

	"github.com/lhbotero1985/booking-platform-backend/api"
)

// TestSpecIsValidOpenAPI31 parses the contract and validates it against the OpenAPI 3.1 schema.
func TestSpecIsValidOpenAPI31(t *testing.T) {
	doc, err := libopenapi.NewDocument(api.Spec)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v := doc.GetVersion(); v != "3.1.0" {
		t.Fatalf("openapi version = %q, want 3.1.0", v)
	}
	if _, err := doc.BuildV3Model(); err != nil {
		t.Fatalf("build model: %v", err)
	}
	v, errs := validator.NewValidator(doc)
	if len(errs) > 0 {
		t.Fatalf("validator: %v", errs)
	}
	if ok, verrs := v.ValidateDocument(); !ok {
		for _, e := range verrs {
			t.Errorf("spec invalid: %s (%s)", e.Message, e.Reason)
		}
	}
}

// TestSpecConventions enforces project rules that the OpenAPI schema itself does not:
// unique operationIds, tags on every operation, and problem+json for every error response.
func TestSpecConventions(t *testing.T) {
	doc, err := libopenapi.NewDocument(api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	model, err := doc.BuildV3Model()
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]string{}
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		for method, op := range item.GetOperations().FromOldest() {
			where := strings.ToUpper(method) + " " + path
			if op.OperationId == "" {
				t.Errorf("%s: missing operationId", where)
			} else if prev, dup := seen[op.OperationId]; dup {
				t.Errorf("%s: operationId %q already used by %s", where, op.OperationId, prev)
			} else {
				seen[op.OperationId] = where
			}
			if len(op.Tags) == 0 {
				t.Errorf("%s: missing tags", where)
			}
			if op.Responses == nil {
				t.Errorf("%s: no responses", where)
				continue
			}
			for code, resp := range op.Responses.Codes.FromOldest() {
				if code[0] != '4' && code[0] != '5' {
					continue
				}
				if resp.Content == nil || resp.Content.GetOrZero("application/problem+json") == nil {
					t.Errorf("%s %s: error responses must use application/problem+json", where, code)
				}
			}
		}
	}
	if len(seen) == 0 {
		t.Fatal("no operations found")
	}
}
