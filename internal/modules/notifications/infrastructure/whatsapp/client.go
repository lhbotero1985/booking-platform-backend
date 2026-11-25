// Package whatsapp sends notification messages through the WhatsApp Cloud API of Meta (T7.2,
// notifications.md §3). Business-initiated messages are approved templates only.
//
// Secrets: the access token travels only in the Authorization header; it is never logged and
// never part of an error. Errors carry Meta's numeric code and trace id, not Meta's text (which
// may quote the recipient).
package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
)

// Config of the client.
type Config struct {
	BaseURL       string // https://graph.facebook.com
	GraphVersion  string // v23.0
	PhoneNumberID string
	AccessToken   string
	// HTTPClient defaults to a client with a 15-second timeout.
	HTTPClient *http.Client
}

// Client implements application.Sender for the WHATSAPP channel.
type Client struct {
	cfg      Config
	http     *http.Client
	endpoint string
}

// New builds the client.
func New(cfg Config) *Client {
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{cfg: cfg, http: h, endpoint: cfg.BaseURL + "/" + cfg.GraphVersion + "/" + cfg.PhoneNumberID + "/messages"}
}

type language struct {
	Code string `json:"code"`
}

type templateRef struct {
	Name       string      `json:"name"`
	Language   language    `json:"language"`
	Components []component `json:"components,omitempty"`
}

type component struct {
	Type       string      `json:"type"`
	SubType    string      `json:"sub_type,omitempty"` // buttons: quick_reply
	Index      string      `json:"index,omitempty"`    // buttons: "0", "1"…
	Parameters []parameter `json:"parameters"`
}

type parameter struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Payload string `json:"payload,omitempty"`
}

type textBody struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

type sendRequest struct {
	MessagingProduct string       `json:"messaging_product"`
	RecipientType    string       `json:"recipient_type"`
	To               string       `json:"to"`
	Type             string       `json:"type"`
	Template         *templateRef `json:"template,omitempty"`
	Text             *textBody    `json:"text,omitempty"`
}

type sendResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error *metaError `json:"error"`
}

type metaError struct {
	Code      int    `json:"code"`
	Subcode   int    `json:"error_subcode"`
	FBTraceID string `json:"fbtrace_id"`
}

// Send implements application.Sender: it sends the template of the message kind to m.To and
// returns Meta's message id (wamid).
func (c *Client) Send(ctx context.Context, m application.Outgoing) (string, error) {
	t, ok := templateFor(m.Kind)
	if !ok {
		return "", &application.DeliveryError{Code: "TEMPLATE_MISSING", Permanent: true, Err: fmt.Errorf("whatsapp: no template for %s", m.Kind)}
	}
	ref := t.ref(m.Content)
	for i, p := range m.ButtonPayloads {
		ref.Components = append(ref.Components, component{
			Type: "button", SubType: "quick_reply", Index: strconv.Itoa(i), Parameters: []parameter{{Type: "payload", Payload: p}},
		})
	}
	return c.post(ctx, sendRequest{
		MessagingProduct: "whatsapp", RecipientType: "individual", To: strings.TrimPrefix(m.To, "+"), Type: "template", Template: &ref,
	})
}

// SendText implements application.TextSender: a free-text answer, allowed within 24 hours of the
// customer's last message.
func (c *Client) SendText(ctx context.Context, to, text string) (string, error) {
	return c.post(ctx, sendRequest{
		MessagingProduct: "whatsapp", RecipientType: "individual", To: strings.TrimPrefix(to, "+"), Type: "text",
		Text: &textBody{Body: text},
	})
}

func (c *Client) post(ctx context.Context, msg sendRequest) (string, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", &application.DeliveryError{Code: "CONFIG_INVALID", Permanent: true, Err: errors.New("whatsapp: invalid endpoint")}
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// The error names the URL (phone number id, no token); the cause class is enough.
		return "", &application.DeliveryError{Code: "PROVIDER_UNREACHABLE", Err: fmt.Errorf("whatsapp: request failed: %w", unwrapTransport(err))}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", &application.DeliveryError{Code: "PROVIDER_UNREACHABLE", Err: fmt.Errorf("whatsapp: read response: %w", err)}
	}
	var out sendResponse
	_ = json.Unmarshal(raw, &out) // a non-JSON answer is classified by the status below

	if resp.StatusCode >= 200 && resp.StatusCode < 300 && len(out.Messages) > 0 && out.Messages[0].ID != "" {
		return out.Messages[0].ID, nil
	}
	return "", classify(resp.StatusCode, out.Error)
}

// unwrapTransport drops the *url.Error wrapper (method and URL) and keeps the cause.
func unwrapTransport(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// classify maps Meta's answer to a DeliveryError (developers.facebook.com, WhatsApp Cloud API
// error codes). Throttling and Meta's own failures are retried; everything about the request,
// the recipient or the template is permanent. A rejected token is retried: once the token is
// replaced the pending messages go out.
func classify(status int, e *metaError) error {
	code := 0
	trace := ""
	if e != nil {
		code, trace = e.Code, e.FBTraceID
	}
	mk := func(c string, permanent bool) error {
		return &application.DeliveryError{Code: c, Permanent: permanent, Err: fmt.Errorf("whatsapp: http %d, meta code %d, trace %q", status, code, trace)}
	}
	switch {
	case code == 190 || status == http.StatusUnauthorized:
		return mk("AUTH_FAILED", false)
	case code == 4 || code == 80007 || code == 130429 || code == 131048 || code == 131056 || status == http.StatusTooManyRequests:
		return mk("RATE_LIMITED", false)
	case code == 131030:
		return mk("RECIPIENT_NOT_ALLOWED", true) // not a registered recipient of the test number
	case code == 131026:
		return mk("RECIPIENT_UNAVAILABLE", true) // not on WhatsApp, or cannot receive
	case code >= 132000 && code < 133000:
		return mk("TEMPLATE_INVALID", true)
	case code == 131000 || code == 1 || code == 2 || status >= 500 || status == 0:
		return mk("PROVIDER_ERROR", false)
	case status == http.StatusForbidden || code == 10 || code == 200:
		return mk("PERMISSION_DENIED", false)
	case status >= 400:
		return mk(fmt.Sprintf("META_%d", code), true)
	default:
		return mk("UNEXPECTED_RESPONSE", false)
	}
}
