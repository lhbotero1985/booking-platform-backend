package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
)

type fakeSender struct {
	calls []string
	err   error
}

func (f *fakeSender) SendVerificationEmail(_ context.Context, id uuid.UUID) error {
	f.calls = append(f.calls, "verify:"+id.String())
	return f.err
}

func (f *fakeSender) SendPasswordResetEmail(_ context.Context, id uuid.UUID) error {
	f.calls = append(f.calls, "reset:"+id.String())
	return f.err
}

func (f *fakeSender) SendPasswordChangedEmail(_ context.Context, id uuid.UUID) error {
	f.calls = append(f.calls, "changed:"+id.String())
	return f.err
}

func TestRegisterAddsEveryWorkerOnce(t *testing.T) {
	workers := river.NewWorkers()
	if err := Register(workers, &fakeSender{}); err != nil {
		t.Fatal(err)
	}
	if err := Register(workers, &fakeSender{}); err == nil {
		t.Fatal("registering twice must fail (duplicate kinds)")
	}
}

func TestJobArgsCarryOnlyTheUserID(t *testing.T) {
	id := uuid.New()
	for _, args := range []river.JobArgs{SendVerificationEmailArgs{id}, SendPasswordResetEmailArgs{id}, SendPasswordChangedEmailArgs{id}} {
		b, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != `{"user_id":"`+id.String()+`"}` {
			t.Errorf("%s args = %s (tokens must never be stored in the queue)", args.Kind(), b)
		}
	}
}

func TestInsertOpts(t *testing.T) {
	for _, o := range []river.InsertOpts{SendVerificationEmailArgs{}.InsertOpts(), SendPasswordResetEmailArgs{}.InsertOpts()} {
		if o.Queue != jobs.QueueNotifications || !o.UniqueOpts.ByArgs || o.UniqueOpts.ByPeriod != time.Minute {
			t.Errorf("coalesced opts = %+v", o)
		}
	}
	changed := SendPasswordChangedEmailArgs{}.InsertOpts()
	if changed.Queue != jobs.QueueNotifications || changed.UniqueOpts.ByArgs {
		t.Errorf("password changed notices must never be coalesced: %+v", changed)
	}
}

func TestWorkersDelegateAndBoundTime(t *testing.T) {
	id := uuid.New()
	s := &fakeSender{}
	ctx := context.Background()
	v := &SendVerificationEmailWorker{sender: s}
	r := &SendPasswordResetEmailWorker{sender: s}
	c := &SendPasswordChangedEmailWorker{sender: s}
	_ = v.Work(ctx, &river.Job[SendVerificationEmailArgs]{JobRow: &rivertype.JobRow{}, Args: SendVerificationEmailArgs{id}})
	_ = r.Work(ctx, &river.Job[SendPasswordResetEmailArgs]{JobRow: &rivertype.JobRow{}, Args: SendPasswordResetEmailArgs{id}})
	_ = c.Work(ctx, &river.Job[SendPasswordChangedEmailArgs]{JobRow: &rivertype.JobRow{}, Args: SendPasswordChangedEmailArgs{id}})
	want := []string{"verify:" + id.String(), "reset:" + id.String(), "changed:" + id.String()}
	for i := range want {
		if i >= len(s.calls) || s.calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", s.calls, want)
		}
	}
	if v.Timeout(nil) != emailTimeout || r.Timeout(nil) != emailTimeout || c.Timeout(nil) != emailTimeout {
		t.Error("every email job must have a bounded timeout")
	}
	s.err = errors.New("smtp down")
	if err := v.Work(ctx, &river.Job[SendVerificationEmailArgs]{JobRow: &rivertype.JobRow{}, Args: SendVerificationEmailArgs{id}}); err == nil {
		t.Error("send errors must be returned so River retries")
	}
}
