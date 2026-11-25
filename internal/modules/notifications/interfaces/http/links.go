package httpapi

// Pages and API of the appointment emails' links (T7.7, ADR-0019). Two taps: GET only shows the
// appointment and a button; the button POSTs and the change happens then (mail scanners open the
// links of every email). Pages are for people: every outcome, an invalid link included, is a 200
// page that says what happened. The code is the credential: it travels in the query (the access log
// omits queries; Referrer-Policy is no-referrer) and in the form body.

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// LinkService is what the links' pages use.
type LinkService interface {
	Link(ctx context.Context, raw string, action domain.Action) (application.LinkView, error)
	UseLink(ctx context.Context, raw string, action domain.Action, start time.Time) (application.LinkView, error)
}

// LinksConfig of the pages.
type LinksConfig struct {
	// AppDownloadURL is where the reschedule page sends the customer while the frontend has no
	// page of its own ("" hides the link).
	AppDownloadURL string
}

// Links serves the pages and the API of the email links.
type Links struct {
	svc     LinkService
	cfg     LinksConfig
	limiter httpx.Allower
	logger  *slog.Logger
}

// NewLinks builds the handler; limiter is the strict per-client limit (nil disables it).
func NewLinks(svc LinkService, cfg LinksConfig, limiter httpx.Allower, logger *slog.Logger) *Links {
	return &Links{svc: svc, cfg: cfg, limiter: limiter, logger: logger}
}

// Routes registers the endpoints.
func (h *Links) Routes(rt *httpx.Router) {
	limit := httpx.RateLimit(h.limiter)
	rt.Handle("GET /citas/confirmar", limit(h.page(domain.ActionConfirm)))
	rt.Handle("POST /citas/confirmar", limit(h.apply(domain.ActionConfirm)))
	rt.Handle("GET /citas/cancelar", limit(h.page(domain.ActionCancel)))
	rt.Handle("POST /citas/cancelar", limit(h.apply(domain.ActionCancel)))
	rt.Handle("GET /citas/reagendar", limit(h.page(domain.ActionReschedule)))
	rt.Handle("POST /api/v1/appointment-links/lookup", limit(http.HandlerFunc(h.lookupJSON)))
	rt.Handle("POST /api/v1/appointment-links/apply", limit(http.HandlerFunc(h.applyJSON)))
}

const maxTokenLength = 100

func validToken(t string) bool { return t != "" && len(t) <= maxTokenLength }

// page is the first tap: the appointment and the button (no change).
func (h *Links) page(action domain.Action) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if !validToken(token) {
			h.render(w, http.StatusOK, pageView{Outcome: domain.OutcomeInvalid})
			return
		}
		v, err := h.svc.Link(r.Context(), token, action)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.render(w, http.StatusOK, h.viewOf(v, token))
	})
}

// apply is the second tap.
func (h *Links) apply(action domain.Action) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.PostFormValue("token")
		if !validToken(token) {
			h.render(w, http.StatusOK, pageView{Outcome: domain.OutcomeInvalid})
			return
		}
		v, err := h.svc.UseLink(r.Context(), token, action, time.Time{})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		h.render(w, http.StatusOK, h.viewOf(v, token))
	})
}

func (h *Links) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "appointment link failed", slog.String("error", err.Error()))
	h.render(w, http.StatusInternalServerError, pageView{Title: "Algo salió mal", Message: "No pudimos procesar tu solicitud. Intenta de nuevo en unos minutos."})
}

// pageView is what the page template shows.
type pageView struct {
	Outcome                    domain.Outcome
	Title, Message             string
	Business, Service, Date    string
	Time, Professional, Status string
	Notes                      []string
	Token, Action, Button      string
	Danger                     bool
	AppURL                     string
}

