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

-- name: UpsertMembership :execrows
-- Adds an ACTIVE membership, or reactivates a REVOKED one with the new role. 0 rows = the user
-- is already an active member.
INSERT INTO memberships (id, user_id, business_id, role, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $5)
ON CONFLICT (user_id, business_id) DO UPDATE
SET role = EXCLUDED.role, status = 'ACTIVE', updated_at = EXCLUDED.updated_at
WHERE memberships.status = 'REVOKED';

-- name: ListActiveMembersOfBusiness :many
SELECT m.user_id, u.full_name, u.email::text AS email, m.role, m.updated_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.business_id = $1 AND m.status = 'ACTIVE'
ORDER BY CASE m.role WHEN 'OWNER' THEN 0 ELSE 1 END, u.full_name, m.user_id;

-- name: ActiveMembershipRole :one
SELECT role FROM memberships
WHERE business_id = $1 AND user_id = $2 AND status = 'ACTIVE';

-- name: LockActiveOwners :many
-- Locks the OWNER rows of a business: removing or demoting an OWNER is serialized so the
-- business can never end up without one.
SELECT user_id FROM memberships
WHERE business_id = $1 AND role = 'OWNER' AND status = 'ACTIVE'
FOR UPDATE;

-- name: UpdateMembershipRole :exec
UPDATE memberships SET role = $3, updated_at = $4
WHERE business_id = $1 AND user_id = $2 AND status = 'ACTIVE';

-- name: RevokeMembership :exec
UPDATE memberships SET status = 'REVOKED', updated_at = $3
WHERE business_id = $1 AND user_id = $2 AND status = 'ACTIVE';

-- name: ActiveMembersAmong :many
SELECT user_id FROM memberships
WHERE business_id = sqlc.arg(business_id) AND status = 'ACTIVE' AND user_id = ANY(sqlc.arg(user_ids)::uuid[]);
