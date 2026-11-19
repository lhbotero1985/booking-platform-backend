package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/schema_validation"

	"github.com/lhbotero1985/booking-platform-backend/api"
)

// contract wraps the OpenAPI validator built from the embedded spec.
type contract struct {
	v   validator.Validator
	doc libopenapi.Document
}

func loadContract(t *testing.T) contract {
	t.Helper()
	doc, err := libopenapi.NewDocument(api.Spec)
	if err != nil {
		t.Fatal(err)
	}
	v, errs := validator.NewValidator(doc)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	return contract{v: v, doc: doc}
}

// assertConforms performs the request against srv and validates the response (status,
// headers, content type and body) against the contract.
func (c contract) assertConforms(t *testing.T, srv *httptest.Server, method, path string, wantStatus int) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: status = %d, want %d (body %s)", method, path, resp.StatusCode, wantStatus, body)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	if ok, errs := c.v.ValidateHttpResponse(req, resp); !ok {
		for _, e := range errs {
			t.Errorf("%s %s %d does not conform to the contract: %s (%s)", method, path, wantStatus, e.Message, e.Reason)
		}
	}
}

func (c contract) validateProblem(t *testing.T, body []byte) []string {
	t.Helper()
	model, err := c.doc.BuildV3Model()
	if err != nil {
		t.Fatal(err)
	}
	schema := model.Model.Components.Schemas.GetOrZero("Problem").Schema()
	ok, errs := schema_validation.NewSchemaValidator().ValidateSchemaBytes(schema, body)
	if ok {
		return nil
	}
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Message+": "+e.Reason)
	}
	return msgs
}

func TestContractOperations(t *testing.T) {
	c := loadContract(t)
	ok := func(context.Context) error { return nil }
	fail := func(context.Context) error { return errors.New("db down") }

	t.Run("GET /healthz 200", func(t *testing.T) {
		c.assertConforms(t, newTestServer(t), http.MethodGet, "/healthz", http.StatusOK)
	})
	t.Run("GET /readyz 200", func(t *testing.T) {
		c.assertConforms(t, newTestServer(t, Check{Name: "database", Fn: ok}), http.MethodGet, "/readyz", http.StatusOK)
	})
	t.Run("GET /readyz 503", func(t *testing.T) {
		c.assertConforms(t, newTestServer(t, Check{Name: "database", Fn: fail}), http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	})
}

// TestContractGenericErrorsAreProblems: 404 (unknown path) and 405 (unsupported method) are
// generic responses, not listed per operation, but must still be valid Problems.
func TestContractGenericErrorsAreProblems(t *testing.T) {
	c := loadContract(t)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/no-such-route", http.StatusNotFound},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{http.MethodDelete, "/readyz", http.StatusMethodNotAllowed},
	} {
		resp := do(t, newTestServer(t), tc.method, tc.path)
		if resp.status != tc.status {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, resp.status, tc.status)
			continue
		}
		if ct := resp.header.Get("Content-Type"); ct != "application/problem+json" {
			t.Errorf("%s %s: Content-Type = %q", tc.method, tc.path, ct)
		}
		if errs := c.validateProblem(t, resp.body); len(errs) > 0 {
			t.Errorf("%s %s: body is not a valid Problem: %v", tc.method, tc.path, errs)
		}
	}
}

// TestContractDetectsViolations proves the validator actually fails on non-conforming
// responses (otherwise the contract tests above would be meaningless).
func TestContractDetectsViolations(t *testing.T) {
	c := loadContract(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost:8080/healthz", nil)
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(http.StatusOK)
	_, _ = rec.WriteString(`{"status":"fine","version":"dev","extra":true}`)

	if ok, _ := c.v.ValidateHttpResponse(req, rec.Result()); ok {
		t.Fatal("validator accepted a /healthz body with wrong status value and an extra field")
	}

	if errs := c.validateProblem(t, []byte(`{"title":"x","status":200,"code":"lowercase"}`)); len(errs) == 0 {
		t.Fatal("validator accepted an invalid Problem (missing type, status < 400, bad code)")
	} else if !strings.Contains(strings.Join(errs, " "), "type") {
		t.Logf("violations: %v", errs)
	}
}
