// Package brand renders account emails (verification, password reset, password changed) with
// the Booka layout: logo, white card, one primary button and a footer, as HTML and as plain text
// for the same message (multipart/alternative). Every value is escaped by html/template; a link
// that is not http(s) is neutralized by it.
//
// The appointment emails of the notifications module keep their own richer template with the
// same colors; both can converge on this package later (P-6, decision A).
package brand

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
)

// Email is the content of one account email.
type Email struct {
	// Title is the heading and the HTML <title>.
	Title string
	// Greeting opens the message ("Hola Ana:").
	Greeting string
	// Paragraphs are shown in order before the button.
	Paragraphs []string
	// Button is the main action; nil for a notice without a link.
	Button *Button
	// Notes are shown in small print after the button (validity, "ignore this message"…).
	Notes []string
	// Reason explains in the footer why the recipient gets this email.
	Reason string
}

// Button is a call to action. URL must be an absolute http(s) link.
type Button struct {
	Label string
	URL   string
}

// Render returns the plain-text and HTML versions of e.
func Render(e Email) (text, html string, err error) {
	if e.Title == "" {
		return "", "", fmt.Errorf("brand: email without title")
	}
	if e.Button != nil && !strings.HasPrefix(e.Button.URL, "https://") && !strings.HasPrefix(e.Button.URL, "http://") {
		return "", "", fmt.Errorf("brand: button link must be http(s)")
	}
	var h, t bytes.Buffer
	if err := htmlTemplate.Execute(&h, e); err != nil {
		return "", "", fmt.Errorf("brand: html: %w", err)
	}
	if err := textTemplate.Execute(&t, e); err != nil {
		return "", "", fmt.Errorf("brand: text: %w", err)
	}
	return t.String(), h.String(), nil
}

var htmlTemplate = htmltemplate.Must(htmltemplate.New("brand").Parse(`<!doctype html>
<html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light"><title>{{.Title}}</title></head>
<body style="margin:0;padding:0;background:#F8FAFC;font-family:Arial,Helvetica,sans-serif;color:#111827;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#F8FAFC;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;">
<tr><td align="center" style="padding:0 0 16px;font-size:26px;font-weight:bold;color:#7C3AED;letter-spacing:-0.5px;">Booka</td></tr>
<tr><td style="background:#FFFFFF;border:1px solid #E2E8F0;border-radius:16px;overflow:hidden;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0">
<tr><td style="background:#7C3AED;height:6px;font-size:0;line-height:0;">&nbsp;</td></tr>
<tr><td style="padding:28px 24px 8px;">
<h1 style="margin:0 0 16px;font-size:22px;line-height:1.3;color:#111827;">{{.Title}}</h1>
{{if .Greeting}}<p style="margin:0 0 12px;font-size:15px;line-height:1.5;color:#334155;">{{.Greeting}}</p>{{end}}
{{range .Paragraphs}}<p style="margin:0 0 12px;font-size:15px;line-height:1.5;color:#334155;">{{.}}</p>{{end}}
</td></tr>
{{with .Button}}<tr><td style="padding:8px 24px 4px;">
<a href="{{.URL}}" style="display:inline-block;padding:12px 20px;border-radius:10px;font-size:15px;font-weight:bold;text-decoration:none;background:#7C3AED;color:#FFFFFF;border:1px solid #7C3AED;">{{.Label}}</a>
<p style="margin:16px 0 0;font-size:12px;line-height:1.5;color:#64748B;">Si el botón no funciona, copia este enlace en tu navegador:<br>
<a href="{{.URL}}" style="color:#6D28D9;word-break:break-all;">{{.URL}}</a></p>
</td></tr>{{end}}
{{if .Notes}}<tr><td style="padding:12px 24px 0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#F3E8FF;border-radius:12px;">
<tr><td style="padding:14px 16px;font-size:13px;line-height:1.5;color:#4C1D95;">{{range $i, $n := .Notes}}{{if $i}}<br>{{end}}{{$n}}{{end}}</td></tr>
</table></td></tr>{{end}}
<tr><td style="padding:16px 24px 24px;"></td></tr>
</table></td></tr>
<tr><td align="center" style="padding:20px 12px;font-size:12px;line-height:1.6;color:#64748B;">
<b style="color:#7C3AED;">Booka</b> · Reservas para tu negocio<br>
{{.Reason}}
</td></tr>
</table>
</td></tr></table>
</body></html>
`))

var textTemplate = texttemplate.Must(texttemplate.New("brand").Parse(`Booka

{{.Title}}
{{if .Greeting}}
{{.Greeting}}
{{end}}{{range .Paragraphs}}
{{.}}
{{end}}{{with .Button}}
{{.Label}}:
{{.URL}}
{{end}}{{range .Notes}}
{{.}}
{{end}}
--
Booka · Reservas para tu negocio
{{.Reason}}
`))
