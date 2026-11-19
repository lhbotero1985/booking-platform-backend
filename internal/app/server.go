package app

import (
	"context"
	"log/slog"
	"net"
	"net/http"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
)

// MaxHeaderBytes caps request headers (Go answers 431 above it).
const MaxHeaderBytes = 1 << 16 // 64 KiB

// NewHTTPServer returns the API server with hardened timeouts (slowloris, slow bodies, idle
// keep-alives) and a header size limit. baseCtx is the parent of every request context.
func NewHTTPServer(baseCtx context.Context, cfg config.Config, logger *slog.Logger, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           h,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		MaxHeaderBytes:    MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
	}
}
