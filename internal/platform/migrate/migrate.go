// Package migrate applies the embedded Goose migrations and reports schema status.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"

	"github.com/lhbotero1985/booking-platform-backend/db/migrations"
)

// ErrPending is returned by Check when migrations are waiting to be applied.
var ErrPending = errors.New("migrate: pending migrations")

// Migrator wraps a Goose provider bound to the embedded migrations.
type Migrator struct {
	p *goose.Provider
}

// New creates a Migrator over db using the embedded migrations.
func New(db *sql.DB) (*Migrator, error) {
	return NewWithFS(db, migrations.FS)
}

// NewWithFS creates a Migrator over an arbitrary migrations filesystem (used in tests).
func NewWithFS(db *sql.DB, fsys fs.FS) (*Migrator, error) {
	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys)
	if err != nil {
		return nil, fmt.Errorf("migrate: provider: %w", err)
	}
	return &Migrator{p: p}, nil
}

// Result describes one applied or reverted migration.
type Result struct {
	Version   int64
	Path      string
	Direction string
}

// Up applies every pending migration.
func (m *Migrator) Up(ctx context.Context) ([]Result, error) {
	res, err := m.p.Up(ctx)
	if err != nil {
		return toResults(res), fmt.Errorf("migrate: up: %w", err)
	}
	return toResults(res), nil
}

// Down reverts the most recent migration.
func (m *Migrator) Down(ctx context.Context) (Result, error) {
	r, err := m.p.Down(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("migrate: down: %w", err)
	}
	return toResult(r), nil
}

// Status describes the state of one migration.
type Status struct {
	Version int64
	Path    string
	Applied bool
}

// Status lists every known migration with its state.
func (m *Migrator) Status(ctx context.Context) ([]Status, error) {
	st, err := m.p.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: status: %w", err)
	}
	out := make([]Status, 0, len(st))
	for _, s := range st {
		out = append(out, Status{
			Version: s.Source.Version,
			Path:    s.Source.Path,
			Applied: s.State == goose.StateApplied,
		})
	}
	return out, nil
}

// Check returns ErrPending if migrations are waiting, or a connection error. It is the
// readiness probe for the schema.
func (m *Migrator) Check(ctx context.Context) error {
	pending, err := m.p.HasPending(ctx)
	if err != nil {
		return fmt.Errorf("migrate: check: %w", err)
	}
	if pending {
		return ErrPending
	}
	return nil
}

func toResults(rs []*goose.MigrationResult) []Result {
	out := make([]Result, 0, len(rs))
	for _, r := range rs {
		out = append(out, toResult(r))
	}
	return out
}

func toResult(r *goose.MigrationResult) Result {
	if r == nil || r.Source == nil {
		return Result{}
	}
	return Result{Version: r.Source.Version, Path: r.Source.Path, Direction: r.Direction}
}
