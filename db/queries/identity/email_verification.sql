-- name: CreateEmailVerificationToken :exec
INSERT INTO email_verification_tokens (token_hash, user_id, created_at, expires_at)
VALUES ($1, $2, sqlc.arg(now)::timestamptz, $3);

-- name: InvalidateEmailVerificationTokens :exec
-- Issuing a new token invalidates the previous unused ones.
UPDATE email_verification_tokens SET used_at = sqlc.arg(now)::timestamptz
WHERE user_id = $1 AND used_at IS NULL;

-- name: ConsumeEmailVerificationToken :one
-- Atomically marks a valid token as used and returns its user (single use).
UPDATE email_verification_tokens SET used_at = sqlc.arg(now)::timestamptz
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > sqlc.arg(now)::timestamptz
RETURNING user_id;

-- name: MarkUserEmailVerified :exec
UPDATE users SET status = 'ACTIVE', email_verified_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = $1 AND status = 'PENDING_VERIFICATION';
