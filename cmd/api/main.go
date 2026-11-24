// Command api runs the booking platform HTTP API.
//
// Usage:
//
//	api                      start the HTTP server (same as "api serve")
//	api serve                start the HTTP server
//	api migrate up           apply pending migrations (uses DATABASE_MIGRATOR_URL)
//	api migrate down         revert the last migration (development/test only)
//	api migrate status       list migrations and their state
//	api admin grant-platform-admin EMAIL    make an active account PLATFORM_ADMIN (audited)
//	api admin revoke-platform-admin EMAIL   remove PLATFORM_ADMIN (audited)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // embedded IANA database: results must not depend on the host OS (ADR-0005)

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/lhbotero1985/booking-platform-backend/internal/app"
	businesseshttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/interfaces/http"
	geographyhttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/interfaces/http"
	identityhttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/interfaces/http"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/buildinfo"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/logging"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/migrate"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		// The logger may not exist yet (invalid config), so write a minimal JSON line.
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("api stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "serve":
		return serve(ctx, cfg, logger)
	case "migrate":
		return runMigrate(ctx, cfg, logger, args[1:])
	case "admin":
		return runAdmin(ctx, cfg, logger, args[1:])
	default:
		return fmt.Errorf("unknown command %q (use: serve | migrate up|down|status | admin ...)", cmd)
	}
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	info := buildinfo.Get()

	pool, err := db.NewPool(ctx, db.PoolConfig{
		URL:             cfg.Database.URL,
		MaxConns:        cfg.Database.MaxConns,
		ConnectTimeout:  cfg.Database.ConnectTimeout,
		ApplicationName: "booking-api",
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	migrator, err := migrate.New(sqlDB)
	if err != nil {
		return err
	}

	if err := pool.Ping(ctx); err != nil {
		// Not fatal: /readyz reports it and the pool reconnects when PostgreSQL is back.
		logger.Warn("database not reachable at startup", slog.String("error", err.Error()))
	}

	jobClient, err := jobs.NewInsertClient(pool, logger)
	if err != nil {
		return err
	}
	limiters := app.NewRateLimiters(cfg.RateLimit, clock.System{})
	identitySvc, err := app.NewIdentityService(cfg, pool, jobClient, logger, limiters.Login)
	if err != nil {
		return err
	}

	identityHandler := identityhttp.New(identitySvc, logger, identityhttp.Options{AllowedOrigins: cfg.HTTP.CORSAllowedOrigins, AuthLimiter: limiters.Auth})
	geographySvc := app.NewGeographyService(pool, identitySvc)
	businessesSvc := app.NewBusinessesService(pool, identitySvc, geographySvc)

	handler := app.NewHTTPHandler(cfg, logger, app.Deps{
		Readiness: []app.Check{
			{Name: "database", Fn: pool.Ping},
			{Name: "migrations", Fn: migrator.Check},
		},
		Modules: []app.RouteRegistrar{
			identityHandler,
			geographyhttp.New(geographySvc, logger, identityHandler.RequireAuth),
			businesseshttp.New(businessesSvc, logger, identityHandler.RequireAuth),
		},
		APILimiter: limiters.API,
	})
	srv := app.NewHTTPServer(ctx, cfg, logger, handler)

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("api listening",
			slog.String("addr", cfg.HTTP.Addr),
			slog.String("env", string(cfg.Env)),
			slog.String("version", info.Version),
			slog.String("commit", info.Commit),
		)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	logger.Info("api shutting down", slog.Duration("timeout", cfg.HTTP.ShutdownTimeout))
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("api stopped cleanly")
	return nil
}

func runMigrate(ctx context.Context, cfg config.Config, logger *slog.Logger, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: api migrate up|down|status")
	}
	action := args[0]
	if action == "down" && cfg.Env.IsDeployed() {
		// QA/PROD are forward-only; reverting there requires a backup restore (rollback.md).
		return fmt.Errorf("migrate down is not allowed in %s", cfg.Env)
	}

	pool, err := db.NewPool(ctx, db.PoolConfig{
		URL:             cfg.Database.MigratorURL,
		MaxConns:        2,
		ConnectTimeout:  cfg.Database.ConnectTimeout,
		ApplicationName: "booking-migrate",
	})
	if err != nil {
		return err
	}
	defer pool.Close()
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	m, err := migrate.New(sqlDB)
	if err != nil {
		return err
	}

	switch action {
	case "up":
		res, err := m.Up(ctx)
		for _, r := range res {
			logger.Info("migration applied", slog.Int64("version", r.Version), slog.String("file", r.Path))
		}
		if err != nil {
			return err
		}
		logger.Info("migrations up to date", slog.Int("applied", len(res)))
	case "down":
		r, err := m.Down(ctx)
		if err != nil {
			return err
		}
		logger.Info("migration reverted", slog.Int64("version", r.Version), slog.String("file", r.Path))
	case "status":
		st, err := m.Status(ctx)
		if err != nil {
			return err
		}
		for _, s := range st {
			logger.Info("migration", slog.Int64("version", s.Version), slog.String("file", s.Path), slog.Bool("applied", s.Applied))
		}
	default:
		return fmt.Errorf("unknown migrate action %q (use: up | down | status)", action)
	}
	return nil
}

// runAdmin runs operator commands. They need server access (they run inside the api container)
// and are never exposed over HTTP.
func runAdmin(ctx context.Context, cfg config.Config, logger *slog.Logger, args []string) error {
	if len(args) != 2 || (args[0] != "grant-platform-admin" && args[0] != "revoke-platform-admin") {
		return errors.New("usage: api admin grant-platform-admin|revoke-platform-admin EMAIL")
	}
	pool, err := db.NewPool(ctx, db.PoolConfig{
		URL:             cfg.Database.URL,
		MaxConns:        2,
		ConnectTimeout:  cfg.Database.ConnectTimeout,
		ApplicationName: "booking-admin",
	})
	if err != nil {
		return err
	}
	defer pool.Close()
	svc, err := app.NewIdentityService(cfg, pool, nil, logger, nil)
	if err != nil {
		return err
	}
	grant := args[0] == "grant-platform-admin"
	if err := svc.SetPlatformAdmin(ctx, args[1], grant); err != nil {
		return err
	}
	logger.Info("platform admin updated", slog.Bool("platform_admin", grant))
	return nil
}
