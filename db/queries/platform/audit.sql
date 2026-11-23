-- name: InsertAuditEntry :exec
-- Shared audit writer for modules other than identity (internal/platform/audit). occurred_at
-- comes from the application clock (ADR-0005), not from now().
INSERT INTO audit_log (id, occurred_at, actor_user_id, business_id, action, resource_type, resource_id, ip, outcome, metadata)
VALUES ($1, sqlc.arg(occurred_at)::timestamptz, $2, $3, $4, $5, $6, $7, $8, $9);