func (h *Links) viewOf(v application.LinkView, token string) pageView {
	c := v.Content
	p := pageView{Outcome: v.Outcome, Business: c.BusinessName, Service: c.ServiceName, Professional: c.ProfessionalName}
	if !c.Start.IsZero() {
		p.Date, p.Time = capitalize(c.DateES()), c.TimeES()
	}
	who := firstName(c.CustomerName)
	switch v.Outcome {
	case "":
		switch v.Action {
		case domain.ActionConfirm:
			p.Title, p.Message = "Confirma tu cita", "Hola "+who+", toca el botón para confirmar que asistirás."
			p.Action, p.Button = "/citas/confirmar", "Sí, confirmar mi cita"
		case domain.ActionCancel:
			p.Title, p.Message = "¿Cancelar tu cita?", "Hola "+who+", si la cancelas, ese horario queda libre para otra persona."
			p.Action, p.Button, p.Danger = "/citas/cancelar", "Sí, cancelar mi cita", true
			if c.CancelCutoffHours > 0 {
				p.Notes = append(p.Notes, "Puedes cancelar hasta "+hours(c.CancelCutoffHours)+" antes de la cita.")
			}
		case domain.ActionReschedule:
			p.Title, p.Message = "Cambia la hora de tu cita", "Hola "+who+", para elegir otra hora entra a la app de Booka. Si lo prefieres, comunícate con el negocio."
			if c.RescheduleCutoffHours > 0 {
				p.Notes = append(p.Notes, "Puedes reprogramar hasta "+hours(c.RescheduleCutoffHours)+" antes de la cita.")
			}
			if s := c.RescheduleLimitES(); s != "" {
				p.Notes = append(p.Notes, s)
			}
			p.AppURL = h.cfg.AppDownloadURL
		}
		p.Token = token
	case domain.OutcomeConfirmed:
		p.Title, p.Message = "¡Listo, "+who+"!", "Tu cita quedó confirmada. Te esperamos."
	case domain.OutcomeCancelled:
		p.Title, p.Message = "Tu cita fue cancelada", "Si quieres, puedes agendar otra desde la app."
	case domain.OutcomeRescheduled:
		p.Title, p.Message = "Tu cita cambió de hora", "Te esperamos en el nuevo horario."
	case domain.OutcomeCutoff:
		p.Title, p.Message = "Ya no es posible hacerlo por aquí", "Falta muy poco para la hora de tu cita. Por favor comunícate con "+c.BusinessName+"."
	case domain.OutcomeSlotUnavailable:
		p.Title, p.Message = "Ese horario ya no está disponible", "Elige otro horario."
	case domain.OutcomeRescheduleLimit:
		p.Title = "Ya no puedes cambiar la hora por aquí"
		p.Message = "Ya cambiaste la hora de esta cita las veces que permite " + c.BusinessName + ". Si necesitas otro cambio, comunícate con el negocio. Aún puedes cancelarla si lo necesitas."
	case domain.OutcomeNotActive:
		p.Title, p.Message = "Esta cita ya no está activa", "Si quieres, puedes agendar una nueva desde la app."
	default:
		p = pageView{Outcome: domain.OutcomeInvalid}
	}
	return p
}

