package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
)

type fakeExpirer struct {
	n     int
	err   error
	calls int
}

func (f *fakeExpirer) ExpireDueTrials(context.Context) (int, error) {
	f.calls++
	return f.n, f.err
}

func TestExpireTrialsWorker(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := &fakeExpirer{n: 3}
	w := &ExpireTrialsWorker{svc: f, logger: logger}
	if err := w.Work(context.Background(), &river.Job[ExpireTrialsArgs]{}); err != nil || f.calls != 1 {
		t.Fatalf("work: %v, calls %d", err, f.calls)
	}
	f.err = errors.New("db down")
	if err := w.Work(context.Background(), &river.Job[ExpireTrialsArgs]{}); !errors.Is(err, f.err) {
		t.Errorf("error not propagated (River must retry): %v", err)
	}

	args := ExpireTrialsArgs{}
	opts := args.InsertOpts()
	if args.Kind() != "subscriptions.expire_trials" || opts.Queue != jobs.QueueMaintenance || opts.UniqueOpts.ByPeriod != time.Hour {
		t.Errorf("args = %s %+v", args.Kind(), opts)
	}
	if len(PeriodicJobs()) != 1 {
		t.Error("the hourly expiry must be scheduled")
	}
	workers := river.NewWorkers()
	if err := Register(workers, f, logger); err != nil {
		t.Fatal(err)
	}
	if err := Register(workers, f, logger); err == nil {
		t.Error("registering twice must fail")
	}
}
