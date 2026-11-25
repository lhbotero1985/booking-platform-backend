// Package email renders and sends the appointment emails (T7.6, ADR-0019): HTML with its plain
// text, Booka's own look (white, purple), the data of the appointment, the business's policy and
// contact. Every value is escaped by html/template.
package email

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"strconv"
	"strings"
	texttemplate "text/template"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
)

// Rendered is an email ready to send.
type Rendered struct {
	Subject, Text, HTML string
}

// texts is what an email says for one kind and recipient.
type texts struct {
	subject, title, intro string
}

// view is what the templates show.
type view struct {
	Title, Intro, Badge, BadgeColor, BadgeBackground    string
	BusinessName, ServiceName, Date, Time, Professional string
	CustomerName, CancelReason                          string
	Address, MapsURL, Phone, WhatsAppURL                string
	// PhoneURL is a tel: link (html/template only trusts http, https and mailto by itself).
	PhoneURL        htmltemplate.URL
	Policy          []string
	AppURL          string
	ForProfessional bool
}

var statuses = map[string][3]string{ // label, text color, background
	"PENDING_CONFIRMATION": {"Pendiente de confirmar", "#92400E", "#FEF3C7"},
	"CONFIRMED":            {"Confirmada", "#166534", "#DCFCE7"},
	"CANCELLED":            {"Cancelada", "#991B1B", "#FEE2E2"}, //nolint:misspell // stored value
	"IN_PROGRESS":          {"En curso", "#5B21B6", "#EDE9FE"},
	"COMPLETED":            {"Atendida", "#334155", "#F1F5F9"},
	"NO_SHOW":              {"No asistió", "#334155", "#F1F5F9"},
}

// copyOf returns the texts of an email (approved by the user on 2026-11-25).
func copyOf(kind domain.Kind, role domain.Role, c domain.Content) (texts, bool) {
	b, cust, pro := c.BusinessName, firstName(c.CustomerName), firstName(c.ProfessionalName)
	short := c.WhenES()
	if role == domain.RecipientProfessional {
		switch kind {
		case domain.KindCreated:
			return texts{"Nueva cita: " + c.CustomerName + ", " + short, "Tienes una nueva cita",
				"Hola " + pro + ", " + c.CustomerName + " agendó una cita contigo:"}, true
		case domain.KindCancelledByCustomer:
			return texts{"Cita cancelada: " + c.CustomerName + ", " + short, "Un cliente canceló su cita",
				"Hola " + pro + ", " + c.CustomerName + " canceló su cita contigo. Ese espacio quedó libre en tu agenda."}, true
		case domain.KindRescheduled:
			return texts{"Cita reprogramada: " + c.CustomerName + ", " + short, "Un cliente cambió la hora de su cita",
				"Hola " + pro + ", " + c.CustomerName + " movió su cita contigo. Ahora es:"}, true
		case domain.KindCreatedByBusiness, domain.KindCancelled, domain.KindReassigned, domain.KindReminder:
			// The staff's own changes and the reminders are not told to the professional.
		}
		return texts{}, false
	}
	switch kind {
	case domain.KindCreated:
		return texts{"Tu cita en " + b + " está agendada", "¡Tu cita está agendada!",
			"Hola " + cust + ", estos son los datos de tu cita:"}, true
	case domain.KindCreatedByBusiness:
		return texts{"Tu cita en " + b + " está agendada", "¡Tu cita está agendada!",
			"Hola " + cust + ", " + b + " te agendó esta cita:"}, true
	case domain.KindCancelled:
		return texts{"Tu cita en " + b + " fue cancelada", "Tu cita fue cancelada",
			"Hola " + cust + ", " + b + " canceló tu cita. Si quieres, puedes agendar otra cuando lo necesites."}, true
	case domain.KindCancelledByCustomer:
		return texts{"Tu cita en " + b + " fue cancelada", "Tu cita fue cancelada",
			"Hola " + cust + ", cancelaste esta cita. Si quieres, puedes agendar otra cuando lo necesites."}, true
	case domain.KindRescheduled:
		return texts{"Tu cita en " + b + " cambió de hora", "Tu cita cambió de hora",
			"Hola " + cust + ", tu cita ahora es:"}, true
	case domain.KindReassigned:
		return texts{"Tu cita en " + b + " tiene otro barbero", "Tu cita tiene otro barbero",
			"Hola " + cust + ", ahora te atenderá " + c.ProfessionalName + ":"}, true
	case domain.KindReminder:
		return texts{"Recordatorio: tu cita en " + b + " es el " + short, "Te esperamos",
			"Hola " + cust + ", te recordamos tu cita. Si no puedes asistir, cancélala a tiempo o comunícate con el negocio."}, true
	}
	return texts{}, false
}

