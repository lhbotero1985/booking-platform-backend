//go:build integration

package migrate_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/migrate"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

var dbURL string

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := pgtest.Start(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dbURL = c.URL
	code := m.Run()
	c.Terminate()
	os.Exit(code)
}

func openDB(t *testing.T) (*pgxpool.Pool, *sql.DB) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close(); pool.Close() })
	return pool, sqlDB
}

func extensions(t *testing.T, pool *pgxpool.Pool) map[string]bool {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT extname FROM pg_extension`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

// TestUpDownUp applies all migrations, reverts them one by one and applies them again,
// proving every Down section works and Up is repeatable.
func TestUpDownUp(t *testing.T) {
	ctx := t.Context()
	pool, sqlDB := openDB(t)
	m, err := migrate.New(sqlDB)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Check(ctx); !errors.Is(err, migrate.ErrPending) {
		t.Fatalf("fresh DB Check() = %v, want ErrPending", err)
	}

	applied, err := m.Up(ctx)
	if err != nil || len(applied) == 0 {
		t.Fatalf("Up() = %d results, err %v", len(applied), err)
	}
	if err := m.Check(ctx); err != nil {
		t.Fatalf("Check() after Up = %v", err)
	}
	ext := extensions(t, pool)
	for _, name := range []string{"postgis", "btree_gist", "citext"} {
		if !ext[name] {
			t.Errorf("extension %s not installed", name)
		}
	}
	st, err := m.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		if !s.Applied {
			t.Errorf("migration %d not applied", s.Version)
		}
	}

	// Revert everything.
	for range applied {
		if _, err := m.Down(ctx); err != nil {
			t.Fatalf("Down() error = %v", err)
		}
	}
	if err := m.Check(ctx); !errors.Is(err, migrate.ErrPending) {
		t.Fatalf("Check() after full Down = %v, want ErrPending", err)
	}
	// postgis is never dropped (see 00001_extensions.sql); the others must be gone.
	if ext := extensions(t, pool); ext["btree_gist"] || ext["citext"] {
		t.Errorf("extensions still present after Down: %v", ext)
	}

	// And again.
	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("second Up() error = %v", err)
	}
	if err := m.Check(ctx); err != nil {
		t.Fatalf("Check() after second Up = %v", err)
	}
}
