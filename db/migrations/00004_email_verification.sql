-- Email verification (required before first login, decision of 2026-11-19).
-- Only the SHA-256 of each token is stored; the raw token exists only in the email.

-- +goose Up
ALTER TABLE users ADD COLUMN email_verified_at timestamptz;
ALTER TABLE users ALTER COLUMN status SET DEFAULT 'PENDING_VERIFICATION';

CREATE TABLE email_verification_tokens (
    token_hash bytea PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    CONSTRAINT email_verification_tokens_sha256 CHECK (octet_length(token_hash) = 32),
    CONSTRAINT email_verification_tokens_expiry CHECK (expires_at > created_at)
);
CREATE INDEX email_verification_tokens_user_idx ON email_verification_tokens (user_id);

-- +goose Down
DROP TABLE email_verification_tokens;
ALTER TABLE users ALTER COLUMN status SET DEFAULT 'ACTIVE';
ALTER TABLE users DROP COLUMN email_verified_at;
