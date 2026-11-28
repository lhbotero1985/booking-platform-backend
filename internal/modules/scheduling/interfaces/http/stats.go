package httpapi

// Statistics of a business and consolidated (frontend proposal P-12).

import (
	"net/http"

	"github.com/google/uuid"

	contract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// CodeInvalidRange is to before from, or a range longer than its grouping allows.
const CodeInvalidRange = "INVALID_RANGE"

func (h *Handler) statsRoutes(rt *httpx.Router) {
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	rt.Handle("GET /api/v1/business/{businessId}/stats", auth(h.businessStats))
	rt.Handle("GET /api/v1/me/stats", auth(h.myStats))
}

type figuresDTO struct {
	Appointments       int   `json:"appointments"`
	Completed          int   `json:"completed"`
	Canceled           int   `json:"cancelled"` //nolint:misspell // API field (P-12), same spelling as the CANCELLED status
	NoShow             int   `json:"noShow"`
	RevenueMinor       int64 `json:"revenueMinor"`
	AverageTicketMinor int64 `json:"averageTicketMinor"`
	CustomersServed    int   `json:"customersServed"`
	NewCustomers       int   `json:"newCustomers"`
	BookedMinutes      int   `json:"bookedMinutes"`
	AvailableMinutes   int   `json:"availableMinutes"`
	OccupancyPercent   *int  `json:"occupancyPercent"`
}

func toFiguresDTO(f domain.Figures) figuresDTO {
	return figuresDTO{Appointments: f.Appointments, Completed: f.Completed, Canceled: f.Canceled, NoShow: f.NoShow,
		RevenueMinor: f.RevenueMinor, AverageTicketMinor: f.AverageTicketMinor, CustomersServed: f.CustomersServed,
		NewCustomers: f.NewCustomers, BookedMinutes: f.BookedMinutes, AvailableMinutes: f.AvailableMinutes,
		OccupancyPercent: f.OccupancyPercent}
}

type periodDTO struct {
	Period  string     `json:"period"`
	Figures figuresDTO `json:"figures"`
}

type professionalFiguresDTO struct {
	ProfessionalID uuid.UUID  `json:"professionalId"`
	DisplayName    string     `json:"displayName"`
	Figures        figuresDTO `json:"figures"`
}

type serviceFiguresDTO struct {
	ServiceID uuid.UUID  `json:"serviceId"`
	Name      string     `json:"name"`
	Figures   figuresDTO `json:"figures"`
}

type businessFiguresDTO struct {
	BusinessID uuid.UUID  `json:"businessId"`
	Name       string     `json:"name"`
	Currency   string     `json:"currency"`
	Figures    figuresDTO `json:"figures"`
}

type currencyFiguresDTO struct {
	Currency string     `json:"currency"`
	Figures  figuresDTO `json:"figures"`
}

// statsDTO is BusinessStats in the OpenAPI document; the optional lists are left out where they
// do not apply.
type statsDTO struct {
	From             string                    `json:"from"`
	To               string                    `json:"to"`
	GroupBy          string                    `json:"groupBy"`
	Currency         *string                   `json:"currency"`
	Totals           *figuresDTO               `json:"totals"`
	Series           []periodDTO               `json:"series"`
	ByProfessional   *[]professionalFiguresDTO `json:"byProfessional,omitempty"`
	ByService        []serviceFiguresDTO       `json:"byService"`
	ByBusiness       *[]businessFiguresDTO     `json:"byBusiness,omitempty"`
	TotalsByCurrency *[]currencyFiguresDTO     `json:"totalsByCurrency,omitempty"`
}

func toStatsDTO(s application.Stats) statsDTO {
	out := statsDTO{From: s.From.String(), To: s.To.String(), GroupBy: string(s.Grouping),
		Series: []periodDTO{}, ByService: []serviceFiguresDTO{}}
	if s.Currency != "" {
		out.Currency = &s.Currency
	}
	if s.Totals != nil {
		t := toFiguresDTO(*s.Totals)
		out.Totals = &t
	}
	for _, p := range s.Series {
		out.Series = append(out.Series, periodDTO{Period: p.Period, Figures: toFiguresDTO(p.Figures)})
	}
	for _, f := range s.ByService {
		out.ByService = append(out.ByService, serviceFiguresDTO{ServiceID: f.ServiceID, Name: f.Name, Figures: toFiguresDTO(f.Figures)})
	}
	if s.ByProfessional != nil {
		list := make([]professionalFiguresDTO, 0, len(s.ByProfessional))
		for _, f := range s.ByProfessional {
			list = append(list, professionalFiguresDTO{ProfessionalID: f.ProfessionalID, DisplayName: f.DisplayName, Figures: toFiguresDTO(f.Figures)})
		}
		out.ByProfessional = &list
	}
	if s.ByBusiness != nil {
		list := make([]businessFiguresDTO, 0, len(s.ByBusiness))
		for _, f := range s.ByBusiness {
			list = append(list, businessFiguresDTO{BusinessID: f.BusinessID, Name: f.Name, Currency: f.Currency, Figures: toFiguresDTO(f.Figures)})
		}
		out.ByBusiness = &list
	}
	if s.TotalsByCurrency != nil {
		list := make([]currencyFiguresDTO, 0, len(s.TotalsByCurrency))
		for _, f := range s.TotalsByCurrency {
			list = append(list, currencyFiguresDTO{Currency: f.Currency, Figures: toFiguresDTO(f.Figures)})
		}
		out.TotalsByCurrency = &list
	}
	return out
}

// businessStats serves GET /api/v1/business/{businessId}/stats?from=&to=&groupBy=&professionalId=.
func (h *Handler) businessStats(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "businessId")
	if !ok {
		return
	}
	q := r.URL.Query()
	p, _ := contract.PrincipalFrom(r.Context())
	s, err := h.agendas.BusinessStats(r.Context(), p, ids[0], application.StatsQuery{
		From: q.Get("from"), To: q.Get("to"), GroupBy: q.Get("groupBy"), ProfessionalID: q.Get("professionalId")})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toStatsDTO(s))
}

// myStats serves GET /api/v1/me/stats?from=&to=&groupBy=: every business the caller owns.
func (h *Handler) myStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, _ := contract.PrincipalFrom(r.Context())
	s, err := h.agendas.MyStats(r.Context(), p, application.StatsQuery{From: q.Get("from"), To: q.Get("to"), GroupBy: q.Get("groupBy")})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toStatsDTO(s))
}
