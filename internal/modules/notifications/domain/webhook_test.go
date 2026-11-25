package domain_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
)

func TestActionTokens(t *testing.T) {
	a, ha, err := domain.NewActionToken(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := domain.NewActionToken(rand.Reader)
	if len(a) != 43 || a == b || strings.ContainsAny(a, "+/=") || !bytes.Equal(ha, domain.HashActionToken(a)) || len(ha) != 32 {
		t.Errorf("token %q hash %x", a, ha)
	}
	if bytes.Equal(domain.HashActionToken(a), domain.HashActionToken(b)) {
		t.Error("different tokens, same hash")
	}
	if _, _, err := domain.NewActionToken(iotest.ErrReader(errors.New("no entropy"))); err == nil {
		t.Error("a failing source must fail")
	}
}

func TestReplyTexts(t *testing.T) {
	c := domain.Content{CustomerName: "Laura", BusinessName: "Barbería El Paisa", ServiceName: "Corte", Start: start}
	for o, want := range map[domain.Outcome]string{
		domain.OutcomeConfirmed: "¡Listo, Laura! Tu cita de Corte en Barbería El Paisa quedó confirmada para el martes 5 de marzo, 3:00 p. m.",
		domain.OutcomeCancelled: "Tu cita de Corte en Barbería El Paisa del martes 5 de marzo, 3:00 p. m. quedó cancelada.",
		domain.OutcomeCutoff:    "comunícate con Barbería El Paisa.",
		domain.OutcomeNotActive: "ya no está activa",
		domain.OutcomeInvalid:   "ya no es válido",
	} {
		if got := domain.ReplyText(o, c); !strings.Contains(got, want) {
			t.Errorf("%s: %q", o, got)
		}
	}
}