// Render builds the email of a message. false when the kind has no email for that recipient.
func Render(m application.Outgoing, appURL string) (Rendered, bool, error) {
	c := m.Content
	cp, ok := copyOf(m.Kind, m.Role, c)
	if !ok {
		return Rendered{}, false, nil
	}
	v := view{
		Title: cp.title, Intro: cp.intro, BusinessName: c.BusinessName, ServiceName: c.ServiceName,
		Date: capitalize(c.DateES()), Time: c.TimeES(), Professional: c.ProfessionalName, CustomerName: c.CustomerName,
		ForProfessional: m.Role == domain.RecipientProfessional, AppURL: appURL,
	}
	if s, ok := statuses[c.Status]; ok {
		v.Badge, v.BadgeColor, v.BadgeBackground = s[0], s[1], s[2]
	}
	if m.Kind == domain.KindCancelled {
		v.CancelReason = c.CancelReason
		if v.CancelReason == "" {
			v.CancelReason = "Sin motivo indicado"
		}
	}
	if !v.ForProfessional {
		v.Address, v.MapsURL = c.BusinessAddress, mapsURL(c)
		if c.BusinessPhone != "" {
			digits := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return r
				}
				return -1
			}, c.BusinessPhone)
			v.Phone = c.BusinessPhone
			v.PhoneURL = htmltemplate.URL("tel:+" + digits) //nolint:gosec // only digits after the scheme
			v.WhatsAppURL = "https://wa.me/" + digits
		}
		if m.Kind != domain.KindCancelled && m.Kind != domain.KindCancelledByCustomer {
			v.Policy = policy(c)
		}
	}
	var html, text bytes.Buffer
	if err := htmlTemplate.Execute(&html, v); err != nil {
		return Rendered{}, false, fmt.Errorf("email: html: %w", err)
	}
	if err := textTemplate.Execute(&text, v); err != nil {
		return Rendered{}, false, fmt.Errorf("email: text: %w", err)
	}
	return Rendered{Subject: oneLine(cp.subject), Text: text.String(), HTML: html.String()}, true, nil
}

func mapsURL(c domain.Content) string {
	q := c.BusinessName + ", " + c.BusinessAddress
	if c.Latitude != nil && c.Longitude != nil {
		q = strconv.FormatFloat(*c.Latitude, 'f', -1, 64) + "," + strconv.FormatFloat(*c.Longitude, 'f', -1, 64)
	} else if c.BusinessAddress == "" {
		return ""
	}
	return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(q)
}

func policy(c domain.Content) []string {
	line := func(verb string, h int) string {
		switch h {
		case 0:
			return "Puedes " + verb + " tu cita en cualquier momento antes de la hora." //nolint:misspell // Spanish
		case 1:
			return "Puedes " + verb + " tu cita hasta 1 hora antes."
		}
		return "Puedes " + verb + " tu cita hasta " + strconv.Itoa(h) + " horas antes."
	}
	return []string{line("cancelar", c.CancelCutoffHours), line("reprogramar", c.RescheduleCutoffHours)}
}

