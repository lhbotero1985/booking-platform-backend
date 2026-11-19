-- name: CreateSession :one
INSERT INTO sessions (id, user_id, token_hash, client_type, idle_expires_at, absolute_expires_at, ip, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetActiveSessionByTokenHash :one
-- Resolves a presented token to its session and user. Expiry is evaluated against $2 (now)
-- so the application clock stays authoritative and testable.
SELECT sqlc.embed(s), sqlc.embed(u)
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND s.idle_expires_at > sqlc.arg(now)::timestamptz
  AND s.absolute_expires_at > sqlc.arg(now)::timestamptz;

-- name: TouchSession :exec
-- Sliding idle expiry, never beyond the absolute expiry.
UPDATE sessions
SET last_seen_at = sqlc.arg(now)::timestamptz,
    idle_expires_at = LEAST(sqlc.arg(idle_expires_at)::timestamptz, absolute_expires_at)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: RevokeSession :execrows
UPDATE sessions SET revoked_at = sqlc.arg(now)::timestamptz
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: RevokeAllUserSessions :execrows
UPDATE sessions SET revoked_at = sqlc.arg(now)::timestamptz
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: ListActiveUserSessions :many
SELECT id, client_type, created_at, last_seen_at, ip, user_agent
FROM sessions
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL
  AND idle_expires_at > sqlc.arg(now)::timestamptz
  AND absolute_expires_at > sqlc.arg(now)::timestamptz
ORDER BY last_seen_at DESC;
