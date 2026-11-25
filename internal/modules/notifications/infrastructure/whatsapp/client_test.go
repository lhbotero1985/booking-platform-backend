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
	"unicode/utf8"

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
	f.req = sendRequest{}
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

var bogota, _ = time.LoadLocation("America/Bogota")

func outgoing() application.Outgoing {
	return application.Outgoing{
		MessageID: uuid.Must(uuid.NewV7()), Kind: domain.KindCreated, Channel: domain.ChannelWhatsApp, To: "+573001234567",
		Content: domain.Content{
			CustomerName: "Laura", BusinessName: "Barbería El Paisa", ServiceName: "Corte + barba", ProfessionalName: "Juan",
			Start: time.Date(2026, 12, 2, 15, 0, 0, 0, time.UTC), Location: bogota, CancelReason: "El barbero está incapacitado",
		},
	}
}

func params(r sendRequest) []string {
	var out []string
	for _, c := range r.Template.Components {
		if c.Type != "body" {
			continue
		}
		for _, p := range c.Parameters {
			if p.Type != "text" {
				return nil
			}
			out = append(out, p.Text)
		}
	}
	return out
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
	if r.MessagingProduct != "whatsapp" || r.RecipientType != "individual" || r.To != "573001234567" || r.Type != "template" {
		t.Errorf("body %+v", r)
	}
}

// TestTemplates: the name, language and order of the parameters of each template, as approved in
// Meta on 2026-11-25.
func TestTemplates(t *testing.T) {
	when := "miércoles 2 de diciembre, 10:00 a. m."
	cases := []struct {
		kind   domain.Kind
		name   string
		params []string
	}{
		{domain.KindCreated, "cita_agendada", []string{"Laura", "Barbería El Paisa", "Corte + barba", "Juan", when}},
		{domain.KindCreatedByBusiness, "cita_agendada_negocio", []string{"Laura", "Barbería El Paisa", "Corte + barba", "Juan", when}},
		{domain.KindCancelled, "cita_cancelada", []string{"Laura", "Barbería El Paisa", "Corte + barba", when, "El barbero está incapacitado"}},
		{domain.KindRescheduled, "cita_reprogramada", []string{"Laura", "Corte + barba", "Barbería El Paisa", when, "Juan"}},
		{domain.KindReassigned, "cita_cambio_barbero", []string{"Laura", "Corte + barba", "Barbería El Paisa", when, "Juan"}},
		{domain.KindReminder, "recordatorio_cita", []string{"Laura", "Corte + barba", "Barbería El Paisa", when, "Juan"}},
	}
	for _, c := range cases {
		f := &fakeMeta{status: http.StatusOK, body: `{"messages":[{"id":"wamid.1"}]}`}
		m := outgoing()
		m.Kind = c.kind
		if _, err := newClient(t, f).Send(context.Background(), m); err != nil {
			t.Fatalf("%s: %v", c.kind, err)
		}
		got := params(f.req)
		if f.req.Template.Name != c.name || f.req.Template.Language.Code != "es" || strings.Join(got, "|") != strings.Join(c.params, "|") {
			t.Errorf("%s: %s/%s %q", c.kind, f.req.Template.Name, f.req.Template.Language.Code, got)
		}
	}
	if len(templates) != len(cases) {
		t.Errorf("%d templates, %d tested", len(templates), len(cases))
	}
}

func TestParamsAreSafeForMeta(t *testing.T) {
	long := strings.Repeat("á", 300)
	for in, want := range map[string]string{
		"  Laura \n\t Gómez  ": "Laura Gómez",
		"":                     "-",
		"   ":                  "-",
		"a    b":               "a b",
	} {
		if got := param(in); got != want {
			t.Errorf("param(%q) = %q, want %q", in, got, want)
		}
	}
	if got := param(long); utf8.RuneCountInString(got) != maxParam || !strings.HasSuffix(got, "…") {
		t.Errorf("long: %d runes", utf8.RuneCountInString(got))
	}
	// A canceled appointment without a reason still fills {{5}}.
	m := outgoing()
	m.Kind, m.Content.CancelReason = domain.KindCancelled, ""
	f := &fakeMeta{status: http.StatusOK, body: `{"messages":[{"id":"wamid.1"}]}`}
	if _, err := newClient(t, f).Send(context.Background(), m); err != nil || params(f.req)[4] != "Sin motivo indicado" {
		t.Errorf("no reason: %v %q", err, params(f.req))
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

func TestReminderButtonsAndTextReplies(t *testing.T) {
	f := &fakeMeta{status: http.StatusOK, body: `{"messages":[{"id":"wamid.1"}]}`}
	c := newClient(t, f)
	m := outgoing()
	m.Kind, m.ButtonPayloads = domain.KindReminder, []string{"tok-confirm", "tok-cancel"}
	if _, err := c.Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	var buttons []string
	for _, comp := range f.req.Template.Components {
		if comp.Type == "button" {
			if comp.SubType != "quick_reply" || len(comp.Parameters) != 1 || comp.Parameters[0].Type != "payload" {
				t.Errorf("button %+v", comp)
			}
			buttons = append(buttons, comp.Index+"="+comp.Parameters[0].Payload)
		}
	}
	if strings.Join(buttons, ",") != "0=tok-confirm,1=tok-cancel" {
		t.Errorf("buttons %v", buttons)
	}

	id, err := c.SendText(context.Background(), "+573001234567", "¡Listo!")
	if err != nil || id != "wamid.1" || f.req.Type != "text" || f.req.Text == nil || f.req.Text.Body != "¡Listo!" || f.req.To != "573001234567" ||
		f.req.Template != nil || f.req.Text.PreviewURL {
		t.Errorf("text: %q %v %+v", id, err, f.req)
	}
}
