// Command worker runs River background jobs (notifications, reminders, maintenance).
//
// It refuses to start while migrations are pending (the job tables may not exist yet); the
// container orchestrator restarts it until `api migrate up` has run.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // embedded IANA database: results must not depend on the host OS (ADR-0005)

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/riverqueue/river"

	"github.com/lhbotero1985/booking-platform-backend/internal/app"
	identityworker "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/worker"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/buildinfo"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/logging"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/migrate"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("worker stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, cfg.LogLevel)
	info := buildinfo.Get()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, db.PoolConfig{
		URL:             cfg.Database.URL,
		MaxConns:        cfg.Database.MaxConns,
		ConnectTimeout:  cfg.Database.ConnectTimeout,
		ApplicationName: "booking-worker",
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
	if err := migrator.Check(ctx); err != nil {
		if errors.Is(err, migrate.ErrPending) {
			return errors.New("pending migrations: run `api migrate up` before starting the worker")
		}
		return err
	}

	// Use cases enqueue follow-up jobs transactionally through an insert-only client (the worker
	// client cannot exist yet: it needs the registered workers).
	inserter, err := jobs.NewInsertClient(pool, logger)
	if err != nil {
		return err
	}
	identitySvc, err := app.NewIdentityService(cfg, pool, inserter, logger)
	if err != nil {
		return err
	}

	workers := river.NewWorkers()
	if err := jobs.RegisterPlatformWorkers(workers, logger); err != nil {
		return err
	}
	if err := identityworker.Register(workers, identitySvc); err != nil {
		return err
	}
	client, err := jobs.NewWorkerClient(pool, logger, workers, river.Config{})
	if err != nil {
		return err
	}

	// Start with a context that is not canceled by the signal: shutdown is driven explicitly
	// below so running jobs get a soft stop first.
	if err := client.Start(context.WithoutCancel(ctx)); err != nil {
		return err
	}
	logger.Info("worker started",
		slog.String("env", string(cfg.Env)),
		slog.String("version", info.Version),
		slog.String("commit", info.Commit),
	)

	<-ctx.Done()
	logger.Info("worker shutting down", slog.Duration("timeout", cfg.Worker.ShutdownTimeout))

	softCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Worker.ShutdownTimeout)
	defer cancel()
	if err := client.Stop(softCtx); err != nil {
		logger.Warn("soft stop timed out, canceling running jobs", slog.String("error", err.Error()))
		hardCtx, hardCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer hardCancel()
		if err := client.StopAndCancel(hardCtx); err != nil {
			return err
		}
	}
	logger.Info("worker stopped cleanly")
	return nil
}
