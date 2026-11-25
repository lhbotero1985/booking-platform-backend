package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

var (
	secret = strings.Repeat("cd", 16)
	verify = strings.Repeat("token", 4)
)

type receiver struct {
	items [][]domain.WebhookItem
	err   error
}

func (r *receiver) ReceiveWebhook(_ context.Context, items []domain.WebhookItem) error {
	r.items = append(r.items, items)
	return r.err
}

func server(t *testing.T, cfg WebhookConfig) (*httptest.Server, *receiver) {
	t.Helper()
	r := &receiver{}
	rt := httpx.NewRouter()
	NewWebhook(r, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes(rt)
	srv := httptest.NewServer(rt)
	t.Cleanup(srv.Close)
	return srv, r
}

func sign(body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func do(t *testing.T, method, url, body string, header map[string]string) (int, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), method, url, bytes.NewBufferString(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestWebhookVerification(t *testing.T) {
	srv, _ := server(t, WebhookConfig{AppSecret: secret, VerifyToken: verify})
	u := srv.URL + "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=" + verify + "&hub.challenge=1158201444"
	if code, body := do(t, http.MethodGet, u, "", nil); code != 200 || body != "1158201444" {
		t.Errorf("valid: %d %q", code, body)
	}
	for name, q := range map[string]string{
		"wrong token":   "hub.mode=subscribe&hub.verify_token=nope&hub.challenge=1",
		"wrong mode":    "hub.mode=unsubscribe&hub.verify_token=" + verify + "&hub.challenge=1",
		"missing token": "hub.mode=subscribe&hub.challenge=1",
	} {
		if code, _ := do(t, http.MethodGet, srv.URL+"/webhooks/whatsapp?"+q, "", nil); code != 403 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := do(t, http.MethodGet, srv.URL+"/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token="+verify+"&hub.challenge=%3Cscript%3E%0A", "", nil); code != 400 {
		t.Errorf("challenge with control characters: %d", code)
	}
	off, _ := server(t, WebhookConfig{})
	if code, _ := do(t, http.MethodGet, off.URL+"/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=&hub.challenge=1", "", nil); code != 404 {
		t.Errorf("not configured: %d", code)
	}
}

const metaBody = `{"object":"whatsapp_business_account","entry":[{"id":"102290129340398","changes":[{"field":"messages","value":{
 "messaging_product":"whatsapp","metadata":{"display_phone_number":"15550000000","phone_number_id":"106540352242922"},
 "statuses":[
  {"id":"wamid.A","status":"sent","timestamp":"1","recipient_id":"573001234567"},
  {"id":"wamid.A","status":"delivered","timestamp":"2","recipient_id":"573001234567"},
  {"id":"wamid.A","status":"read","timestamp":"3","recipient_id":"573001234567"},
  {"id":"wamid.B","status":"failed","timestamp":"4","recipient_id":"573001234567","errors":[{"code":131026,"title":"Message undeliverable"}]}],
 "messages":[
  {"from":"573001234567","id":"wamid.IN1","timestamp":"5","type":"button","context":{"from":"15550000000","id":"wamid.R"},
   "button":{"payload":"tok-123","text":"Confirmar cita"}},
  {"from":"573001234567","id":"wamid.IN2","timestamp":"6","type":"text","text":{"body":"hola"}},
  {"from":"+57 300","id":"wamid.IN3","timestamp":"7","type":"button","button":{"payload":"x","text":"y"}}]}},
 {"field":"account_update","value":{}}]}]}`

func TestWebhookReceive(t *testing.T) {
	srv, r := server(t, WebhookConfig{AppSecret: secret, VerifyToken: verify})
	url := srv.URL + "/webhooks/whatsapp"
	if code, _ := do(t, http.MethodPost, url, metaBody, map[string]string{"X-Hub-Signature-256": sign(metaBody), "Content-Type": "application/json"}); code != 200 {
		t.Fatalf("signed: %d", code)
	}
	if len(r.items) != 1 {
		t.Fatalf("received %d batches", len(r.items))
	}
	got := r.items[0]
	want := []domain.WebhookItem{
		{Kind: domain.WebhookStatus, EventID: "status:wamid.A:DELIVERED", ProviderMessageID: "wamid.A", Status: domain.StatusDelivered},
		{Kind: domain.WebhookStatus, EventID: "status:wamid.A:READ", ProviderMessageID: "wamid.A", Status: domain.StatusRead},
		{Kind: domain.WebhookStatus, EventID: "status:wamid.B:FAILED", ProviderMessageID: "wamid.B", Status: domain.StatusFailed, ErrorCode: "META_131026"},
		{Kind: domain.WebhookButton, EventID: "message:wamid.IN1", From: "+573001234567", Payload: "tok-123"},
	}
	if len(got) != len(want) {
		t.Fatalf("items %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Not signed by Meta: refused, nothing received.
	for name, sig := range map[string]string{
		"missing":        "",
		"wrong secret":   "sha256=" + strings.Repeat("0", 64),
		"other body":     sign(metaBody + " "),
		"not hex":        "sha256=zz",
		"without prefix": strings.TrimPrefix(sign(metaBody), "sha256="),
	} {
		if code, body := do(t, http.MethodPost, url, metaBody, map[string]string{"X-Hub-Signature-256": sig}); code != 401 || !strings.Contains(body, CodeInvalidSignature) {
			t.Errorf("%s: %d %s", name, code, body)
		}
	}
	if len(r.items) != 1 {
		t.Errorf("unsigned bodies reached the service")
	}
	// Signed but not JSON: 400. The service fails: 500 so Meta retries.
	if code, _ := do(t, http.MethodPost, url, "nope", map[string]string{"X-Hub-Signature-256": sign("nope")}); code != 400 {
		t.Errorf("not JSON: %d", code)
	}
	r.err = errors.New("db down")
	if code, _ := do(t, http.MethodPost, url, metaBody, map[string]string{"X-Hub-Signature-256": sign(metaBody)}); code != 500 {
		t.Errorf("service failure: %d", code)
	}
	big := strings.Repeat("a", maxWebhookBody+1)
	if code, _ := do(t, http.MethodPost, url, big, map[string]string{"X-Hub-Signature-256": sign(big)}); code != 413 {
		t.Errorf("too large: %d", code)
	}
	off, _ := server(t, WebhookConfig{})
	if code, _ := do(t, http.MethodPost, off.URL+"/webhooks/whatsapp", metaBody, map[string]string{"X-Hub-Signature-256": sign(metaBody)}); code != 404 {
		t.Errorf("not configured: %d", code)
	}
}
