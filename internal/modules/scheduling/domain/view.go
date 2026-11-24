package domain

// Agenda views (T6.5): a day, a week (Monday to Sunday), a month or a year of the business's
// local calendar.

// View is an agenda period.
type View string

// Agenda views.
const (
	ViewDay   View = "day"
	ViewWeek  View = "week"
	ViewMonth View = "month"
	ViewYear  View = "year"
)

// ParseView validates a view name.
func ParseView(s string) (View, bool) {
	switch v := View(s); v {
	case ViewDay, ViewWeek, ViewMonth, ViewYear:
		return v, true
	default:
		return "", false
	}
}

// Range returns the local days [from, to) of the view that contains d.
func (v View) Range(d LocalDate) (from, to LocalDate) {
	switch v {
	case ViewWeek:
		from = d.AddDays(-((int(d.Weekday()) + 6) % 7)) // back to Monday
		return from, from.AddDays(7)
	case ViewMonth:
		from = LocalDate{d.Year, d.Month, 1}
		t := from.civil().AddDate(0, 1, 0)
		return from, LocalDate{t.Year(), t.Month(), 1}
	case ViewYear:
		return LocalDate{d.Year, 1, 1}, LocalDate{d.Year + 1, 1, 1}
	default:
		return d, d.AddDays(1)
	}
}
