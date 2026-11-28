package application

// Statistics of a business and of every business an account owns (frontend proposal P-12,
// 2026-11-27). The rules of each figure are in domain/stats.go. Decisions of the user:
// the available minutes of past periods use the current schedule (approximate if it changed),
// inactive professionals count with the schedule they left, revenue uses the price when booked,
// the platform administrator has no access.

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// ErrInvalidRange means to is before from or the range is longer than its grouping allows
// (366 days by day, five years by month or year).
var ErrInvalidRange = errors.New("scheduling: invalid statistics range")

// StatsQuery selects the statistics, as received: it is validated after the caller is
// authorized (a business of someone else is a 404 whatever the parameters).
type StatsQuery struct {
	From, To       string // YYYY-MM-DD, inclusive, in the calendar of each business
	GroupBy        string // day (default), month or year
	ProfessionalID string // optional, only for one business
}

type statsRange struct {
	from, to domain.LocalDate
	grouping domain.Grouping
}

func (q StatsQuery) parse() (statsRange, error) {
	var r statsRange
	verr := &ValidationError{}
	var ok bool
	if r.from, ok = domain.ParseLocalDate(q.From); !ok {
		verr.add("from", "INVALID_DATE")
	}
	if r.to, ok = domain.ParseLocalDate(q.To); !ok {
		verr.add("to", "INVALID_DATE")
	}
	if r.grouping, ok = domain.ParseGrouping(q.GroupBy); !ok {
		verr.add("groupBy", "INVALID_GROUP_BY")
	}
	if err := verr.orNil(); err != nil {
		return r, err
	}
	if n := domain.DaysBetween(r.from, r.to); n < 1 || n > r.grouping.MaxDays() {
		return r, ErrInvalidRange
	}
	return r, nil
}

// PeriodFigures are the figures of one period of the series.
type PeriodFigures struct {
	Period  string
	Figures domain.Figures
}

// ProfessionalFigures are the figures of one professional.
type ProfessionalFigures struct {
	ProfessionalID uuid.UUID
	DisplayName    string
	Figures        domain.Figures
}

// ServiceFigures are the figures of one service (no available minutes: they belong to people).
type ServiceFigures struct {
	ServiceID uuid.UUID
	Name      string
	Figures   domain.Figures
}

// BusinessFigures are the figures of one business in the consolidated statistics.
type BusinessFigures struct {
	BusinessID uuid.UUID
	Name       string
	Currency   string
	Figures    domain.Figures
}

// CurrencyFigures are the totals of the businesses in one currency.
type CurrencyFigures struct {
	Currency string
	Figures  domain.Figures
}

// Stats is the result of a statistics query.
type Stats struct {
	From, To domain.LocalDate
	Grouping domain.Grouping
	// Currency of the figures; empty in the consolidated statistics without businesses or with
	// several currencies (then Totals is nil, Series empty and TotalsByCurrency has each one).
	Currency string
	Totals   *domain.Figures
	Series   []PeriodFigures
	// ByProfessional: one business, for its OWNER only (nil otherwise).
	ByProfessional []ProfessionalFigures
	ByService      []ServiceFigures
	// ByBusiness and TotalsByCurrency: consolidated statistics only (nil otherwise).
	ByBusiness       []BusinessFigures
	TotalsByCurrency []CurrencyFigures
}

