package application

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/mail/brand"
)

// Account emails with the Booka layout (P-6). Links point to the client app (PUBLIC_APP_URL);
// they are never put in the subject nor logged.

const (
	verificationEmailSubject    = "Confirma tu correo electrónico"
	passwordResetEmailSubject   = "Restablece tu contraseña"   //nolint:gosec // G101: email subject, not a credential
	passwordChangedEmailSubject = "Tu contraseña fue cambiada" //nolint:gosec // G101: email subject, not a credential
)

// accountTimeZone shows times in the password changed notice: accounts have no time zone and
// the platform starts in Colombia.
const accountTimeZone = "America/Bogota"

func (s *Service) sendBranded(ctx context.Context, to, subject string, e brand.Email) error {
	text, html, err := brand.Render(e)
	if err != nil {
		return err
	}
	return s.mailer.SendMessage(ctx, to, subject, text, html)
}

func verificationEmail(name, link string, ttl time.Duration) brand.Email {
	return brand.Email{
		Title:      "Confirma tu correo electrónico",
		Greeting:   greeting(name),
		Paragraphs: []string{"Gracias por registrarte en Booka. Para activar tu cuenta, confirma tu correo electrónico."},
		Button:     &brand.Button{Label: "Confirmar mi correo", URL: link},
		Notes: []string{
			"El enlace vence en " + plural(int(ttl.Hours()), "hora", "horas") + " y solo se puede usar una vez.",
			"Si no creaste esta cuenta, ignora este mensaje.",
		},
		Reason: "Recibes este correo porque se creó una cuenta en Booka con esta dirección.",
	}
}

func passwordResetEmail(name, link string, ttl time.Duration) brand.Email {
	return brand.Email{
		Title:      "Restablece tu contraseña",
		Greeting:   greeting(name),
		Paragraphs: []string{"Recibimos una solicitud para restablecer la contraseña de tu cuenta. Para elegir una nueva, usa este botón."},
		Button:     &brand.Button{Label: "Crear una nueva contraseña", URL: link},
		Notes: []string{
			"El enlace vence en " + plural(int(ttl.Minutes()), "minuto", "minutos") + " y solo se puede usar una vez. Al cambiar la contraseña se cerrarán todas tus sesiones.",
			"Si no solicitaste este cambio, ignora este mensaje: tu contraseña actual sigue siendo válida.",
		},
		Reason: "Recibes este correo porque se pidió restablecer la contraseña de tu cuenta en Booka.",
	}
}

func passwordChangedEmail(name string, at time.Time) brand.Email {
	return brand.Email{
		Title:      "Tu contraseña fue cambiada",
		Greeting:   greeting(name),
		Paragraphs: []string{"La contraseña de tu cuenta fue cambiada el " + spanishDateTime(at) + "."},
		Notes: []string{
			"Si fuiste tú, no necesitas hacer nada.",
			"Si no reconoces este cambio, restablece tu contraseña de inmediato con la opción «¿Olvidaste tu contraseña?» y contacta a soporte.",
		},
		Reason: "Recibes este aviso de seguridad por un cambio en tu cuenta de Booka.",
	}
}

func greeting(name string) string {
	if name = strings.TrimSpace(name); name == "" {
		return "Hola:"
	}
	return "Hola " + name + ":"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

var spanishMonths = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}

// spanishDateTime formats t as "26 de noviembre de 2026 a las 3:04 p. m. (hora de Colombia)";
// UTC when the zone database is missing.
func spanishDateTime(t time.Time) string {
	zone := " (hora de Colombia)"
	if loc, err := time.LoadLocation(accountTimeZone); err == nil {
		t = t.In(loc)
	} else {
		t, zone = t.UTC(), " (UTC)"
	}
	h, suffix := t.Hour(), "a. m."
	if h >= 12 {
		suffix = "p. m."
	}
	if h %= 12; h == 0 {
		h = 12
	}
	return fmt.Sprintf("%d de %s de %d a las %d:%02d %s%s", t.Day(), spanishMonths[t.Month()-1], t.Year(), h, t.Minute(), suffix, zone)
}
