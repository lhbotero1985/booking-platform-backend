// Package worker holds the River jobs of the subscriptions module: the hourly expiry of demos
// (ADR-0018).
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
)

// ExpireTrialsArgs runs the expiry of ended demos. It carries no data.
type ExpireTrialsArgs struct{}

// Kind implements river.JobArgs.
func (ExpireTrialsArgs) Kind() string { return "subscriptions.expire_trials" }

// InsertOpts implements river.JobArgsWithInsertOpts. Unique per hour so that several worker
// instances (or a restart) do not run it twice.
func (ExpireTrialsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       jobs.QueueMaintenance,
		MaxAttempts: 5,
		UniqueOpts:  river.UniqueOpts{ByPeriod: time.Hour},
	}
}

// Expirer is the use case run by the job.
type Expirer interface {
	ExpireDueTrials(ctx context.Context) (int, error)
}

// ExpireTrialsWorker runs Expirer.
type ExpireTrialsWorker struct {
	river.WorkerDefaults[ExpireTrialsArgs]
	svc    Expirer
	logger *slog.Logger
}

// Work implements river.Worker.
func (w *ExpireTrialsWorker) Work(ctx context.Context, _ *river.Job[ExpireTrialsArgs]) error {
	n, err := w.svc.ExpireDueTrials(ctx)
	if err != nil {
		return fmt.Errorf("expire trials: %w", err)
	}
	if n > 0 {
		w.logger.InfoContext(ctx, "demos expired", slog.Int("count", n))
	}
	return nil
}

// Register adds the workers of the module.
func Register(workers *river.Workers, svc Expirer, logger *slog.Logger) error {
	return river.AddWorkerSafely(workers, &ExpireTrialsWorker{svc: svc, logger: logger})
}

// PeriodicJobs returns the module's scheduled jobs: demo expiry every hour, also at startup.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return ExpireTrialsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true}),
	}
}
