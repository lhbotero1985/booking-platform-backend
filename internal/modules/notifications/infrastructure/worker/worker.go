// Package worker holds the River jobs of the notifications module (notifications.md §5). The job
// carries only the message id: the recipient and the content are read when it runs.
package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
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

// ProcessWebhookItemArgs applies one item of a WhatsApp webhook (T7.5).
type ProcessWebhookItemArgs struct {
	Item domain.WebhookItem `json:"item"`
}

// Kind implements river.JobArgs.
func (ProcessWebhookItemArgs) Kind() string { return "notifications.process_webhook_item" }

// InsertOpts implements river.JobArgsWithInsertOpts. Uniqueness comes from webhook_events.
func (ProcessWebhookItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.QueueNotifications, MaxAttempts: 10}
}

// SendReplyArgs answers a customer inside the 24-hour window it opened (T7.5).
type SendReplyArgs struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

// Kind implements river.JobArgs.
func (SendReplyArgs) Kind() string { return "notifications.send_reply" }

// InsertOpts implements river.JobArgsWithInsertOpts: a few retries (the window is short).
func (SendReplyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.QueueNotifications, MaxAttempts: 5}
}

// Webhooks is what the webhook jobs run.
type Webhooks interface {
	ProcessWebhookItem(ctx context.Context, it domain.WebhookItem) error
	SendReply(ctx context.Context, to, text string) error
}

// ProcessWebhookItemWorker runs ProcessWebhookItemArgs.
type ProcessWebhookItemWorker struct {
	river.WorkerDefaults[ProcessWebhookItemArgs]
	svc Webhooks
}

// Work implements river.Worker.
func (w *ProcessWebhookItemWorker) Work(ctx context.Context, job *river.Job[ProcessWebhookItemArgs]) error {
	return w.svc.ProcessWebhookItem(ctx, job.Args.Item)
}

// SendReplyWorker runs SendReplyArgs.
type SendReplyWorker struct {
	river.WorkerDefaults[SendReplyArgs]
	svc Webhooks
}

// Work implements river.Worker.
func (w *SendReplyWorker) Work(ctx context.Context, job *river.Job[SendReplyArgs]) error {
	return w.svc.SendReply(ctx, job.Args.To, job.Args.Text)
}

// Timeout bounds one attempt (provider call included).
func (w *SendReplyWorker) Timeout(*river.Job[SendReplyArgs]) time.Duration { return 30 * time.Second }

// Service is everything the workers of the module run.
type Service interface {
	Sender
	Webhooks
}

// Register adds the workers of the module.
func Register(workers *river.Workers, svc Service) error {
	for _, add := range []func() error{
		func() error { return river.AddWorkerSafely(workers, &SendMessageWorker{svc: svc}) },
		func() error { return river.AddWorkerSafely(workers, &ProcessWebhookItemWorker{svc: svc}) },
		func() error { return river.AddWorkerSafely(workers, &SendReplyWorker{svc: svc}) },
	} {
		if err := add(); err != nil {
			return fmt.Errorf("notifications worker: %w", err)
		}
	}
	return nil
}