func firstName(full string) string {
	if f := strings.Fields(full); len(f) > 0 {
		return f[0]
	}
	return "hola"
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// oneLine keeps a header value on one line (header injection is also refused by the mailer).
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var htmlTemplate = htmltemplate.Must(htmltemplate.New("email").Parse(`<!doctype html>
<html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light"><title>{{.Title}}</title></head>
<body style="margin:0;padding:0;background:#F8FAFC;font-family:Arial,Helvetica,sans-serif;color:#111827;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#F8FAFC;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;">
<tr><td align="center" style="padding:0 0 16px;font-size:26px;font-weight:bold;color:#7C3AED;letter-spacing:-0.5px;">Booka</td></tr>
<tr><td style="background:#FFFFFF;border:1px solid #E2E8F0;border-radius:16px;overflow:hidden;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0">
<tr><td style="background:#7C3AED;padding:20px 24px;color:#FFFFFF;font-size:18px;font-weight:bold;">{{.BusinessName}}</td></tr>
<tr><td style="padding:28px 24px 8px;">
<h1 style="margin:0 0 12px;font-size:22px;line-height:1.3;color:#111827;">{{.Title}}</h1>
{{if .Badge}}<span style="display:inline-block;padding:4px 10px;border-radius:999px;font-size:12px;font-weight:bold;color:{{.BadgeColor}};background:{{.BadgeBackground}};">{{.Badge}}</span>{{end}}
<p style="margin:16px 0 0;font-size:15px;line-height:1.5;color:#334155;">{{.Intro}}</p>
</td></tr>
<tr><td style="padding:12px 24px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="font-size:15px;line-height:1.5;">
{{if .ForProfessional}}<tr><td width="32" valign="top">🙋</td><td style="padding:4px 0;"><b>{{.CustomerName}}</b></td></tr>{{end}}
<tr><td width="32" valign="top">✂️</td><td style="padding:4px 0;"><b>{{.ServiceName}}</b></td></tr>
<tr><td width="32" valign="top">📅</td><td style="padding:4px 0;">{{.Date}}</td></tr>
<tr><td width="32" valign="top">🕙</td><td style="padding:4px 0;">{{.Time}}</td></tr>
{{if and .Professional (not .ForProfessional)}}<tr><td width="32" valign="top">👤</td><td style="padding:4px 0;">Con {{.Professional}}</td></tr>{{end}}
{{if .Address}}<tr><td width="32" valign="top">📍</td><td style="padding:4px 0;">{{if .MapsURL}}<a href="{{.MapsURL}}" style="color:#6D28D9;">{{.Address}}</a>{{else}}{{.Address}}{{end}}</td></tr>{{end}}
{{if .CancelReason}}<tr><td width="32" valign="top">💬</td><td style="padding:4px 0;">Motivo: {{.CancelReason}}</td></tr>{{end}}
</table>
</td></tr>
{{if .Policy}}<tr><td style="padding:8px 24px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#F3E8FF;border-radius:12px;">
<tr><td style="padding:14px 16px;font-size:14px;line-height:1.5;color:#4C1D95;"><b>Políticas del negocio</b>{{range .Policy}}<br>{{.}}{{end}}</td></tr>
</table></td></tr>{{end}}
{{if .Phone}}<tr><td style="padding:12px 24px 4px;font-size:14px;line-height:1.8;">
📞 <a href="{{.PhoneURL}}" style="color:#6D28D9;">{{.Phone}}</a><br>
💬 <a href="{{.WhatsAppURL}}" style="color:#6D28D9;">Escríbenos por WhatsApp</a>
</td></tr>{{end}}
<tr><td style="padding:16px 24px 24px;"></td></tr>
</table></td></tr>
<tr><td align="center" style="padding:20px 12px;font-size:12px;line-height:1.6;color:#64748B;">
Agendado con <b style="color:#7C3AED;">Booka</b>{{if .AppURL}} · <a href="{{.AppURL}}" style="color:#6D28D9;">Descarga la app</a>{{end}}<br>
Recibes este correo por una cita en {{.BusinessName}}.
</td></tr>
</table>
</td></tr></table>
</body></html>
`))

var textTemplate = texttemplate.Must(texttemplate.New("email").Parse(`{{.BusinessName}}

{{.Title}}{{if .Badge}} ({{.Badge}}){{end}}

{{.Intro}}
{{if .ForProfessional}}
Cliente: {{.CustomerName}}{{end}}
Servicio: {{.ServiceName}}
Fecha: {{.Date}}
Hora: {{.Time}}{{if and .Professional (not .ForProfessional)}}
Con {{.Professional}}{{end}}{{if .Address}}
Dirección: {{.Address}}{{end}}{{if .MapsURL}}
Mapa: {{.MapsURL}}{{end}}{{if .CancelReason}}
Motivo: {{.CancelReason}}{{end}}
{{if .Policy}}
Políticas del negocio:{{range .Policy}}
- {{.}}{{end}}
{{end}}{{if .Phone}}
Teléfono: {{.Phone}}
WhatsApp: {{.WhatsAppURL}}
{{end}}
--
Agendado con Booka{{if .AppURL}} · Descarga la app: {{.AppURL}}{{end}}
Recibes este correo por una cita en {{.BusinessName}}.
`))
