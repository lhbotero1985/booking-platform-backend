package email

import (
	"context"
	"errors"
	"net/textproto"
	"strconv"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/mail"
)

// Mailer delivers one email (mail.SMTP).
type Mailer interface {
	SendMessage(ctx context.Context, to, subject, text, html string) error
}

// Config of the sender.
type Config struct {
	// AppDownloadURL, when set, adds "Descarga la app" to the footer.
	AppDownloadURL string
}

// Sender implements application.Sender for the EMAIL channel.
type Sender struct {
	mailer Mailer
	cfg    Config
}

// New builds the sender.
func New(m Mailer, cfg Config) *Sender { return &Sender{mailer: m, cfg: cfg} }

// Send renders and delivers the email; SMTP gives no message id, so it returns "". A malformed
// address or a 5xx answer of the server is permanent; anything else is retried.
func (s *Sender) Send(ctx context.Context, m application.Outgoing) (string, error) {
	r, ok, err := Render(m, s.cfg.AppDownloadURL)
	if err != nil {
		return "", &application.DeliveryError{Code: "RENDER_FAILED", Permanent: true, Err: err}
	}
	if !ok {
		return "", &application.DeliveryError{Code: "NO_TEMPLATE", Permanent: true, Err: errors.New("email: no email for " + string(m.Kind) + " to " + string(m.Role))}
	}
	if err := s.mailer.SendMessage(ctx, m.To, r.Subject, r.Text, r.HTML); err != nil {
		return "", classify(err)
	}
	return "", nil
}

func classify(err error) *application.DeliveryError {
	var tp *textproto.Error
	switch {
	case errors.Is(err, mail.ErrInvalidRecipient), errors.Is(err, mail.ErrUnsafeHeader):
		return &application.DeliveryError{Code: "INVALID_EMAIL", Permanent: true, Err: err}
	case errors.As(err, &tp) && tp.Code >= 500:
		return &application.DeliveryError{Code: "SMTP_" + strconv.Itoa(tp.Code), Permanent: true, Err: err}
	}
	return &application.DeliveryError{Code: "SMTP_ERROR", Err: err}
}
