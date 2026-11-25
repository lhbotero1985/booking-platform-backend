// Package simulated is the delivery used while no provider is configured (T7.1, and DEV/QA until
// the Meta credentials exist): nothing leaves the server, the delivery is logged with the
// number masked and gets a synthetic provider id.
package simulated

import (
	"context"
	"log/slog"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
)

// Sender logs instead of sending.
type Sender struct{ logger *slog.Logger }

// New builds the sender.
func New(logger *slog.Logger) *Sender { return &Sender{logger: logger} }

// Send implements application.Sender.
func (s *Sender) Send(ctx context.Context, m application.Outgoing) (string, error) {
	s.logger.InfoContext(ctx, "notification simulated",
		slog.String("message_id", m.MessageID.String()),
		slog.String("kind", string(m.Kind)),
		slog.String("channel", string(m.Channel)),
		slog.String("to", domain.MaskPhone(m.To)),
	)
	return "simulated-" + m.MessageID.String(), nil
}
