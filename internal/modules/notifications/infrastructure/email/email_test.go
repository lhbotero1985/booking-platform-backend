package email

import (
	"context"
	"errors"
	"html"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/mail"
)

func content() domain.Content {
	bogota, _ := time.LoadLocation("America/Bogota")
	lat, lng := 6.2442, -75.5812
	return domain.Content{
		CustomerName: "Laura Gómez", BusinessName: "Barbería El Paisa", ServiceName: "Corte de cabello",
		ProfessionalName: "Andrés Botero", Start: time.Date(2026, 12, 2, 15, 0, 0, 0, time.UTC), Location: bogota,
		Status: "PENDING_CONFIRMATION", BusinessAddress: "Calle 10 # 43-12, Medellín", BusinessPhone: "+573001234567",
		Latitude: &lat, Longitude: &lng, CancelCutoffHours: 2, RescheduleCutoffHours: 4,
	}
}

func out(kind domain.Kind, role domain.Role, c domain.Content) application.Outgoing {
	return application.Outgoing{MessageID: uuid.Must(uuid.NewV7()), Kind: kind, Channel: domain.ChannelEmail, Role: role, To: "laura@example.com", Content: c}
}

func render(t *testing.T, kind domain.Kind, role domain.Role, c domain.Content) Rendered {
	t.Helper()
	r, ok, err := Render(out(kind, role, c), Config{AppDownloadURL: "https://booka.example/app"})
	if err != nil || !ok {
		t.Fatalf("%s to %s: %v %v", kind, role, ok, err)
	}
	return r
}

// TestRenderEveryEmail: the approved subjects and the data of the appointment, in HTML and text.
func TestRenderEveryEmail(t *testing.T) {
	c := content()
	cases := []struct {
		kind    domain.Kind
		role    domain.Role
		subject string
		has     []string
		hasNot  []string
	}{
		{domain.KindCreated, domain.RecipientCustomer, "Tu cita en Barbería El Paisa está agendada",
			[]string{"¡Tu cita está agendada!", "Hola Laura,", "Pendiente de confirmar", "Con Andrés Botero", "Miércoles 2 de diciembre de 2026", "10:00 a. m.",
				"Puedes cancelar tu cita hasta 2 horas antes.", "Puedes reprogramar tu cita hasta 4 horas antes.", "https://wa.me/573001234567", "+573001234567",
				"query=6.2442%2C-75.5812", "Agendado con", "Descarga la app"}, []string{"Motivo"}},
		{domain.KindCreatedByBusiness, domain.RecipientCustomer, "Tu cita en Barbería El Paisa está agendada",
			[]string{"Barbería El Paisa te agendó esta cita"}, nil},
		{domain.KindCancelled, domain.RecipientCustomer, "Tu cita en Barbería El Paisa fue cancelada",
			[]string{"canceló tu cita", "Motivo: Sin motivo indicado"}, []string{"Políticas del negocio"}},
		{domain.KindCancelledByCustomer, domain.RecipientCustomer, "Tu cita en Barbería El Paisa fue cancelada",
			[]string{"cancelaste esta cita"}, []string{"Motivo", "Políticas del negocio"}},
		{domain.KindRescheduled, domain.RecipientCustomer, "Tu cita en Barbería El Paisa cambió de hora", []string{"tu cita ahora es"}, nil},
		{domain.KindReassigned, domain.RecipientCustomer, "Tu cita en Barbería El Paisa tiene otro especialista", []string{"ahora te atenderá Andrés Botero"}, nil},
		{domain.KindReminder, domain.RecipientCustomer, "Recordatorio: tu cita en Barbería El Paisa es el miércoles 2 de diciembre, 10:00 a. m.",
			[]string{"Te esperamos"}, nil},
		{domain.KindCreated, domain.RecipientProfessional, "Nueva cita: Laura Gómez, miércoles 2 de diciembre, 10:00 a. m.",
			[]string{"Hola Andrés, Laura Gómez agendó una cita contigo"}, []string{"Políticas del negocio", "wa.me", "Calle 10", "Con Andrés"}},
		{domain.KindCancelledByCustomer, domain.RecipientProfessional, "Cita cancelada: Laura Gómez, miércoles 2 de diciembre, 10:00 a. m.",
			[]string{"Ese espacio quedó libre"}, []string{"wa.me"}},
		{domain.KindRescheduled, domain.RecipientProfessional, "Cita reprogramada: Laura Gómez, miércoles 2 de diciembre, 10:00 a. m.",
			[]string{"movió su cita contigo"}, []string{"wa.me"}},
	}
	for _, tc := range cases {
		r := render(t, tc.kind, tc.role, c)
		if r.Subject != tc.subject {
			t.Errorf("%s/%s subject %q", tc.kind, tc.role, r.Subject)
		}
		for _, body := range []string{html.UnescapeString(r.HTML), r.Text} {
			for _, s := range tc.has {
				if !strings.Contains(strings.ToLower(body), strings.ToLower(s)) {
					t.Errorf("%s/%s: missing %q", tc.kind, tc.role, s)
				}
			}
			for _, s := range tc.hasNot {
				if strings.Contains(body, s) {
					t.Errorf("%s/%s: unexpected %q", tc.kind, tc.role, s)
				}
			}
		}
	}
	// Kinds with no email for that recipient.
	for _, k := range []domain.Kind{domain.KindReminder, domain.KindReassigned, domain.KindCancelled, domain.KindCreatedByBusiness} {
		if _, ok, _ := Render(out(k, domain.RecipientProfessional, c), Config{}); ok {
			t.Errorf("%s to the professional rendered", k)
		}
	}
}

