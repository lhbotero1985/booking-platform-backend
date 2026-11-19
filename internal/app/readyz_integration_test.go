//go:build integration

package app

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/migrate"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestReadyzAgainstRealDatabase: 503 while migrations are pending, 200 once applied, 503
// again when the database goes away.
func TestReadyzAgainstRealDatabase(t *testing.T) {
	c, err := pgtest.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Terminate)

	pool, err := pgxpool.New(t.Context(), c.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close() })
	m, err := migrate.New(sqlDB)
	if err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t,
		Check{Name: "database", Fn: pool.Ping},
		Check{Name: "migrations", Fn: m.Check},
	)

	if resp := do(t, srv, http.MethodGet, "/readyz"); resp.status != http.StatusServiceUnavailable {
		t.Fatalf("before migrations: status = %d, want 503", resp.status)
	}

	if _, err := m.Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	if resp := do(t, srv, http.MethodGet, "/readyz"); resp.status != http.StatusOK {
		t.Fatalf("after migrations: status = %d body %s, want 200", resp.status, resp.body)
	}

	c.Terminate()
	if resp := do(t, srv, http.MethodGet, "/readyz"); resp.status != http.StatusServiceUnavailable {
		t.Fatalf("database down: status = %d, want 503", resp.status)
	}
}
