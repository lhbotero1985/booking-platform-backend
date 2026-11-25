package domain

import (
	"errors"
	"time"
)

// CustomerPreferences are the messages a customer account wants (T7.9), in every business:
// WhatsApp and/or email (at least one) and the reminders. Customers booked by hand without an
// account get everything.
type CustomerPreferences struct {
	WhatsApp, Email, Reminders bool
}

// ErrNoChannel means a customer must keep at least one channel.
var ErrNoChannel = errors.New("notifications: keep at least one channel")

// DefaultCustomerPreferences turns everything on.
func DefaultCustomerPreferences() CustomerPreferences {
	return CustomerPreferences{WhatsApp: true, Email: true, Reminders: true}
}

// Validate returns ErrNoChannel without any channel.
func (p CustomerPreferences) Validate() error {
	if !p.WhatsApp && !p.Email {
		return ErrNoChannel
	}
	return nil
}

// SkipFor tells why a message to the customer is not wanted ("" when it is).
func (p CustomerPreferences) SkipFor(m Message) SkipReason {
	switch {
	case m.Channel == ChannelWhatsApp && !p.WhatsApp, m.Channel == ChannelEmail && !p.Email:
		return SkipCustomerOptedOut
	case m.Kind == KindReminder && !p.Reminders:
		return SkipRemindersOff
	}
	return ""
}

// Limits of the WhatsApp messages (T7.9, P16: the platform pays them).
const (
	// MaxWhatsAppMonthlyLimit bounds the cap of a business.
	MaxWhatsAppMonthlyLimit = 100000
)

// MonthOf is the calendar month (UTC) of t: [from, to). The caps count sent messages in it.
func MonthOf(t time.Time) (from, to time.Time) {
	t = t.UTC()
	from = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 1, 0)
}

// DayOf is the day (UTC) of t: [from, to).
func DayOf(t time.Time) (from, to time.Time) {
	t = t.UTC()
	from = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 0, 1)
}
