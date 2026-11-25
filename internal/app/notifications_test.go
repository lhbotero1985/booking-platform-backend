package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/simulated"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/whatsapp"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
)

func TestWhatsAppSenderChoice(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	if _, ok := WhatsAppSender(config.Notifications{WhatsAppEnabled: true}, logger).(*simulated.Sender); !ok {
		t.Error("without credentials: simulated")
	}
	const token = "EAAsecretTOKEN1234567890"
	cfg := config.Notifications{
		WhatsAppEnabled: true, WhatsAppAccessToken: token, WhatsAppPhoneNumberID: "106540352242922",
		WhatsAppAPIBaseURL: "https://graph.facebook.com", WhatsAppGraphVersion: "v23.0", WhatsAppAllowlist: []string{"+573001234567"},
	}
	if _, ok := WhatsAppSender(cfg, logger).(*whatsapp.Client); !ok {
		t.Error("with credentials: Meta")
	}
	log := buf.String()
	if !strings.Contains(log, `"provider":"meta"`) || !strings.Contains(log, `"provider":"simulated"`) || strings.Contains(log, token) ||
		strings.Contains(log, "3001234567") {
		t.Errorf("log = %s", log)
	}
}
