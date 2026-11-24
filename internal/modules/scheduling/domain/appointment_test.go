package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

func TestNewAppointment(t *testing.T) {
	id, biz, pro, user := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	svc := domain.ServiceSnapshot{ServiceID: uuid.Must(uuid.NewV7()), Name: "Corte", DurationMinutes: 45, PriceMinor: 3_500_000, Currency: "COP"}
	c := domain.Customer{UserID: user, Name: "Ana", Phone: "+573001234567"}
	bog, _ := time.LoadLocation("America/Bogota")
	start := time.Date(2030, 3, 5, 9, 0, 0, 0, bog)
	now := time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)
	a, err := domain.NewAppointment(id, biz, pro, svc, c, start, 10*time.Minute, user, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != domain.StatusPendingConfirmation || a.Version != 1 || a.Start.Location() != time.UTC ||
		!a.End.Equal(start.Add(45*time.Minute)) || !a.OccupiedUntil().Equal(start.Add(55*time.Minute)) {
		t.Errorf("appointment = %+v", a)
	}
	for name, edit := range map[string]func(*domain.ServiceSnapshot, *domain.Customer, *time.Duration){
		"no service":      func(s *domain.ServiceSnapshot, _ *domain.Customer, _ *time.Duration) { s.ServiceID = uuid.Nil },
		"zero duration":   func(s *domain.ServiceSnapshot, _ *domain.Customer, _ *time.Duration) { s.DurationMinutes = 0 },
		"no phone":        func(_ *domain.ServiceSnapshot, c *domain.Customer, _ *time.Duration) { c.Phone = "" },
		"no name":         func(_ *domain.ServiceSnapshot, c *domain.Customer, _ *time.Duration) { c.Name = "" },
		"negative buffer": func(_ *domain.ServiceSnapshot, _ *domain.Customer, b *time.Duration) { *b = -time.Minute },
	} {
		s, cu, b := svc, c, 10*time.Minute
		edit(&s, &cu, &b)
		if _, err := domain.NewAppointment(id, biz, pro, s, cu, start, b, user, now); !errors.Is(err, domain.ErrInvalidAppointment) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for st, occ := range map[domain.Status]bool{
		domain.StatusPendingConfirmation: true, domain.StatusConfirmed: true, domain.StatusInProgress: true,
		domain.StatusCompleted: false, domain.StatusCancelled: false, domain.StatusNoShow: false,
	} {
		if st.Occupies() != occ {
			t.Errorf("%s occupies = %v", st, st.Occupies())
		}
	}
}
