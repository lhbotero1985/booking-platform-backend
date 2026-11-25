package httpapi

// WhatsApp webhook (T7.5, notifications.md §3): GET answers Meta's verification with the verify
// token; POST accepts only bodies signed by Meta (X-Hub-Signature-256, HMAC-SHA256 with the App
// Secret over the raw body), turns them into items, stores and queues them and answers 200 at
// once. Nothing of the body is logged.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// CodeInvalidSignature means the body is not signed by Meta.
const CodeInvalidSignature = "INVALID_SIGNATURE"

// WebhookConfig are the secrets of the webhook; empty disables it (404).
type WebhookConfig struct {
	AppSecret   string
	VerifyToken string
}

// WebhookService is what the webhook calls.
type WebhookService interface {
	ReceiveWebhook(ctx context.Context, items []domain.WebhookItem) error
}

// Webhook serves /webhooks/whatsapp.
type Webhook struct {
	svc    WebhookService
	cfg    WebhookConfig
	logger *slog.Logger
}

// NewWebhook builds the handler.
func NewWebhook(svc WebhookService, cfg WebhookConfig, logger *slog.Logger) *Webhook {
	return &Webhook{svc: svc, cfg: cfg, logger: logger}
}

// Routes registers the endpoints.
func (h *Webhook) Routes(rt *httpx.Router) {
	rt.HandleFunc("GET /webhooks/whatsapp", h.verify)
	rt.HandleFunc("POST /webhooks/whatsapp", h.receive)
}

// maxWebhookBody bounds what is read (Meta's batches are small).
const maxWebhookBody = 1 << 20

func (h *Webhook) enabled(w http.ResponseWriter, r *http.Request) bool {
	if h.cfg.AppSecret == "" || h.cfg.VerifyToken == "" {
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
		return false
	}
	return true
}

// verify answers GET ?hub.mode=subscribe&hub.verify_token=…&hub.challenge=… with the challenge.
func (h *Webhook) verify(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w, r) {
		return
	}
	q := r.URL.Query()
	challenge := q.Get("hub.challenge")
	if q.Get("hub.mode") != "subscribe" || subtle.ConstantTimeCompare([]byte(q.Get("hub.verify_token")), []byte(h.cfg.VerifyToken)) != 1 {
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusForbidden, httpx.CodeForbidden, "verification failed"))
		return
	}
	// Meta's challenge is a number: only a number is echoed back.
	n, err := strconv.ParseUint(challenge, 10, 64)
	if err != nil {
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "invalid challenge"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, strconv.FormatUint(n, 10))
}

func (h *Webhook) receive(w http.ResponseWriter, r *http.Request) {
	if !h.enabled(w, r) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody+1))
	if err != nil || len(body) > maxWebhookBody {
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusRequestEntityTooLarge, httpx.CodeBodyTooLarge, ""))
		return
	}
	if !validSignature(h.cfg.AppSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		h.logger.WarnContext(r.Context(), "whatsapp webhook with an invalid signature",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusUnauthorized, CodeInvalidSignature, "invalid signature"))
		return
	}
	items, err := ParseWebhook(body)
	if err != nil {
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "invalid payload"))
		return
	}
	if err := h.svc.ReceiveWebhook(r.Context(), items); err != nil {
		h.logger.ErrorContext(r.Context(), "whatsapp webhook failed",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())), slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, "")) // Meta retries
		return
	}
	w.WriteHeader(http.StatusOK)
}

// validSignature checks "sha256=<hex>" against HMAC-SHA256(secret, body) in constant time.
func validSignature(secret string, body []byte, header string) bool {
	got, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil || !strings.HasPrefix(header, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// The part of Meta's webhook payload that matters (field "messages").
type webhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				Statuses []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
					Errors []struct {
						Code int `json:"code"`
					} `json:"errors"`
				} `json:"statuses"`
				Messages []struct {
					ID     string `json:"id"`
					From   string `json:"from"`
					Type   string `json:"type"`
					Button *struct {
						Payload string `json:"payload"`
					} `json:"button"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

var deliveryStatus = map[string]domain.Status{
	"delivered": domain.StatusDelivered,
	"read":      domain.StatusRead,
	"failed":    domain.StatusFailed,
}

// ParseWebhook extracts the delivery statuses and the quick-reply answers. Everything else (sent
// statuses, texts the customer writes, other fields) is ignored.
func ParseWebhook(body []byte) ([]domain.WebhookItem, error) {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	var out []domain.WebhookItem
	for _, e := range p.Entry {
		for _, c := range e.Changes {
			if c.Field != "messages" {
				continue
			}
			for _, s := range c.Value.Statuses {
				st, ok := deliveryStatus[s.Status]
				if !ok || s.ID == "" || len(s.ID) > 200 {
					continue
				}
				it := domain.WebhookItem{Kind: domain.WebhookStatus, EventID: "status:" + s.ID + ":" + string(st), ProviderMessageID: s.ID, Status: st}
				if st == domain.StatusFailed && len(s.Errors) > 0 {
					it.ErrorCode = "META_" + strconv.Itoa(s.Errors[0].Code)
				}
				out = append(out, it)
			}
			for _, m := range c.Value.Messages {
				if m.Type != "button" || m.Button == nil || m.ID == "" || len(m.ID) > 200 || m.Button.Payload == "" ||
					len(m.Button.Payload) > 256 || !digits(m.From) {
					continue
				}
				out = append(out, domain.WebhookItem{Kind: domain.WebhookButton, EventID: "message:" + m.ID, From: "+" + m.From, Payload: m.Button.Payload})
			}
		}
	}
	return out, nil
}

func digits(s string) bool {
	if len(s) < 8 || len(s) > 15 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
