package domain

import (
	"fmt"
	"strings"
	"time"
)

// Content is what a message says about its appointment (T7.3), read when it is sent.
type Content struct {
	CustomerName     string
	BusinessName     string
	ServiceName      string
	ProfessionalName string
	Start            time.Time      // UTC
	Location         *time.Location // zone of the business
	CancelReason     string

	// T7.6: what the emails also show.
	Status                string // of the appointment
	BusinessAddress       string
	BusinessPhone         string   // E.164
	Latitude, Longitude   *float64 // nil without coordinates
	CancelCutoffHours     int      // 0: no cut-off shown
	RescheduleCutoffHours int

	// T7.8: the business's limit of moves by the customer (0 = no limit) and those done.
	MaxReschedules, Reschedules int
}

// CanReschedule tells whether the customer still may move the appointment (within the business's limit).
func (c Content) CanReschedule() bool {
	return c.MaxReschedules == 0 || c.Reschedules < c.MaxReschedules
}

var (
	weekdaysES = [...]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}
	monthsES   = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre",
		"octubre", "noviembre", "diciembre"}
)

// WhenES is the start in the business's zone, in Spanish (Colombia): "martes 2 de diciembre,
// 10:00 a. m.". 12:00 is "p. m." and 00:00 is "12:00 a. m.".
func (c Content) WhenES() string {
	loc := c.Location
	if loc == nil {
		loc = time.UTC
	}
	t := c.Start.In(loc)
	h, suffix := t.Hour(), "a. m."
	if h >= 12 {
		suffix = "p. m."
	}
	if h %= 12; h == 0 {
		h = 12
	}
	return fmt.Sprintf("%s %d de %s, %d:%02d %s", weekdaysES[t.Weekday()], t.Day(), monthsES[t.Month()-1], h, t.Minute(), suffix)
}

// DateES is the day of the start in the business's zone: "martes 2 de diciembre de 2026".
func (c Content) DateES() string {
	t := c.Start.In(c.loc())
	return fmt.Sprintf("%s %d de %s de %d", weekdaysES[t.Weekday()], t.Day(), monthsES[t.Month()-1], t.Year())
}

// TimeES is the hour of the start in the business's zone: "10:00 a. m.".
func (c Content) TimeES() string {
	when := c.WhenES()
	return when[strings.LastIndex(when, ", ")+2:]
}

func (c Content) loc() *time.Location {
	if c.Location == nil {
		return time.UTC
	}
	return c.Location
}

// RescheduleLimitES tells the customer how many moves are left (T7.8); "" without a limit.
func (c Content) RescheduleLimitES() string {
	if c.MaxReschedules == 0 {
		return ""
	}
	switch left := c.MaxReschedules - c.Reschedules; {
	case left <= 0:
		return "Ya usaste los cambios de hora que permite el negocio para esta cita (" + times(c.MaxReschedules) + ")."
	case left == 1:
		return "Te queda 1 cambio de hora para esta cita (máximo " + times(c.MaxReschedules) + ")."
	default:
		return fmt.Sprintf("Te quedan %d cambios de hora para esta cita (máximo %s).", left, times(c.MaxReschedules))
	}
}

func times(n int) string {
	if n == 1 {
		return "1 vez"
	}
	return fmt.Sprintf("%d veces", n)
}