func hours(h int) string {
	if h == 1 {
		return "1 hora"
	}
	return itoa(h) + " horas"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func firstName(full string) string {
	if f := strings.Fields(full); len(f) > 0 {
		return f[0]
	}
	return ""
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// pageCSP lets the page use its inline styles and post its form to this origin, nothing else.
const pageCSP = "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

func (h *Links) render(w http.ResponseWriter, status int, p pageView) {
	if p.Outcome == domain.OutcomeInvalid {
		p.Title = "Este enlace ya no es válido"
		p.Message = "Ya lo usaste, venció o tu cita cambió. Revisa el correo más reciente de tu cita o comunícate con el negocio."
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Security-Policy", pageCSP)
	hd.Set("Cache-Control", "no-store")
	hd.Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(status)
	if err := linkPage.Execute(w, p); err != nil {
		h.logger.Error("appointment link page", slog.String("error", err.Error()))
	}
}

var linkPage = template.Must(template.New("link").Parse(`<!doctype html>
<html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow"><title>{{.Title}} · Booka</title></head>
<body style="margin:0;background:#F8FAFC;font-family:Arial,Helvetica,sans-serif;color:#111827;">
<main style="max-width:480px;margin:0 auto;padding:24px 16px;">
<div style="text-align:center;font-size:26px;font-weight:bold;color:#7C3AED;margin-bottom:16px;">Booka</div>
<section style="background:#FFFFFF;border:1px solid #E2E8F0;border-radius:16px;overflow:hidden;">
{{if .Business}}<div style="background:#7C3AED;color:#FFFFFF;padding:18px 20px;font-size:18px;font-weight:bold;">{{.Business}}</div>{{end}}
<div style="padding:24px 20px;">
<h1 style="margin:0 0 12px;font-size:22px;">{{.Title}}</h1>
<p style="margin:0 0 16px;font-size:15px;line-height:1.5;color:#334155;">{{.Message}}</p>
{{if .Service}}<div style="font-size:15px;line-height:1.8;margin-bottom:12px;">
<div>✂️ <b>{{.Service}}</b></div>{{if .Date}}<div>📅 {{.Date}}</div><div>🕙 {{.Time}}</div>{{end}}{{if .Professional}}<div>👤 Con {{.Professional}}</div>{{end}}
</div>{{end}}
{{range .Notes}}<p style="margin:0 0 12px;font-size:14px;color:#4C1D95;background:#F3E8FF;border-radius:10px;padding:10px 12px;">{{.}}</p>{{end}}
{{if .Button}}<form method="post" action="{{.Action}}" style="margin:16px 0 0;">
<input type="hidden" name="token" value="{{.Token}}">
<button type="submit" style="width:100%;padding:14px;border-radius:10px;font-size:16px;font-weight:bold;cursor:pointer;{{if .Danger}}background:#FFFFFF;color:#B91C1C;border:1px solid #FCA5A5;{{else}}background:#7C3AED;color:#FFFFFF;border:1px solid #7C3AED;{{end}}">{{.Button}}</button>
</form>{{end}}
{{if .AppURL}}<p style="margin:16px 0 0;"><a href="{{.AppURL}}" style="color:#6D28D9;font-weight:bold;">Abrir la app de Booka</a></p>{{end}}
</div></section>
<p style="text-align:center;font-size:12px;color:#64748B;margin-top:16px;">Booka · reservas de citas</p>
</main></body></html>
`))

// --- JSON API for the frontend's reschedule page ---------------------------------------------

type linkRequest struct {
	Token   string     `json:"token"`
	Action  string     `json:"action"`
	StartAt *time.Time `json:"startAt,omitempty"`
}

type linkAppointmentDTO struct {
	BusinessID            uuid.UUID `json:"businessId"`
	BusinessSlug          string    `json:"businessSlug"`
	BusinessName          string    `json:"businessName"`
	ServiceID             uuid.UUID `json:"serviceId"`
	ServiceName           string    `json:"serviceName"`
	ProfessionalID        uuid.UUID `json:"professionalId"`
	ProfessionalName      string    `json:"professionalName"`
	StartAt               time.Time `json:"startAt"`
	TimeZone              string    `json:"timeZone"`
	CancelCutoffHours     int       `json:"cancelCutoffHours"`
	RescheduleCutoffHours int       `json:"rescheduleCutoffHours"`
	// T7.8: the business's limit of moves by the customer (0 = no limit) and those done.
	MaxCustomerReschedules int `json:"maxCustomerReschedules"`
	CustomerReschedules    int `json:"customerReschedules"`
}

type linkDTO struct {
	Action      string              `json:"action"`
	Outcome     *string             `json:"outcome"`
	Appointment *linkAppointmentDTO `json:"appointment"`
}

func parseAction(s string) (domain.Action, bool) {
	switch a := domain.Action(s); a {
	case domain.ActionConfirm, domain.ActionCancel, domain.ActionReschedule:
		return a, true
	}
	return "", false
}

func (h *Links) decode(w http.ResponseWriter, r *http.Request) (linkRequest, domain.Action, bool) {
	var in linkRequest
	if !httpx.DecodeJSON(w, r, &in) {
		return in, "", false
	}
	var fields []httpx.FieldError
	if !validToken(in.Token) {
		fields = append(fields, httpx.FieldError{Field: "token", Message: "INVALID"})
	}
	a, ok := parseAction(in.Action)
	if !ok {
		fields = append(fields, httpx.FieldError{Field: "action", Message: "INVALID"})
	}
	if len(fields) > 0 {
		writeValidation(w, r, fields)
		return in, "", false
	}
	return in, a, true
}

func writeValidation(w http.ResponseWriter, r *http.Request, fields []httpx.FieldError) {
	p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "one or more fields are invalid")
	p.Errors = fields
	httpx.WriteProblem(w, r, p)
}

func (h *Links) lookupJSON(w http.ResponseWriter, r *http.Request) {
	in, a, ok := h.decode(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Link(r.Context(), in.Token, a)
	h.writeLink(w, r, v, err)
}

func (h *Links) applyJSON(w http.ResponseWriter, r *http.Request) {
	in, a, ok := h.decode(w, r)
	if !ok {
		return
	}
	var start time.Time
	if a == domain.ActionReschedule {
		if in.StartAt == nil {
			writeValidation(w, r, []httpx.FieldError{{Field: "startAt", Message: "REQUIRED"}})
			return
		}
		start = in.StartAt.UTC()
	}
	v, err := h.svc.UseLink(r.Context(), in.Token, a, start)
	h.writeLink(w, r, v, err)
}

func (h *Links) writeLink(w http.ResponseWriter, r *http.Request, v application.LinkView, err error) {
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			h.logger.ErrorContext(r.Context(), "appointment link failed", slog.String("error", err.Error()))
		}
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, "internal error"))
		return
	}
	out := linkDTO{Action: string(v.Action)}
	if v.Outcome != "" {
		o := string(v.Outcome)
		out.Outcome = &o
	}
	if v.Outcome != domain.OutcomeInvalid && !v.Content.Start.IsZero() {
		c := v.Content
		tz := "UTC"
		if c.Location != nil {
			tz = c.Location.String()
		}
		out.Appointment = &linkAppointmentDTO{
			BusinessID: v.BusinessID, BusinessSlug: v.BusinessSlug, BusinessName: c.BusinessName, ServiceID: v.ServiceID,
			ServiceName: c.ServiceName, ProfessionalID: v.ProfessionalID, ProfessionalName: c.ProfessionalName,
			StartAt: c.Start.UTC(), TimeZone: tz, CancelCutoffHours: c.CancelCutoffHours, RescheduleCutoffHours: c.RescheduleCutoffHours,
			MaxCustomerReschedules: c.MaxReschedules, CustomerReschedules: c.Reschedules,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
