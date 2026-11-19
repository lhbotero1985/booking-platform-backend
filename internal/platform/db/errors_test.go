package db

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestErrorClassification(t *testing.T) {
	wrap := func(code, constraint string) error {
		return fmt.Errorf("repo: insert: %w", &pgconn.PgError{Code: code, ConstraintName: constraint})
	}

	exclusion := wrap(CodeExclusionViolation, "appointments_no_overlap")
	if !IsExclusionViolation(exclusion) || ConstraintName(exclusion) != "appointments_no_overlap" {
		t.Errorf("exclusion violation not detected through wrapping")
	}
	if !IsUniqueViolation(wrap(CodeUniqueViolation, "")) {
		t.Error("unique violation not detected")
	}
	if !IsForeignKeyViolation(wrap(CodeForeignKeyViolation, "")) {
		t.Error("foreign key violation not detected")
	}
	for _, code := range []string{CodeSerializationFail, CodeDeadlockDetected, CodeLockNotAvailable} {
		if !IsRetryable(wrap(code, "")) {
			t.Errorf("%s should be retryable", code)
		}
	}
	if IsRetryable(wrap(CodeUniqueViolation, "")) {
		t.Error("unique violation must not be retryable")
	}

	plain := errors.New("boom")
	if SQLState(plain) != "" || ConstraintName(plain) != "" || IsUniqueViolation(plain) || IsRetryable(plain) {
		t.Error("non-PostgreSQL error misclassified")
	}
	if !IsNoRows(fmt.Errorf("get: %w", pgx.ErrNoRows)) || IsNoRows(plain) {
		t.Error("IsNoRows misclassified")
	}
}

func TestNewPoolRejectsInvalidURLWithoutLeakingIt(t *testing.T) {
	_, err := NewPool(t.Context(), PoolConfig{URL: "postgres://user:hunter2@[bad-host/db"})
	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("err = %v, want ErrInvalidURL", err)
	}
}
