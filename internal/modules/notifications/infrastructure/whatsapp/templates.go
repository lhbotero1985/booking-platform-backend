package whatsapp

import (
	"strings"
	"unicode/utf8"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
)

// Template is a message template approved in the WhatsApp Business account, versioned here so
// the code and Meta agree on its name, language and the order of its body parameters
// (notifications.md §3). Texts approved by the user on 2026-11-25 (T7.3).
type Template struct {
	Name     string
	Language string
	// Params returns the body parameters {{1}}, {{2}}… in order.
	Params func(c domain.Content) []string
}

func (t Template) ref(c domain.Content) templateRef {
	r := templateRef{Name: t.Name, Language: language{Code: t.Language}}
	if t.Params == nil {
		return r
	}
	ps := t.Params(c)
	body := component{Type: "body", Parameters: make([]parameter, 0, len(ps))}
	for _, p := range ps {
		body.Parameters = append(body.Parameters, parameter{Type: "text", Text: param(p)})
	}
	r.Components = []component{body}
	return r
}

const spanish = "es"

// Templates by kind. cita_agendada_negocio also has a static "Descargar la app" URL button
// (https://<domain>/app, P40): static buttons take no parameter.
var templates = map[domain.Kind]Template{
	// Hola {{1}}, tu cita en {{2}} quedó agendada. Servicio: {{3}} Con: {{4}} Fecha: {{5}} …
	domain.KindCreated: {Name: "cita_agendada", Language: spanish, Params: func(c domain.Content) []string {
		return []string{c.CustomerName, c.BusinessName, c.ServiceName, c.ProfessionalName, c.WhenES()}
	}},
	// Hola {{1}}, {{2}} te agendó una cita. Servicio: {{3}} Con: {{4}} Fecha: {{5}} …
	domain.KindCreatedByBusiness: {Name: "cita_agendada_negocio", Language: spanish, Params: func(c domain.Content) []string {
		return []string{c.CustomerName, c.BusinessName, c.ServiceName, c.ProfessionalName, c.WhenES()}
	}},
	// Hola {{1}}, {{2}} canceló tu cita de {{3}} del {{4}}. Motivo: {{5}} …
	domain.KindCancelled: {Name: "cita_cancelada", Language: spanish, Params: func(c domain.Content) []string {
		return []string{c.CustomerName, c.BusinessName, c.ServiceName, c.WhenES(), orDefault(c.CancelReason, "Sin motivo indicado")}
	}},
	// Hola {{1}}, tu cita de {{2}} en {{3}} cambió de horario. Nueva fecha: {{4}} Con: {{5}} …
	domain.KindRescheduled: {Name: "cita_reprogramada", Language: spanish, Params: func(c domain.Content) []string {
		return []string{c.CustomerName, c.ServiceName, c.BusinessName, c.WhenES(), c.ProfessionalName}
	}},
	// Hola {{1}}, tu cita de {{2}} en {{3}} del {{4}} ahora será con {{5}}. …
	domain.KindReassigned: {Name: "cita_cambio_barbero", Language: spanish, Params: func(c domain.Content) []string {
		return []string{c.CustomerName, c.ServiceName, c.BusinessName, c.WhenES(), c.ProfessionalName}
	}},
}

func templateFor(k domain.Kind) (Template, bool) {
	t, ok := templates[k]
	return t, ok
}

// maxParam bounds one parameter (the body of a template is at most 1024 characters).
const maxParam = 120

// param makes a value acceptable to Meta: no line breaks or tabs, no runs of spaces, not empty,
// bounded.
func param(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxParam {
		r := []rune(s)
		s = strings.TrimSpace(string(r[:maxParam-1])) + "…"
	}
	return orDefault(s, "-")
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
