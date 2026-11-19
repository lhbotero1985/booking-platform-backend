-- name: CreateMembership :one
INSERT INTO memberships (id, user_id, business_id, role)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListActiveMembershipsByUser :many
-- Loaded per request for authorization (no cached claims, ADR-0007).
SELECT business_id, role FROM memberships
WHERE user_id = $1 AND status = 'ACTIVE';

-- name: InsertAuditEntry :exec
INSERT INTO audit_log (id, actor_user_id, business_id, action, resource_type, resource_id, ip, outcome, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
