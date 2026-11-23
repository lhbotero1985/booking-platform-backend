// Package audit builds audit_log entries and writes them for the modules other than identity
// (which predates it and keeps its own writer). Entries are written in the same transaction as
// the change they describe (security.md §3).
//
// Metadata must never contain passwords, tokens, full emails, phone numbers or user agents:
// only identifiers, enums and counters.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db/platformq"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/reqctx"
)

// Outcome matches the audit_log_outcome_valid CHECK.
type Outcome string

// Outcomes.
const (
	Success Outcome = "SUCCESS"
	Failure Outcome = "FAILURE"
	Denied  Outcome = "DENIED"
)

// Entry is one audited event.
type Entry struct {
	ID           uuid.UUID
	OccurredAt   time.Time
	Action       string
	Outcome      Outcome
	ActorUserID  *uuid.UUID
	BusinessID   *uuid.UUID
	ResourceType string
	ResourceID   string
	IP           *netip.Addr
	Metadata     map[string]string
}

// New starts an entry at now (application clock) with a fresh ID and the client IP and request
// ID of the request context.
func New(ctx context.Context, now time.Time, action string, outcome Outcome) *Entry {
	e := &Entry{
		ID: uuid.Must(uuid.NewV7()), OccurredAt: now,
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

// Actor sets the acting user (ignored when nil).
func (e *Entry) Actor(id uuid.UUID) *Entry {
	if id != uuid.Nil {
		e.ActorUserID = &id
	}
	return e
}

// Business sets the business the event belongs to (ignored when nil).
func (e *Entry) Business(id uuid.UUID) *Entry {
	if id != uuid.Nil {
		e.BusinessID = &id
	}
	return e
}

// Resource sets the affected resource.
func (e *Entry) Resource(kind, id string) *Entry {
	e.ResourceType, e.ResourceID = kind, id
	return e
}

// Meta adds a metadata key.
func (e *Entry) Meta(k, v string) *Entry {
	e.Metadata[k] = v
	return e
}

// Append inserts e through db; pass the transaction of the change to make both atomic.
func Append(ctx context.Context, db platformq.DBTX, e *Entry) error {
	meta := []byte("{}")
	if len(e.Metadata) > 0 {
		var err error
		if meta, err = json.Marshal(e.Metadata); err != nil {
			return fmt.Errorf("audit metadata: %w", err)
		}
	}
	return platformq.New(db).InsertAuditEntry(ctx, platformq.InsertAuditEntryParams{
		ID: e.ID, OccurredAt: e.OccurredAt, ActorUserID: e.ActorUserID, BusinessID: e.BusinessID,
		Action: e.Action, ResourceType: optional(e.ResourceType), ResourceID: optional(e.ResourceID),
		Ip: e.IP, Outcome: string(e.Outcome), Metadata: meta,
	})
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
