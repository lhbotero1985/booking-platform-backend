package simulated_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/simulated"
)

func TestSimulatedSenderLogsWithoutTheFullNumber(t *testing.T) {
	var buf bytes.Buffer
	s := simulated.New(slog.New(slog.NewJSONHandler(&buf, nil)))
	id := uuid.Must(uuid.NewV7())
	got, err := s.Send(context.Background(), application.Outgoing{MessageID: id, Kind: domain.KindCreated, Channel: domain.ChannelWhatsApp, To: "+573001234567"})
	if err != nil || got != "simulated-"+id.String() {
		t.Fatalf("%q %v", got, err)
	}
	log := buf.String()
	if strings.Contains(log, "3001234567") || !strings.Contains(log, "4567") || !strings.Contains(log, "APPOINTMENT_CREATED") {
		t.Errorf("log = %s", log)
	}
}

func TestSimulatedReplyLogsWithoutTheText(t *testing.T) {
	var buf bytes.Buffer
	s := simulated.New(slog.New(slog.NewJSONHandler(&buf, nil)))
	if id, err := s.SendText(context.Background(), "+573001234567", "¡Listo, Laura!"); err != nil || id == "" {
		t.Fatalf("%q %v", id, err)
	}
	if log := buf.String(); strings.Contains(log, "3001234567") || strings.Contains(log, "Laura") || !strings.Contains(log, "reply simulated") {
		t.Errorf("log = %s", log)
	}
}
