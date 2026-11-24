-- name: CreateMembership :one
INSERT INTO memberships (id, user_id, business_id, role)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListActiveMembershipsByUser :many
-- Loaded per request for authorization (no cached claims, ADR-0007).
SELECT business_id, role FROM memberships
WHERE user_id = $1 AND status = 'ACTIVE';

-- name: InsertAuditEntry :exec
-- occurred_at comes from the application clock (ADR-0005), not from now().
INSERT INTO audit_log (id, occurred_at, actor_user_id, business_id, action, resource_type, resource_id, ip, outcome, metadata)
VALUES ($1, sqlc.arg(occurred_at)::timestamptz, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: InsertMembership :exec
INSERT INTO memberships (id, user_id, business_id, role, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $5);
