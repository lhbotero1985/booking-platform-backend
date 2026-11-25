// Package worker holds the River jobs of the notifications module (notifications.md §5). The job
// carries only the message id: the recipient and the content are read when it runs.
package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
)

// MaxAttempts of a delivery: River retries with exponential backoff (about a day in total).
const MaxAttempts = 8

// SendMessageArgs delivers one notification message.
type SendMessageArgs struct {
	MessageID uuid.UUID `json:"message_id"`
}

// Kind implements river.JobArgs.
func (SendMessageArgs) Kind() string { return "notifications.send_message" }

// InsertOpts implements river.JobArgsWithInsertOpts: one job per message.
func (SendMessageArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       jobs.QueueNotifications,
		MaxAttempts: MaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// Sender is the use case the job runs.
type Sender interface {
	Send(ctx context.Context, id uuid.UUID, lastAttempt bool) error
}

// SendMessageWorker runs SendMessageArgs.
type SendMessageWorker struct {
	river.WorkerDefaults[SendMessageArgs]
	svc Sender
}

// Work implements river.Worker.
func (w *SendMessageWorker) Work(ctx context.Context, job *river.Job[SendMessageArgs]) error {
	return w.svc.Send(ctx, job.Args.MessageID, job.Attempt >= job.MaxAttempts)
}

// Timeout bounds one delivery attempt (provider call included).
func (w *SendMessageWorker) Timeout(*river.Job[SendMessageArgs]) time.Duration {
	return 30 * time.Second
}

// Register adds the workers of the module.
func Register(workers *river.Workers, svc Sender) error {
	if err := river.AddWorkerSafely(workers, &SendMessageWorker{svc: svc}); err != nil {
		return fmt.Errorf("notifications worker: %w", err)
	}
	return nil
}
