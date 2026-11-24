// Package db provides the PostgreSQL connection pool, transaction helper and error mapping.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig configures NewPool.
type PoolConfig struct {
	URL             string
	MaxConns        int32
	ConnectTimeout  time.Duration
	ApplicationName string
}

// ErrInvalidURL is returned when the connection URL cannot be parsed. The URL itself is never
// included in errors because it carries credentials.
var ErrInvalidURL = errors.New("db: invalid connection URL")

// NewPool builds a pgx pool. It does not connect eagerly: connections are opened on first use,
// so the API can start (and report not-ready) while PostgreSQL is still coming up.
func NewPool(ctx context.Context, cfg PoolConfig) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, ErrInvalidURL
	}
	if cfg.MaxConns > 0 {
		pc.MaxConns = cfg.MaxConns
	}
	if cfg.ConnectTimeout > 0 {
		pc.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	}
	if cfg.ApplicationName != "" {
		pc.ConnConfig.RuntimeParams["application_name"] = cfg.ApplicationName
	}
	// Safety nets so one bad query cannot hold a connection or a lock forever.
	pc.ConnConfig.RuntimeParams["statement_timeout"] = "15000"                   // ms
	pc.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "30000" // ms
	pc.MaxConnIdleTime = 5 * time.Minute
	pc.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}
	return pool, nil
}

// TxBeginner is implemented by *pgxpool.Pool and pgx.Tx (nested savepoints).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type txKey struct{}

// ContextWithTx returns ctx carrying tx. A use case that must change data of two modules
// atomically (e.g. a business and its OWNER membership) passes this context to the other
// module's contract: that module's store then joins tx instead of opening its own.
func ContextWithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFromContext returns the transaction carried by ctx, if any.
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// WithTx runs fn inside a transaction. It commits when fn returns nil and rolls back when fn
// returns an error or panics (the panic is re-raised after the rollback).
//
// If ctx already carries a transaction (ContextWithTx), fn joins it: nothing is begun or
// committed here and an error from fn makes the outer transaction roll back.
func WithTx(ctx context.Context, b TxBeginner, fn func(tx pgx.Tx) error) (err error) {
	if outer, ok := TxFromContext(ctx); ok {
		return fn(outer)
	}
	tx, err := b.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("db: rollback: %w", rbErr))
			}
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}
