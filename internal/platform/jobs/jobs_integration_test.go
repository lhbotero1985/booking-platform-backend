//go:build integration

package jobs_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/riverqueue/river"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/migrate"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

var (
	pool   *pgxpool.Pool
	logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	nextID atomic.Int64
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := pgtest.Start(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := func() int {
		defer c.Terminate()
		p, err := pgxpool.New(ctx, c.URL)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer p.Close()
		sqlDB := stdlib.OpenDBFromPool(p)
		defer sqlDB.Close()
		mg, err := migrate.New(sqlDB)
		if err == nil {
			_, err = mg.Up(ctx) // includes 00002_river.sql
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		pool = p
		return m.Run()
	}()
	os.Exit(code)
}

// recordArgs is a test job; each test uses unique IDs so jobs left by one test cannot be
// mistaken for another's.
type recordArgs struct {
	ID int64 `json:"id"`
}

func (recordArgs) Kind() string { return "test.record" }

type recordWorker struct {
	river.WorkerDefaults[recordArgs]
	done chan int64
}

func (w *recordWorker) Work(_ context.Context, job *river.Job[recordArgs]) error {
	w.done <- job.Args.ID
	return nil
}

func newID() int64 { return nextID.Add(1) }

// startWorker starts a worker client with the test worker and the platform workers, and stops
// it gracefully at the end of the test (asserting the soft stop succeeds).
func startWorker(t *testing.T) (*jobs.Client, chan int64) {
	t.Helper()
	done := make(chan int64, 100)
	workers := river.NewWorkers()
	river.AddWorker(workers, &recordWorker{done: done})
	if err := jobs.RegisterPlatformWorkers(workers, logger); err != nil {
		t.Fatal(err)
	}
	client, err := jobs.NewWorkerClient(pool, logger, workers, river.Config{
		FetchPollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(context.WithoutCancel(t.Context())); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Stop(ctx); err != nil {
			t.Errorf("graceful Stop() = %v", err)
		}
	})
	return client, done
}

func waitFor(t *testing.T, done <-chan int64, id int64, timeout time.Duration) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case got := <-done:
			if got == id {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func countJobs(t *testing.T, id int64) int {
	t.Helper()
	var n int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM river_job WHERE kind = 'test.record' AND (args->>'id')::bigint = $1`, id).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestInsertTxCommittedJobIsProcessed: a job inserted in a committed transaction runs.
func TestInsertTxCommittedJobIsProcessed(t *testing.T) {
	client, done := startWorker(t)
	id := newID()

	err := db.WithTx(t.Context(), pool, func(tx pgx.Tx) error {
		_, err := client.InsertTx(t.Context(), tx, recordArgs{ID: id}, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, done, id, 15*time.Second) {
		t.Fatalf("job %d not processed", id)
	}
}

// TestInsertTxRolledBackJobIsNeverProcessed: if the business transaction fails, the job does
// not exist (transactional outbox, ADR-0008).
func TestInsertTxRolledBackJobIsNeverProcessed(t *testing.T) {
	client, done := startWorker(t)
	id := newID()
	businessErr := errors.New("slot unavailable")

	err := db.WithTx(t.Context(), pool, func(tx pgx.Tx) error {
		if _, err := client.InsertTx(t.Context(), tx, recordArgs{ID: id}, nil); err != nil {
			return err
		}
		return businessErr
	})
	if !errors.Is(err, businessErr) {
		t.Fatalf("err = %v", err)
	}
	if n := countJobs(t, id); n != 0 {
		t.Fatalf("river_job rows for rolled back job = %d, want 0", n)
	}
	if waitFor(t, done, id, 2*time.Second) {
		t.Fatalf("rolled back job %d was processed", id)
	}
}

// TestUniqueByArgsSkipsDuplicates: idempotent enqueue (e.g. the same reminder twice).
func TestUniqueByArgsSkipsDuplicates(t *testing.T) {
	client, err := jobs.NewInsertClient(pool, logger)
	if err != nil {
		t.Fatal(err)
	}
	id := newID()
	opts := &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}

	first, err := client.Insert(t.Context(), recordArgs{ID: id}, opts)
	if err != nil || first.UniqueSkippedAsDuplicate {
		t.Fatalf("first insert: skipped=%v err=%v", first != nil && first.UniqueSkippedAsDuplicate, err)
	}
	second, err := client.Insert(t.Context(), recordArgs{ID: id}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !second.UniqueSkippedAsDuplicate || second.Job.ID != first.Job.ID {
		t.Fatalf("second insert not deduplicated: skipped=%v ids %d/%d",
			second.UniqueSkippedAsDuplicate, first.Job.ID, second.Job.ID)
	}
	if n := countJobs(t, id); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

// TestPingWorkerCompletes exercises the production worker registry and the maintenance queue.
func TestPingWorkerCompletes(t *testing.T) {
	client, _ := startWorker(t)
	events, cancel := client.Subscribe(river.EventKindJobCompleted)
	defer cancel()

	res, err := client.Insert(t.Context(), jobs.PingArgs{Source: "integration-test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Job.Queue != jobs.QueueMaintenance {
		t.Errorf("queue = %q, want %q", res.Job.Queue, jobs.QueueMaintenance)
	}
	timeout := time.After(15 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Job.ID == res.Job.ID {
				return
			}
		case <-timeout:
			t.Fatalf("ping job %d did not complete", res.Job.ID)
		}
	}
}
