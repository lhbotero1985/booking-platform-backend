package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
)

// TestAuditCarriesRequestContext: the request ID returned to the client and the TCP peer
// address reach the audit entry written by the use case.
func TestAuditCarriesRequestContext(t *testing.T) {
	f := newFixture()
	f.activeAccount(t)
	r := f.do(t, http.MethodPost, "/api/v1/auth/login", loginMobile, map[string]string{"X-Request-ID": "trace-login-1"})
	if r.status != http.StatusOK {
		t.Fatalf("login: %d %s", r.status, r.body)
	}
	a := f.store.LastAudit()
	if a.Action != application.AuditLogin || a.Outcome != application.OutcomeSuccess {
		t.Fatalf("audit = %+v", a)
	}
	if a.Metadata["request_id"] != "trace-login-1" || a.IP == nil || a.IP.String() != "192.0.2.1" {
		t.Fatalf("request context: %+v ip=%v", a.Metadata, a.IP)
	}
}
