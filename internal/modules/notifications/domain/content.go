package domain

import (
	"fmt"
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
