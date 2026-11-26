package brand_test

import (
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/mail/brand"
)

func sample() brand.Email {
	return brand.Email{
		Title:      "Confirma tu correo",
		Greeting:   "Hola Ana:",
		Paragraphs: []string{"Gracias por registrarte."},
		Button:     &brand.Button{Label: "Confirmar mi correo", URL: "https://app.example.test/verify-email?token=bp_ev_abc&x=1"},
		Notes:      []string{"El enlace vence en 24 horas.", "Si no creaste esta cuenta, ignora este mensaje."},
		Reason:     "Recibes este correo porque se creó una cuenta con esta dirección.",
	}
}

func TestRenderBothVersions(t *testing.T) {
	text, html, err := brand.Render(sample())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Booka", "Confirma tu correo", "Hola Ana:", "Gracias por registrarte.", "Confirmar mi correo:\nhttps://app.example.test/verify-email?token=bp_ev_abc&x=1\n", "El enlace vence en 24 horas.", "Si no creaste esta cuenta", "Recibes este correo"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	for _, want := range []string{"<!doctype html>", "<title>Confirma tu correo</title>", `href="https://app.example.test/verify-email?token=bp_ev_abc&amp;x=1"`, ">Confirmar mi correo</a>", "Si el botón no funciona", "#7C3AED", "Si no creaste esta cuenta"} {
		if !strings.Contains(html, want) {
			t.Errorf("html lacks %q", want)
		}
	}
}

func TestRenderEscapes(t *testing.T) {
	e := sample()
	e.Greeting = `Hola <script>alert(1)</script>:`
	e.Paragraphs = []string{`<img src=x onerror=alert(1)>`}
	_, html, err := brand.Render(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>alert") || strings.Contains(html, "<img src=x") {
		t.Fatalf("not escaped:\n%s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatal("expected escaped markup")
	}
}

func TestRenderWithoutButton(t *testing.T) {
	e := sample()
	e.Button = nil
	text, html, err := brand.Render(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<a href") || strings.Contains(html, "Si el botón") || strings.Contains(text, "http") {
		t.Fatal("a notice without button shows a link")
	}
}

func TestRenderRejects(t *testing.T) {
	e := sample()
	e.Button.URL = "javascript:alert(1)"
	if _, _, err := brand.Render(e); err == nil {
		t.Error("javascript: link accepted")
	}
	if _, _, err := brand.Render(brand.Email{}); err == nil {
		t.Error("email without title accepted")
	}
}
