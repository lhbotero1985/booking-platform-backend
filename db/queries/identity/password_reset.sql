-- name: CreatePasswordResetToken :exec
INSERT INTO password_reset_tokens (token_hash, user_id, created_at, expires_at)
VALUES ($1, $2, sqlc.arg(now)::timestamptz, $3);

-- name: InvalidatePasswordResetTokens :exec
-- Issuing a new token (or changing the password) invalidates previous unused ones.
UPDATE password_reset_tokens SET used_at = sqlc.arg(now)::timestamptz
WHERE user_id = $1 AND used_at IS NULL;

-- name: ConsumePasswordResetToken :one
-- Atomically marks a valid token as used and returns its user (single use).
UPDATE password_reset_tokens SET used_at = sqlc.arg(now)::timestamptz
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > sqlc.arg(now)::timestamptz
RETURNING user_id;

-- name: RevokeOtherUserSessions :execrows
-- Used after an authenticated password change: keeps the current session.
UPDATE sessions SET revoked_at = sqlc.arg(now)::timestamptz
WHERE user_id = sqlc.arg(user_id) AND id <> sqlc.arg(keep_session_id) AND revoked_at IS NULL;