// TestRenderEscapes: what customers and businesses type cannot inject markup or headers.
func TestRenderEscapes(t *testing.T) {
	c := content()
	c.CustomerName = `<script>alert(1)</script> "Ana"`
	c.BusinessName = "Barbería\r\nBcc: victim@example.com"
	c.BusinessAddress = `<img src=x onerror=alert(1)>`
	c.Latitude, c.Longitude = nil, nil
	r := render(t, domain.KindCreated, domain.RecipientCustomer, c)
	for _, bad := range []string{"<script>", "<img", "onerror=alert(1)>"} {
		if strings.Contains(r.HTML, bad) {
			t.Errorf("html contains %q", bad)
		}
	}
	if strings.ContainsAny(r.Subject, "\r\n") {
		t.Errorf("subject %q", r.Subject)
	}
	if !strings.Contains(r.HTML, "query=Barber") { // no coordinates: the address
		t.Error("maps link by address missing")
	}
	c.BusinessAddress, c.BusinessPhone, c.Status = "", "", "UNKNOWN"
	r = render(t, domain.KindCreated, domain.RecipientCustomer, c)
	for _, s := range []string{"maps", "wa.me", "Pendiente"} {
		if strings.Contains(r.HTML, s) {
			t.Errorf("without address/phone/status: %q present", s)
		}
	}
}

func TestPolicyLines(t *testing.T) {
	c := content()
	c.CancelCutoffHours, c.RescheduleCutoffHours = 0, 1
	got := policy(c)
	anytime := "Puedes cancelar tu cita en cualquier momento antes de la hora." //nolint:misspell // Spanish
	if len(got) != 2 || got[0] != anytime || got[1] != "Puedes reprogramar tu cita hasta 1 hora antes." {
		t.Errorf("%q", got)
	}
	c.MaxReschedules, c.Reschedules = 3, 1 // T7.8
	if got := policy(c); len(got) != 3 || got[2] != "Te quedan 2 cambios de hora para esta cita (máximo 3 veces)." {
		t.Errorf("with a limit: %q", got)
	}
	if firstName("") != "hola" || capitalize("") != "" {
		t.Error("empty names")
	}
}

type fakeMailer struct {
	to, subject, text, html string
	err                     error
}

func (f *fakeMailer) SendMessage(_ context.Context, to, subject, text, html string) error {
	f.to, f.subject, f.text, f.html = to, subject, text, html
	return f.err
}

func TestSender(t *testing.T) {
	ctx := context.Background()
	m := &fakeMailer{}
	s := New(m, Config{})
	if id, err := s.Send(ctx, out(domain.KindCreated, domain.RecipientCustomer, content())); err != nil || id != "" ||
		m.to != "laura@example.com" || !strings.Contains(m.subject, "agendada") || m.text == "" || !strings.Contains(m.html, "<html") {
		t.Fatalf("send: %v %+v", err, m)
	}
	if strings.Contains(m.html, "Descarga la app") {
		t.Error("app link without a URL")
	}
	cases := []struct {
		err       error
		code      string
		permanent bool
	}{
		{mail.ErrInvalidRecipient, "INVALID_EMAIL", true},
		{&textproto.Error{Code: 550, Msg: "no such user"}, "SMTP_550", true},
		{&textproto.Error{Code: 421, Msg: "try later"}, "SMTP_ERROR", false},
		{errors.New("dial tcp: timeout"), "SMTP_ERROR", false},
	}
	for _, c := range cases {
		m.err = c.err
		_, err := s.Send(ctx, out(domain.KindCreated, domain.RecipientCustomer, content()))
		var de *application.DeliveryError
		if !errors.As(err, &de) || de.Code != c.code || de.Permanent != c.permanent {
			t.Errorf("%v: %+v", c.err, err)
		}
	}
	_, err := s.Send(ctx, out(domain.KindReminder, domain.RecipientProfessional, content()))
	var de *application.DeliveryError
	if !errors.As(err, &de) || de.Code != "NO_TEMPLATE" || !de.Permanent {
		t.Errorf("no template: %v", err)
	}
}

