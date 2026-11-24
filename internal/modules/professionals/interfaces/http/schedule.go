package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// ScheduleService is the weekly schedule part of the application (T5.3).
type ScheduleService interface {
	GetSchedule(ctx context.Context, p contract.Principal, businessID, id uuid.UUID) (domain.WeeklySchedule, error)
	ReplaceSchedule(ctx context.Context, p contract.Principal, businessID, id uuid.UUID, days []domain.DayInput) (domain.WeeklySchedule, error)
}

type rangeDTO struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type dayDTO struct {
	Weekday      string     `json:"weekday"`
	WorkingHours []rangeDTO `json:"workingHours"`
	Breaks       []rangeDTO `json:"breaks"`
}

type scheduleDTO struct {
	Days []dayDTO `json:"days"`
}

// weekOrder lists the days Monday first, as people read a week.
var weekOrder = [...]time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}

func toScheduleDTO(w domain.WeeklySchedule) scheduleDTO {
	out := scheduleDTO{Days: make([]dayDTO, 0, len(weekOrder))}
	for _, wd := range weekOrder {
		d := w.Days[wd]
		out.Days = append(out.Days, dayDTO{Weekday: domain.WeekdayName(wd), WorkingHours: toRanges(d.WorkingHours), Breaks: toRanges(d.Breaks)})
	}
	return out
}

func toRanges(rs []domain.TimeRange) []rangeDTO {
	out := make([]rangeDTO, 0, len(rs))
	for _, r := range rs {
		out = append(out, rangeDTO{Start: r.Start.String(), End: r.End.String()})
	}
	return out
}

func fromRanges(rs []rangeDTO) []domain.RangeInput {
	out := make([]domain.RangeInput, len(rs))
	for i, r := range rs {
		out[i] = domain.RangeInput{Start: r.Start, End: r.End}
	}
	return out
}

func (h *Handler) getSchedule(w http.ResponseWriter, r *http.Request) {
	v, ok := ids(r, "businessId", "professionalId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	week, err := h.svc.GetSchedule(r.Context(), p, v[0], v[1])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toScheduleDTO(week))
}

func (h *Handler) replaceSchedule(w http.ResponseWriter, r *http.Request) {
	var req scheduleDTO
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, ok := ids(r, "businessId", "professionalId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	days := make([]domain.DayInput, len(req.Days))
	for i, d := range req.Days {
		days[i] = domain.DayInput{Weekday: d.Weekday, WorkingHours: fromRanges(d.WorkingHours), Breaks: fromRanges(d.Breaks)}
	}
	p, _ := contract.PrincipalFrom(r.Context())
	week, err := h.svc.ReplaceSchedule(r.Context(), p, v[0], v[1], days)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toScheduleDTO(week))
}
