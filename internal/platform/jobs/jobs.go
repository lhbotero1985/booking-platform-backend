// Package jobs wires River (ADR-0008): persistent background jobs stored in PostgreSQL and
// inserted in the same transaction as the domain change that triggers them (transactional
// outbox). Business modules define their own job args and workers and register them here.
package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// Queues. Separate queues keep a slow provider (WhatsApp, push) from delaying maintenance.
const (
	QueueDefault       = river.QueueDefault
	QueueNotifications = "notifications"
	QueueMaintenance   = "maintenance"
)

// Client is the River client bound to pgx transactions.
type Client = river.Client[pgx.Tx]

// NewInsertClient returns a client that can only insert jobs (used by the API process).
// Jobs are processed by cmd/worker.
func NewInsertClient(pool *pgxpool.Pool, logger *slog.Logger) (*Client, error) {
	c, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("jobs: insert client: %w", err)
	}
	return c, nil
}

// QueueConfig returns the queues processed by cmd/worker and their concurrency.
func QueueConfig() map[string]river.QueueConfig {
	return map[string]river.QueueConfig{
		QueueDefault:       {MaxWorkers: 5},
		QueueNotifications: {MaxWorkers: 10},
		QueueMaintenance:   {MaxWorkers: 2},
	}
}

// NewWorkerClient returns a client that fetches and runs jobs for the registered workers.
// cfg may carry overrides (e.g. FetchPollInterval in tests); Queues, Workers and Logger are set
// here.
func NewWorkerClient(pool *pgxpool.Pool, logger *slog.Logger, workers *river.Workers, cfg river.Config) (*Client, error) {
	cfg.Logger = logger
	cfg.Workers = workers
	if cfg.Queues == nil {
		cfg.Queues = QueueConfig()
	}
	c, err := river.NewClient(riverpgxv5.New(pool), &cfg)
	if err != nil {
		return nil, fmt.Errorf("jobs: worker client: %w", err)
	}
	return c, nil
}

// RegisterPlatformWorkers registers technical workers owned by the platform (not by a module).
func RegisterPlatformWorkers(workers *river.Workers, logger *slog.Logger) error {
	if err := river.AddWorkerSafely(workers, &PingWorker{logger: logger}); err != nil {
		return fmt.Errorf("jobs: register ping worker: %w", err)
	}
	return nil
}

// PingArgs is a technical job used to verify the queue end to end (smoke tests, health).
type PingArgs struct {
	Source string `json:"source"`
}

// Kind implements river.JobArgs.
func (PingArgs) Kind() string { return "platform.ping" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (PingArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueMaintenance, MaxAttempts: 3}
}

// PingWorker logs the ping and completes.
type PingWorker struct {
	river.WorkerDefaults[PingArgs]
	logger *slog.Logger
}

// Work implements river.Worker.
func (w *PingWorker) Work(ctx context.Context, job *river.Job[PingArgs]) error {
	w.logger.InfoContext(ctx, "ping job processed",
		slog.Int64("job_id", job.ID),
		slog.String("source", job.Args.Source),
		slog.Int("attempt", job.Attempt),
	)
	return nil
}
