// Package app is the composition root: it wires platform components and modules together.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/buildinfo"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// CodeNotReady is returned by /readyz when a dependency is unavailable.
const CodeNotReady = "NOT_READY"

// readinessTimeout bounds each readiness check so a hung dependency cannot hang the probe.
const readinessTimeout = 2 * time.Second

// Check is a named readiness probe (database reachable, migrations applied, …).
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

// RouteRegistrar is implemented by each module's HTTP adapter.
type RouteRegistrar interface {
	Routes(rt *httpx.Router)
}

// Deps are the runtime dependencies the HTTP layer needs.
type Deps struct {
	Readiness []Check
	Modules   []RouteRegistrar
	// APILimiter is the general per-client limit (health probes are exempt). nil disables it.
	APILimiter httpx.Allower
	// Media serves the stored images at GET /media/{key...}; nil serves none.
	Media http.Handler
}

// NewHTTPHandler builds the complete API handler with its middleware chain.
func NewHTTPHandler(cfg config.Config, logger *slog.Logger, deps Deps) http.Handler {
	return httpx.Chain(newRouter(logger, deps, cfg.AppDownloadURL),
		// First: every later middleware (request ID, rate limit, audit) sees the real client.
		httpx.RealIP(cfg.HTTP.TrustedProxies),
		httpx.RequestID,
		httpx.SecurityHeaders,
		httpx.AccessLog(logger),
		httpx.Recover(logger),
		exemptProbes(httpx.RateLimit(deps.APILimiter)),
		httpx.CORS(cfg.HTTP.CORSAllowedOrigins),
		httpx.LimitBody(cfg.HTTP.MaxBodyBytes),
	)
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// healthz is the liveness probe: it only reports that the process serves HTTP.
func healthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: buildinfo.Get().Version})
}

type readyResponse struct {
	Status string `json:"status"`
}

// readyz reports whether the instance can serve traffic. Failure details go to the log only;
// the response names the failing check but never its error text.
func readyz(logger *slog.Logger, checks []Check) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, c := range checks {
			ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
			err := c.Fn(ctx)
			cancel()
			if err != nil {
				logger.WarnContext(r.Context(), "readiness check failed",
					slog.String("request_id", httpx.RequestIDFrom(r.Context())),
					slog.String("check", c.Name),
					slog.String("error", err.Error()),
				)
				httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusServiceUnavailable, CodeNotReady,
					"dependency not ready: "+c.Name))
				return
			}
		}
		httpx.WriteJSON(w, http.StatusOK, readyResponse{Status: "ready"})
	})
}

// exemptProbes skips mw for the health probes, which the orchestrator calls constantly from a
// single address.
func exemptProbes(mw httpx.Middleware) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		limited := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}

// newRouter registers the health probes, the app download page and every module's routes.
func newRouter(logger *slog.Logger, deps Deps, appDownloadURL string) *httpx.Router {
	rt := httpx.NewRouter()
	rt.HandleFunc("GET /healthz", healthz)
	rt.HandleFunc("GET /app", appDownload(appDownloadURL))
	rt.Handle("GET /readyz", readyz(logger, deps.Readiness))
	if deps.Media != nil {
		rt.Handle("GET /media/{key...}", deps.Media)
	}
	for _, m := range deps.Modules {
		m.Routes(rt)
	}
	return rt
}