// BusinessStats returns the statistics of a business (P-12): the OWNER sees the whole business
// or one professional, with the figures of each professional; a PROFESSIONAL only its own agenda
// (another professional → authz.ErrForbidden), without ByProfessional.
func (s *Service) BusinessStats(ctx context.Context, p identity.Principal, businessID uuid.UUID, q StatsQuery) (Stats, error) {
	role, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return Stats{}, err
	}
	r, err := q.parse()
	var pro uuid.UUID
	if q.ProfessionalID != "" {
		if pro, err = uuid.Parse(q.ProfessionalID); err != nil {
			return Stats{}, &ValidationError{Fields: []FieldError{{"professionalId", "INVALID_ID"}}}
		}
	}
	if err != nil {
		return Stats{}, err
	}
	roster, err := s.d.Professionals.Roster(ctx, businessID)
	if err != nil {
		return Stats{}, err
	}
	names := map[uuid.UUID]string{}
	var own []uuid.UUID
	for _, e := range roster {
		names[e.ID] = e.DisplayName
		if e.UserID == p.UserID {
			own = append(own, e.ID)
		}
	}
	var filter []uuid.UUID // nil = every professional
	switch _, known := names[pro]; {
	case pro != uuid.Nil && !known:
		return Stats{}, &ValidationError{Fields: []FieldError{{"professionalId", "PROFESSIONAL_NOT_FOUND"}}}
	case role == identity.RoleOwner && pro != uuid.Nil:
		filter = []uuid.UUID{pro}
	case role == identity.RoleOwner:
	case pro != uuid.Nil && !contains(own, pro):
		return Stats{}, authz.ErrForbidden
	default: // a PROFESSIONAL: its own profile (none → zeros)
		filter = append([]uuid.UUID{}, own...)
	}
	b, err := s.statsOf(ctx, businessID, r, filter)
	if err != nil {
		return Stats{}, err
	}
	totals := b.stats.Total.Figures()
	out := Stats{From: r.from, To: r.to, Grouping: r.grouping, Currency: b.currency, Totals: &totals,
		Series: series(r, b.stats.Series), ByService: byService(b.stats)}
	if role == identity.RoleOwner {
		out.ByProfessional = []ProfessionalFigures{}
		for id, t := range b.stats.ByProfessional {
			out.ByProfessional = append(out.ByProfessional, ProfessionalFigures{ProfessionalID: id, DisplayName: names[id], Figures: t.Figures()})
		}
		sort.Slice(out.ByProfessional, func(i, j int) bool {
			a, c := out.ByProfessional[i], out.ByProfessional[j]
			return strings.ToLower(a.DisplayName) < strings.ToLower(c.DisplayName) || (strings.EqualFold(a.DisplayName, c.DisplayName) && a.ProfessionalID.String() < c.ProfessionalID.String())
		})
	}
	return out, nil
}

// MyStats returns the consolidated statistics of every business where the caller is OWNER
// (P-12), with the figures of each one; none → zeros. The range is applied in the calendar of
// each business.
func (s *Service) MyStats(ctx context.Context, p identity.Principal, q StatsQuery) (Stats, error) {
	r, err := q.parse()
	if err != nil {
		return Stats{}, err
	}
	ms, err := s.d.Memberships.MyMemberships(ctx, p)
	if err != nil {
		return Stats{}, err
	}
	var owned []uuid.UUID
	for _, m := range ms {
		if m.Role == identity.RoleOwner {
			owned = append(owned, m.BusinessID)
		}
	}
	summaries, err := s.d.Businesses.BusinessSummaries(ctx, owned)
	if err != nil {
		return Stats{}, err
	}
	type group struct {
		total  domain.Tally
		series map[string]*domain.Tally
	}
	byCurrency := map[string]*group{}
	all := domain.NewStats(time.UTC, r.grouping, r.from, r.to) // services of every business
	out := Stats{From: r.from, To: r.to, Grouping: r.grouping, ByBusiness: []BusinessFigures{}, TotalsByCurrency: []CurrencyFigures{}}
	for _, id := range owned {
		sum, ok := summaries[id]
		if !ok {
			continue
		}
		b, err := s.statsOf(ctx, id, r, nil)
		if err != nil {
			return Stats{}, err
		}
		out.ByBusiness = append(out.ByBusiness, BusinessFigures{BusinessID: id, Name: sum.Name, Currency: b.currency, Figures: b.stats.Total.Figures()})
		g, ok := byCurrency[b.currency]
		if !ok {
			g = &group{series: map[string]*domain.Tally{}}
			byCurrency[b.currency] = g
		}
		g.total.Merge(&b.stats.Total)
		for period, t := range b.stats.Series {
			if g.series[period] == nil {
				g.series[period] = &domain.Tally{}
			}
			g.series[period].Merge(t)
		}
		for svc, t := range b.stats.ByService {
			all.ByService[svc] = t
			all.ServiceNames[svc] = b.stats.ServiceNames[svc]
		}
	}
	sort.Slice(out.ByBusiness, func(i, j int) bool {
		return strings.ToLower(out.ByBusiness[i].Name) < strings.ToLower(out.ByBusiness[j].Name)
	})
	for c, g := range byCurrency {
		out.TotalsByCurrency = append(out.TotalsByCurrency, CurrencyFigures{Currency: c, Figures: g.total.Figures()})
	}
	sort.Slice(out.TotalsByCurrency, func(i, j int) bool { return out.TotalsByCurrency[i].Currency < out.TotalsByCurrency[j].Currency })
	out.ByService = byService(all)
	switch len(byCurrency) {
	case 0:
		var zero domain.Tally
		f := zero.Figures()
		out.Totals, out.Series = &f, series(r, nil)
	case 1:
		for c, g := range byCurrency {
			f := g.total.Figures()
			out.Currency, out.Totals, out.Series = c, &f, series(r, g.series)
		}
	default: // the figures in money are not added across currencies
		out.Series = []PeriodFigures{}
	}
	return out, nil
}

