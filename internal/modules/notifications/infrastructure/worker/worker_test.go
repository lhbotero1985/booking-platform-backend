package worker

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
)

type recorder struct {
	id   uuid.UUID
	last bool
}

func (r *recorder) Send(_ context.Context, id uuid.UUID, last bool) error {
	r.id, r.last = id, last
	return nil
}

func TestSendMessageJob(t *testing.T) {
	args := SendMessageArgs{MessageID: uuid.Must(uuid.NewV7())}
	opts := args.InsertOpts()
	if args.Kind() != "notifications.send_message" || opts.Queue != jobs.QueueNotifications || opts.MaxAttempts != MaxAttempts || !opts.UniqueOpts.ByArgs {
		t.Fatalf("%s %+v", args.Kind(), opts)
	}
	r := &recorder{}
	w := &SendMessageWorker{svc: r}
	for _, c := range []struct {
		attempt int
		last    bool
	}{{1, false}, {MaxAttempts - 1, false}, {MaxAttempts, true}} {
		job := &river.Job[SendMessageArgs]{JobRow: &rivertype.JobRow{Attempt: c.attempt, MaxAttempts: MaxAttempts}, Args: args}
		if err := w.Work(context.Background(), job); err != nil || r.id != args.MessageID || r.last != c.last {
			t.Errorf("attempt %d: %v, last %v", c.attempt, err, r.last)
		}
	}
	if w.Timeout(nil) <= 0 {
		t.Error("a delivery attempt must be bounded")
	}
	if err := Register(river.NewWorkers(), r); err != nil {
		t.Fatal(err)
	}
}
