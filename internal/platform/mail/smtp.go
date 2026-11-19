// Package mail sends transactional email over SMTP (any provider: Amazon SES, Resend, Postmark,
// … or Mailpit in DEV). Plain text, UTF-8, quoted-printable.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// TLS modes.
const (
	TLSNone     = "none"     // DEV only (Mailpit)
	TLSStartTLS = "starttls" // port 587
	TLSImplicit = "tls"      // port 465
)

// Config configures the SMTP sender.
type Config struct {
	Host     string
	Port     int
	From     string
	Username string
	Password string
	TLSMode  string
	Timeout  time.Duration
}

// ErrUnsafeHeader is returned when a header value could inject extra headers.
var ErrUnsafeHeader = errors.New("mail: unsafe header value")

// SMTP sends messages with net/smtp.
type SMTP struct {
	cfg  Config
	from *mail.Address
}

// NewSMTP validates cfg.
func NewSMTP(cfg Config) (*SMTP, error) {
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mail: invalid from address: %w", err)
	}
	switch cfg.TLSMode {
	case TLSNone, TLSStartTLS, TLSImplicit:
	default:
		return nil, fmt.Errorf("mail: unknown TLS mode %q", cfg.TLSMode)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &SMTP{cfg: cfg, from: from}, nil
}

// Send delivers a plain-text email to one recipient.
func (s *SMTP) Send(ctx context.Context, to, subject, body string) error {
	rcpt, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("mail: invalid recipient: %w", err)
	}
	msg, err := buildMessage(s.from, rcpt, subject, body, time.Now())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	dialer := &net.Dialer{}
	var conn net.Conn
	if s.cfg.TLSMode == TLSImplicit {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: dial: %w", err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: client: %w", err)
	}
	defer c.Close()

	if s.cfg.TLSMode == TLSStartTLS {
		if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(rcpt.Address); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mail: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: close data: %w", err)
	}
	return c.Quit()
}

// buildMessage renders RFC 5322 headers and a quoted-printable UTF-8 body. Header values with
// CR/LF are rejected (header injection).
func buildMessage(from, to *mail.Address, subject, body string, now time.Time) ([]byte, error) {
	if strings.ContainsAny(subject, "\r\n") || strings.ContainsAny(to.Address, "\r\n") || strings.ContainsAny(from.Address, "\r\n") {
		return nil, ErrUnsafeHeader
	}
	idBytes := make([]byte, 12)
	_, _ = rand.Read(idBytes)
	_, domain, _ := strings.Cut(from.Address, "@")

	var buf bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }
	header("From", from.String())
	header("To", to.String())
	header("Subject", mime.QEncoding.Encode("utf-8", subject))
	header("Date", now.UTC().Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(idBytes)+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=UTF-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	buf.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&buf)
	if _, err := qp.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n"))); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
