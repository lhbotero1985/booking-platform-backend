package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
)

const token = "EAAtestTOKENvalue1234567890"

// fakeMeta answers like the Cloud API and records the last request.
type fakeMeta struct {
	status int
	body   string
	path   string
	auth   string
	req    sendRequest
}

func (f *fakeMeta) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.path, f.auth = r.URL.Path, r.Header.Get("Authorization")
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &f.req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	_, _ = io.WriteString(w, f.body)
}

func newClient(t *testing.T, f http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return New(Config{BaseURL: srv.URL, GraphVersion: "v23.0", PhoneNumberID: "106540352242922", AccessToken: token})
}

func outgoing() application.Outgoing {
	return application.Outgoing{MessageID: uuid.Must(uuid.NewV7()), Kind: domain.KindCreated, Channel: domain.ChannelWhatsApp, To: "+573001234567"}
}

func TestSendTemplate(t *testing.T) {
	f := &fakeMeta{status: http.StatusOK, body: `{"messaging_product":"whatsapp","contacts":[{"input":"573001234567","wa_id":"573001234567"}],"messages":[{"id":"wamid.HBgM123"}]}`}
	c := newClient(t, f)
	id, err := c.Send(context.Background(), outgoing())
	if err != nil || id != "wamid.HBgM123" {
		t.Fatalf("%q %v", id, err)
	}
	if f.path != "/v23.0/106540352242922/messages" || f.auth != "Bearer "+token {
		t.Errorf("request to %s with %q", f.path, f.auth)
	}
	r := f.req
	if r.MessagingProduct != "whatsapp" || r.RecipientType != "individual" || r.To != "573001234567" || r.Type != "template" ||
		r.Template.Name != "hello_world" || r.Template.Language.Code != "en_US" {
		t.Errorf("body %+v", r)
	}
	for _, k := range []domain.Kind{domain.KindCreated, domain.KindCancelled, domain.KindRescheduled, domain.KindReassigned} {
		if _, ok := templateFor(k); !ok {
			t.Errorf("no template for %s", k)
		}
	}
}

func TestSendFailures(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		code      string
		permanent bool
	}{
		{"expired token", 401, `{"error":{"message":"Error validating access token","type":"OAuthException","code":190,"fbtrace_id":"A1"}}`, "AUTH_FAILED", false},
		{"throttled", 429, `{"error":{"code":130429,"fbtrace_id":"A2"}}`, "RATE_LIMITED", false},
		{"spam rate", 400, `{"error":{"code":131048}}`, "RATE_LIMITED", false},
		{"not a test recipient", 400, `{"error":{"message":"Recipient phone number +573001234567 not in allowed list","code":131030,"fbtrace_id":"A3"}}`, "RECIPIENT_NOT_ALLOWED", true},
		{"not on WhatsApp", 400, `{"error":{"code":131026}}`, "RECIPIENT_UNAVAILABLE", true},
		{"template missing", 404, `{"error":{"code":132001}}`, "TEMPLATE_INVALID", true},
		{"meta down", 503, `upstream unavailable`, "PROVIDER_ERROR", false},
		{"meta internal", 500, `{"error":{"code":131000}}`, "PROVIDER_ERROR", false},
		{"missing permission", 403, `{"error":{"code":10}}`, "PERMISSION_DENIED", false},
		{"bad parameter", 400, `{"error":{"code":100}}`, "META_100", true},
		{"ok without id", 200, `{"messages":[]}`, "UNEXPECTED_RESPONSE", false},
	}
	for _, c := range cases {
		cl := newClient(t, &fakeMeta{status: c.status, body: c.body})
		_, err := cl.Send(context.Background(), outgoing())
		var de *application.DeliveryError
		if !errors.As(err, &de) || de.Code != c.code || de.Permanent != c.permanent {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		// Neither the token nor the number (Meta's text may quote it) end up in the error.
		if msg := err.Error(); strings.Contains(msg, token) || strings.Contains(msg, "3001234567") {
			t.Errorf("%s: error leaks data: %s", c.name, msg)
		}
	}
}

func TestSendUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // nothing listens there any more
	c := New(Config{BaseURL: srv.URL, GraphVersion: "v23.0", PhoneNumberID: "106540352242922", AccessToken: token})
	_, err := c.Send(context.Background(), outgoing())
	var de *application.DeliveryError
	if !errors.As(err, &de) || de.Code != "PROVIDER_UNREACHABLE" || de.Permanent {
		t.Fatalf("%v", err)
	}
	if strings.Contains(err.Error(), "106540352242922") || strings.Contains(err.Error(), token) {
		t.Errorf("leaks the URL: %v", err)
	}

	slow := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	})
	srv2 := httptest.NewServer(slow)
	t.Cleanup(srv2.Close)
	c = New(Config{BaseURL: srv2.URL, GraphVersion: "v23.0", PhoneNumberID: "1", AccessToken: token, HTTPClient: &http.Client{Timeout: 50 * time.Millisecond}})
	if _, err := c.Send(context.Background(), outgoing()); !errors.As(err, &de) || de.Permanent {
		t.Errorf("timeout must be retried: %v", err)
	}

	c = New(Config{BaseURL: "://bad", GraphVersion: "v23.0", PhoneNumberID: "1", AccessToken: token})
	if _, err := c.Send(context.Background(), outgoing()); !errors.As(err, &de) || de.Code != "CONFIG_INVALID" || !de.Permanent {
		t.Errorf("bad endpoint: %v", err)
	}
	m := outgoing()
	m.Kind = "UNKNOWN"
	if _, err := c.Send(context.Background(), m); !errors.As(err, &de) || de.Code != "TEMPLATE_MISSING" {
		t.Errorf("unknown kind: %v", err)
	}
}
