package db

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL SQLSTATE codes the application reacts to.
const (
	CodeUniqueViolation     = "23505"
	CodeForeignKeyViolation = "23503"
	CodeCheckViolation      = "23514"
	CodeExclusionViolation  = "23P01" // e.g. overlapping appointments (ADR-0004)
	CodeSerializationFail   = "40001"
	CodeDeadlockDetected    = "40P01"
	CodeLockNotAvailable    = "55P03"
	CodeQueryCanceled       = "57014" // statement_timeout / lock_timeout
)

// ErrNotFound reports that a query returned no rows.
var ErrNotFound = errors.New("db: not found")

// SQLState returns the SQLSTATE of a PostgreSQL error, or "" if err is not one.
func SQLState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// ConstraintName returns the violated constraint name, or "".
func ConstraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

// IsUniqueViolation reports a duplicate key error.
func IsUniqueViolation(err error) bool { return SQLState(err) == CodeUniqueViolation }

// IsExclusionViolation reports an exclusion constraint error (double booking guard).
func IsExclusionViolation(err error) bool { return SQLState(err) == CodeExclusionViolation }

// IsForeignKeyViolation reports a missing referenced row.
func IsForeignKeyViolation(err error) bool { return SQLState(err) == CodeForeignKeyViolation }

// IsRetryable reports transient concurrency errors that a caller may retry.
func IsRetryable(err error) bool {
	switch SQLState(err) {
	case CodeSerializationFail, CodeDeadlockDetected, CodeLockNotAvailable:
		return true
	default:
		return false
	}
}

// IsNoRows reports pgx's "no rows in result set".
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
