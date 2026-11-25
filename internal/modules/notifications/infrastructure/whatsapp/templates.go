package whatsapp

import "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"

// Template is a message template approved in the WhatsApp Business account, versioned here so
// the code and Meta agree on its name, language and parameters (notifications.md §3).
type Template struct {
	Name     string
	Language string
}

func (t Template) ref() templateRef {
	return templateRef{Name: t.Name, Language: language{Code: t.Language}}
}

// helloWorld is the template every WhatsApp Business account has, approved by Meta. T7.2 sends
// it for every kind to prove the connection end to end; T7.3 replaces it with the templates of
// the appointment (with the business, service, date and time as parameters) once approved.
var helloWorld = Template{Name: "hello_world", Language: "en_US"}

// templates maps each kind of message to its template.
var templates = map[domain.Kind]Template{
	domain.KindCreated:     helloWorld,
	domain.KindCancelled:   helloWorld,
	domain.KindRescheduled: helloWorld,
	domain.KindReassigned:  helloWorld,
}

func templateFor(k domain.Kind) (Template, bool) {
	t, ok := templates[k]
	return t, ok
}
