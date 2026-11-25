package worker

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
)

type recorder struct {
	id       uuid.UUID
	last     bool
	item     domain.WebhookItem
	to, text string
}

func (r *recorder) Send(_ context.Context, id uuid.UUID, last bool) error {
	r.id, r.last = id, last
	return nil
}

func (r *recorder) ProcessWebhookItem(_ context.Context, it domain.WebhookItem) error {
	r.item = it
	return nil
}

func (r *recorder) SendReply(_ context.Context, to, text string) error {
	r.to, r.text = to, text
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

func TestWebhookJobs(t *testing.T) {
	r := &recorder{}
	item := domain.WebhookItem{Kind: domain.WebhookStatus, EventID: "status:wamid.1:READ"}
	pa := ProcessWebhookItemArgs{Item: item}
	if pa.Kind() != "notifications.process_webhook_item" || pa.InsertOpts().Queue != jobs.QueueNotifications {
		t.Errorf("%s %+v", pa.Kind(), pa.InsertOpts())
	}
	if err := (&ProcessWebhookItemWorker{svc: r}).Work(context.Background(), &river.Job[ProcessWebhookItemArgs]{JobRow: &rivertype.JobRow{}, Args: pa}); err != nil || r.item != item {
		t.Errorf("process: %v %+v", err, r.item)
	}
	ra := SendReplyArgs{To: "+573001234567", Text: "Listo"}
	if ra.Kind() != "notifications.send_reply" || ra.InsertOpts().MaxAttempts != 5 {
		t.Errorf("%s %+v", ra.Kind(), ra.InsertOpts())
	}
	w := &SendReplyWorker{svc: r}
	if err := w.Work(context.Background(), &river.Job[SendReplyArgs]{JobRow: &rivertype.JobRow{}, Args: ra}); err != nil || r.to != ra.To || r.text != ra.Text {
		t.Errorf("reply: %v %q %q", err, r.to, r.text)
	}
	if w.Timeout(nil) <= 0 {
		t.Error("a reply attempt must be bounded")
	}
}
