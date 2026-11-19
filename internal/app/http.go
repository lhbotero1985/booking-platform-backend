// Package app is the composition root: it wires platform components and modules together.
package app

import (
	"log/slog"
	"net/http"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/buildinfo"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// NewHTTPHandler builds the complete API handler with its middleware chain.
func NewHTTPHandler(cfg config.Config, logger *slog.Logger) http.Handler {
	rt := httpx.NewRouter()
	rt.HandleFunc("GET /healthz", healthz)

	return httpx.Chain(rt,
		httpx.RequestID,
		httpx.SecurityHeaders,
		httpx.AccessLog(logger),
		httpx.Recover(logger),
		httpx.CORS(cfg.HTTP.CORSAllowedOrigins),
		httpx.LimitBody(cfg.HTTP.MaxBodyBytes),
	)
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// healthz is the liveness probe: it only reports that the process serves HTTP.
// Dependency checks belong to /readyz (T1.4).
func healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: buildinfo.Get().Version})
}
