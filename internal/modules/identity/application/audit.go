package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/reqctx"
)

// Audited actions (audit_log.action, security.md §3). Stable identifiers: dashboards and
// alerts (Phase 12) will depend on them.
const (
	AuditRegistered             = "auth.registered"
	AuditEmailVerified          = "auth.email_verified"
	AuditLogin                  = "auth.login"
	AuditLogout                 = "auth.logout"
	AuditLogoutAll              = "auth.logout_all"
	AuditPasswordResetRequested = "auth.password_reset_requested"
	AuditPasswordReset          = "auth.password_reset"
	AuditPasswordChanged        = "auth.password_changed"
	AuditAuthzDenied            = "authz.denied"
)

// AuditOutcome matches the audit_log_outcome_valid CHECK.
type AuditOutcome string

// Outcomes.
const (
	OutcomeSuccess AuditOutcome = "SUCCESS"
	OutcomeFailure AuditOutcome = "FAILURE" // wrong credentials, invalid token…
	OutcomeDenied  AuditOutcome = "DENIED"  // refused by a policy: backoff, unverified, role
)

// Audit reasons (metadata "reason").
const (
	reasonUnknownAccount   = "unknown_account"
	reasonWrongPassword    = "wrong_password"
	reasonAccountSuspended = "account_suspended"
	reasonEmailNotVerified = "email_not_verified"
	reasonThrottled        = "throttled"
	reasonEmailTaken       = "email_taken"
	reasonInvalidToken     = "invalid_token"
	reasonNotEligible      = "account_not_eligible"
	reasonNoMembership     = "no_membership"
	reasonInsufficientRole = "insufficient_role"
)

// AuditEntry is one security event. Metadata must never contain passwords, tokens, full
// emails, phone numbers or user agents: only identifiers, enums and counters.
type AuditEntry struct {
	ID           uuid.UUID
	OccurredAt   time.Time
	Action       string
	Outcome      AuditOutcome
	ActorUserID  *uuid.UUID
	BusinessID   *uuid.UUID
	ResourceType string
	ResourceID   string
	IP           *netip.Addr
	Metadata     map[string]string
}

// newAudit starts an entry with ID, time (application clock), client IP and request ID taken
// from the request context.
func (s *Service) newAudit(ctx context.Context, action string, outcome AuditOutcome) *AuditEntry {
	e := &AuditEntry{
		ID: uuid.Must(uuid.NewV7()), OccurredAt: s.clock.Now(),
		Action: action, Outcome: outcome, Metadata: map[string]string{},
	}
	if ip, ok := reqctx.ClientIP(ctx); ok {
		e.IP = &ip
	}
	if id := reqctx.RequestID(ctx); id != "" {
		e.Metadata["request_id"] = id
	}
	return e
}

func (e *AuditEntry) actor(id uuid.UUID) *AuditEntry {
	if id != uuid.Nil {
		e.ActorUserID = &id
	}
	return e
}

func (e *AuditEntry) business(id uuid.UUID) *AuditEntry {
	if id != uuid.Nil {
		e.BusinessID = &id
	}
	return e
}

func (e *AuditEntry) resource(kind string, id uuid.UUID) *AuditEntry {
	e.ResourceType, e.ResourceID = kind, id.String()
	return e
}

func (e *AuditEntry) meta(k, v string) *AuditEntry {
	e.Metadata[k] = v
	return e
}

func (e *AuditEntry) withIP(ip *netip.Addr) *AuditEntry {
	if ip != nil {
		e.IP = ip
	}
	return e
}

// auditDetached writes e in its own transaction. It is used for events that change no data
// (failed login, denials): a failure to audit is logged but does not fail the request, so the
// audit table can never become an availability dependency of authentication.
func (s *Service) auditDetached(ctx context.Context, e *AuditEntry) {
	// Detached from cancellation: a client that disconnects must not erase its own trace.
	dctx := context.WithoutCancel(ctx)
	err := s.store.WithinTx(dctx, func(tx Tx) error { return tx.AppendAudit(dctx, *e) })
	if err != nil {
		s.logger.ErrorContext(ctx, "audit write failed",
			slog.String("action", e.Action), slog.String("outcome", string(e.Outcome)),
			slog.String("error", err.Error()))
	}
}

// emailFingerprint identifies an email in audit metadata without storing it in clear (used
// only when no account exists, so there is no actor ID). Truncated SHA-256 of the normalized
// address: enough to correlate attempts, not meant as a secret.
func emailFingerprint(email domain.Email) string {
	sum := sha256.Sum256([]byte(email.String()))
	return hex.EncodeToString(sum[:8])
}