// TestPreview writes every email to EMAIL_PREVIEW_DIR, to look at them in a browser.
func TestPreview(t *testing.T) {
	dir := os.Getenv("EMAIL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("EMAIL_PREVIEW_DIR not set")
	}
	for _, k := range []domain.Kind{domain.KindCreated, domain.KindCreatedByBusiness, domain.KindCancelled, domain.KindCancelledByCustomer,
		domain.KindRescheduled, domain.KindReassigned, domain.KindReminder} {
		for _, role := range []domain.Role{domain.RecipientCustomer, domain.RecipientProfessional} {
			r, ok, err := Render(withLinks(out(k, role, content())), previewConfig)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				name := filepath.Join(dir, strings.ToLower(string(k)+"-"+string(role))+".html")
				if err := os.WriteFile(name, []byte(r.HTML), 0o600); err != nil { //nolint:gosec // a directory chosen by the developer

					t.Fatal(err)
				}
			}
		}
	}
}

var previewConfig = Config{AppDownloadURL: "https://booka.example/app", LinksBaseURL: "https://api.booka.example/"}

// withLinks gives a customer's email about an active appointment its three links.
func withLinks(m application.Outgoing) application.Outgoing {
	if m.Role == domain.RecipientCustomer && m.Kind != domain.KindCancelled && m.Kind != domain.KindCancelledByCustomer {
		m.Links = map[domain.Action]string{domain.ActionConfirm: "c0nf+rm", domain.ActionReschedule: "r3sch", domain.ActionCancel: "c4nc"}
	}
	return m
}

// TestRenderButtons (T7.7): the links open the API's pages (two taps); the reschedule link goes to
// the frontend when configured; without a base URL there are no buttons.
func TestRenderButtons(t *testing.T) {
	m := withLinks(out(domain.KindReminder, domain.RecipientCustomer, content()))
	r, _, err := Render(m, previewConfig)
	if err != nil {
		t.Fatal(err)
	}
	h := html.UnescapeString(r.HTML)
	for _, want := range []string{
		"https://api.booka.example/citas/confirmar?token=c0nf%2Brm", "https://api.booka.example/citas/reagendar?token=r3sch",
		"https://api.booka.example/citas/cancelar?token=c4nc", ">Confirmar cita<", ">Reagendar<", ">Cancelar cita<",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("html without %q", want)
		}
	}
	if !strings.Contains(r.Text, "Confirmar tu cita: https://api.booka.example/citas/confirmar?token=c0nf%2Brm") {
		t.Errorf("text:\n%s", r.Text)
	}
	if i, j := strings.Index(h, "Confirmar cita"), strings.Index(h, "Cancelar cita"); i < 0 || j < i {
		t.Error("order of the buttons")
	}
	cfg := previewConfig
	cfg.RescheduleURL = "https://app.booka.example/reagendar"
	r, _, _ = Render(m, cfg)
	if !strings.Contains(html.UnescapeString(r.HTML), "https://app.booka.example/reagendar?token=r3sch") {
		t.Error("frontend reschedule page")
	}
	delete(m.Links, domain.ActionConfirm)
	if r, _, _ = Render(m, previewConfig); strings.Contains(r.HTML, "Confirmar cita<") {
		t.Error("confirm button without its link")
	}
	if r, _, _ = Render(m, Config{}); strings.Contains(r.HTML, "citas/") {
		t.Error("buttons without a base URL")
	}
}

// TestRenderReviewRequest (2026-11-28): "Calificar" to the frontend's page with the appointment;
// without that page, the text points to the app. No policies, nothing to the professional.
func TestRenderReviewRequest(t *testing.T) {
	c := content()
	c.Status = "COMPLETED"
	m := out(domain.KindReviewRequest, domain.RecipientCustomer, c)
	m.AppointmentID = uuid.MustParse("01900000-0000-7000-8000-0000000000aa")
	r, ok, err := Render(m, Config{ReviewURL: "https://app.booka.example/citas/calificar"})
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	h := html.UnescapeString(r.HTML)
	if r.Subject != "¿Cómo te fue en Barbería El Paisa?" {
		t.Errorf("subject %q", r.Subject)
	}
	for _, want := range []string{"Cuéntanos cómo te fue", "Hola Laura,", "Atendida", ">Calificar<",
		"https://app.booka.example/citas/calificar?appointmentId=01900000-0000-7000-8000-0000000000aa", "Tienes 30 días"} {
		if !strings.Contains(h, want) {
			t.Errorf("html without %q", want)
		}
	}
	if strings.Contains(h, "Políticas del negocio") || strings.Contains(h, "confirmas el cambio") {
		t.Error("policies or the links' note in a review request")
	}
	if !strings.Contains(r.Text, "Calificar tu cita: https://app.booka.example/citas/calificar?appointmentId=") {
		t.Errorf("text:\n%s", r.Text)
	}
	r, _, _ = Render(m, Config{})
	if strings.Contains(r.HTML, "Calificar<") || !strings.Contains(html.UnescapeString(r.HTML), "«Mis citas» de la app") {
		t.Error("without the review page")
	}
	if _, ok, _ := Render(out(domain.KindReviewRequest, domain.RecipientProfessional, c), Config{}); ok {
		t.Error("review request to the professional")
	}
}
