package application_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	_ "time/tzdata" // as in cmd/api and cmd/worker
)

// TestAccountEmailsAreBranded (P-6): the three account emails carry the Booka HTML layout and its
// plain-text version, link to the client app (PUBLIC_APP_URL) and never put the token in the
// subject; the password changed notice has no link and shows Colombian time.
func TestAccountEmailsAreBranded(t *testing.T) {
	e := newEnv()
	e.register(t, "ana@example.com")
	raw := e.sendAndExtract(t)
	verify := e.mailer.Last()

	e.verifiedUser(t, "bob@example.com")
	_ = e.svc.ForgotPassword(context.Background(), "bob@example.com")
	rawReset := e.resetToken(t)
	reset := e.mailer.Last()

	for name, c := range map[string]struct {
		subject, button, path, token, note string
	}{
		"verification": {"Confirma tu correo electrónico", "Confirmar mi correo", "https://app.example.test/verify-email?token=", raw, "24 horas"},
		"reset":        {"Restablece tu contraseña", "Crear una nueva contraseña", "https://app.example.test/reset-password?token=", rawReset, "30 minutos"},
	} {
		m := map[string]struct{ Subject, Body, HTML string }{
			"verification": {verify.Subject, verify.Body, verify.HTML},
			"reset":        {reset.Subject, reset.Body, reset.HTML},
		}[name]
		link := c.path + url.QueryEscape(c.token)
		if m.Subject != c.subject || strings.Contains(m.Subject, c.token) {
			t.Errorf("%s subject %q", name, m.Subject)
		}
		if !strings.Contains(m.Body, link+"\n") || !strings.Contains(m.Body, c.note) || !strings.HasPrefix(m.Body, "Booka\n") {
			t.Errorf("%s text:\n%s", name, m.Body)
		}
		for _, want := range []string{"<!doctype html>", ">" + c.button + "</a>", `href="` + link + `"`, c.note, "Hola Ana Pérez:"} {
			if !strings.Contains(m.HTML, want) {
				t.Errorf("%s html lacks %q", name, want)
			}
		}
		if strings.Contains(m.HTML, "nolint") || strings.Contains(m.Body, "nolint") {
			t.Errorf("%s: lint directive leaked", name)
		}
	}

	if err := e.svc.SendPasswordChangedEmail(context.Background(), e.store.User("bob@example.com").ID()); err != nil {
		t.Fatal(err)
	}
	changed := e.mailer.Last()
	// The fixed clock is 2026-11-19 12:00 UTC: 7:00 a. m. in Bogotá.
	when := "19 de noviembre de 2026 a las 7:00 a. m. (hora de Colombia)"
	if changed.Subject != "Tu contraseña fue cambiada" || !strings.Contains(changed.Body, when) || !strings.Contains(changed.HTML, when) {
		t.Errorf("changed notice:\n%s", changed.Body)
	}
	if strings.Contains(changed.Body, "http") || strings.Contains(changed.HTML, "<a href") || strings.Contains(changed.Body, "token") {
		t.Errorf("the changed notice carries a link:\n%s", changed.Body)
	}
}
