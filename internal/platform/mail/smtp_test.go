package mail

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestBuildMessage(t *testing.T) {
	from, _ := mail.ParseAddress("Reservas <no-reply@booking.example>")
	to, _ := mail.ParseAddress("ana@example.com")
	msg, err := buildMessage(from, to, "Confirma tu correo electrónico", "Hola Ana:\nenlace: https://x/y?token=abc\n", "", time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	s := string(msg)
	for _, want := range []string{
		"From: \"Reservas\" <no-reply@booking.example>\r\n",
		"To: <ana@example.com>\r\n",
		"Subject: =?utf-8?q?Confirma_tu_correo_electr=C3=B3nico?=\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"Content-Transfer-Encoding: quoted-printable\r\n",
		"@booking.example>\r\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("message missing %q:\n%s", want, s)
		}
	}
	_, body, _ := strings.Cut(s, "\r\n\r\n")
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if err != nil || !strings.Contains(string(decoded), "token=abc") {
		t.Errorf("body = %q, %v", decoded, err)
	}
}

// TestBuildHTMLMessage (T7.6): text and HTML as multipart/alternative, the HTML last.
func TestBuildHTMLMessage(t *testing.T) {
	from, _ := mail.ParseAddress("Booka <no-reply@booking.example>")
	to, _ := mail.ParseAddress("ana@example.com")
	msg, err := buildMessage(from, to, "Tu cita está agendada", "Hola Ana: tu cita", "<p>Hola Ana: <b>tu cita</b> ✂</p>", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mail.ReadMessage(strings.NewReader(string(msg)))
	if err != nil {
		t.Fatal(err)
	}
	media, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || media != "multipart/alternative" {
		t.Fatalf("content type %q: %v", media, err)
	}
	r := multipart.NewReader(parsed.Body, params["boundary"])
	var types, bodies []string
	for {
		p, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p) // multipart decodes quoted-printable
		types, bodies = append(types, p.Header.Get("Content-Type")), append(bodies, string(b))
	}
	if len(types) != 2 || types[0] != "text/plain; charset=UTF-8" || types[1] != "text/html; charset=UTF-8" ||
		bodies[0] != "Hola Ana: tu cita" || bodies[1] != "<p>Hola Ana: <b>tu cita</b> ✂</p>" {
		t.Errorf("parts %q %q", types, bodies)
	}
}

func TestBuildMessageRejectsHeaderInjection(t *testing.T) {
	from, _ := mail.ParseAddress("no-reply@booking.example")
	to, _ := mail.ParseAddress("ana@example.com")
	_, err := buildMessage(from, to, "hola\r\nBcc: victim@example.com", "x", "", time.Now())
	if !errors.Is(err, ErrUnsafeHeader) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewSMTPValidation(t *testing.T) {
	if _, err := NewSMTP(Config{Host: "h", Port: 25, From: "not an address", TLSMode: TLSNone}); err == nil {
		t.Error("invalid from accepted")
	}
	if _, err := NewSMTP(Config{Host: "h", Port: 25, From: "a@b.co", TLSMode: "ssl3"}); err == nil {
		t.Error("unknown TLS mode accepted")
	}
	s, err := NewSMTP(Config{Host: "h", Port: 25, From: "a@b.co", TLSMode: TLSStartTLS})
	if err != nil || s.cfg.Timeout != 15*time.Second {
		t.Errorf("defaults: %v %+v", err, s)
	}
}