type businessStats struct {
	stats    *domain.Stats
	currency string
}

// statsOf computes the statistics of one business for the professionals in filter (nil: all).
func (s *Service) statsOf(ctx context.Context, businessID uuid.UUID, r statsRange, filter []uuid.UUID) (businessStats, error) {
	pol, err := s.d.Businesses.BookingPolicyOf(ctx, businessID)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return businessStats{}, ErrNotFound
	}
	if err != nil {
		return businessStats{}, err
	}
	loc, err := location(pol)
	if err != nil {
		return businessStats{}, err
	}
	currency, err := s.d.Currencies.CurrencyOf(ctx, businessID)
	if err != nil {
		return businessStats{}, err
	}
	in := func(pro uuid.UUID) bool { return filter == nil || slices.Contains(filter, pro) }
	from, to := domain.ResolveLocal(r.from, 0, loc), domain.ResolveLocal(r.to.AddDays(1), 0, loc)
	st := domain.NewStats(loc, r.grouping, r.from, r.to)
	if filter != nil && len(filter) == 0 {
		return businessStats{stats: st, currency: currency}, nil // a PROFESSIONAL without a profile
	}
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		appts, err := tx.StatsAppointments(ctx, businessID, from, to)
		if err != nil {
			return err
		}
		for _, a := range appts {
			if in(a.ProfessionalID) {
				st.AddAppointment(a)
			}
		}
		visits, err := tx.FirstVisits(ctx, businessID, from, to)
		for _, v := range visits {
			if in(v.ProfessionalID) {
				st.AddFirstVisit(v)
			}
		}
		return err
	})
	if err != nil {
		return businessStats{}, err
	}
	schedules, err := s.d.Schedules.WeekSchedules(ctx, businessID, from, to)
	if err != nil {
		return businessStats{}, err
	}
	for pro, ws := range schedules {
		if in(pro) {
			addAvailable(st, pro, ws, r, loc)
		}
	}
	return businessStats{stats: st, currency: currency}, nil
}

// addAvailable counts the working minutes of a professional on every day of the range since its
// profile was created, with its current schedule.
func addAvailable(st *domain.Stats, pro uuid.UUID, ws professionals.WeekSchedule, r statsRange, loc *time.Location) {
	ranges := func(in []professionals.MinuteRange) []domain.LocalRange {
		out := make([]domain.LocalRange, 0, len(in))
		for _, m := range in {
			out = append(out, domain.LocalRange{Start: m.Start, End: m.End})
		}
		return out
	}
	var working, breaks [7][]domain.LocalRange
	for d := range ws.Days {
		working[d], breaks[d] = ranges(ws.Days[d].WorkingHours), ranges(ws.Days[d].Breaks)
	}
	absences := make([]domain.Interval, 0, len(ws.Absences))
	for _, a := range ws.Absences {
		absences = append(absences, domain.Interval{Start: a.Start, End: a.End})
	}
	start := r.from
	if created := domain.LocalDateOf(ws.CreatedAt, loc); r.from.Before(created) {
		start = created
	}
	for d := start; !r.to.Before(d); d = d.AddDays(1) {
		if w := working[d.Weekday()]; len(w) > 0 {
			st.AddAvailable(pro, d, domain.WorkingMinutes(d, w, breaks[d.Weekday()], absences, loc))
		}
	}
}

// series lists every period of the range in order, empty ones included.
func series(r statsRange, by map[string]*domain.Tally) []PeriodFigures {
	periods := r.grouping.Periods(r.from, r.to)
	out := make([]PeriodFigures, 0, len(periods))
	for _, p := range periods {
		t := by[p]
		if t == nil {
			t = &domain.Tally{}
		}
		out = append(out, PeriodFigures{Period: p, Figures: t.Figures()})
	}
	return out
}

func byService(st *domain.Stats) []ServiceFigures {
	out := []ServiceFigures{}
	for id, t := range st.ByService {
		out = append(out, ServiceFigures{ServiceID: id, Name: st.ServiceNames[id], Figures: t.Figures()})
	}
	sort.Slice(out, func(i, j int) bool {
		a, c := out[i], out[j]
		return strings.ToLower(a.Name) < strings.ToLower(c.Name) || (strings.EqualFold(a.Name, c.Name) && a.ServiceID.String() < c.ServiceID.String())
	})
	return out
}
