-- Identity & Access schema (ADR-0006, ADR-0007, docs/architecture/database.md).
-- Owned exclusively by internal/modules/identity.
--
-- Notes:
-- - Session and reset tokens are never stored: only their SHA-256 (32 bytes).
-- - memberships.business_id / audit_log.business_id get their FOREIGN KEY to businesses in the
--   Phase 4 migration that creates that table.

-- +goose Up
CREATE TABLE users (
    id                uuid PRIMARY KEY,
    email             citext      NOT NULL,
    phone             text,
    password_hash     text        NOT NULL,
    full_name         text        NOT NULL,
    status            text        NOT NULL DEFAULT 'ACTIVE',
    is_platform_admin boolean     NOT NULL DEFAULT false,
    locale            text        NOT NULL DEFAULT 'es',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_unique UNIQUE (email),
    CONSTRAINT users_email_length CHECK (char_length(email) BETWEEN 3 AND 254),
    CONSTRAINT users_status_valid CHECK (status IN ('PENDING_VERIFICATION', 'ACTIVE', 'SUSPENDED')),
    CONSTRAINT users_full_name_length CHECK (char_length(full_name) BETWEEN 1 AND 120),
    CONSTRAINT users_phone_e164 CHECK (phone IS NULL OR phone ~ '^\+[1-9][0-9]{6,14}$'),
    CONSTRAINT users_password_hash_phc CHECK (password_hash LIKE '$argon2id$%')
);

CREATE TABLE sessions (
    id                  uuid PRIMARY KEY,
    user_id             uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash          bytea       NOT NULL,
    client_type         text        NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    last_seen_at        timestamptz NOT NULL DEFAULT now(),
    idle_expires_at     timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    revoked_at          timestamptz,
    ip                  inet,
    user_agent          text,
    CONSTRAINT sessions_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT sessions_token_hash_sha256 CHECK (octet_length(token_hash) = 32),
    CONSTRAINT sessions_client_type_valid CHECK (client_type IN ('MOBILE', 'WEB')),
    CONSTRAINT sessions_expiry_order CHECK (idle_expires_at <= absolute_expires_at),
    CONSTRAINT sessions_user_agent_length CHECK (user_agent IS NULL OR char_length(user_agent) <= 512)
);
CREATE INDEX sessions_user_active_idx ON sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE password_reset_tokens (
    token_hash bytea PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    CONSTRAINT password_reset_tokens_sha256 CHECK (octet_length(token_hash) = 32)
);
CREATE INDEX password_reset_tokens_user_idx ON password_reset_tokens (user_id);

CREATE TABLE memberships (
    id          uuid PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    business_id uuid        NOT NULL,
    role        text        NOT NULL,
    status      text        NOT NULL DEFAULT 'ACTIVE',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT memberships_user_business_unique UNIQUE (user_id, business_id),
    CONSTRAINT memberships_role_valid CHECK (role IN ('OWNER', 'MANAGER', 'PROFESSIONAL')),
    CONSTRAINT memberships_status_valid CHECK (status IN ('ACTIVE', 'REVOKED'))
);
-- Exactly one active OWNER per business (invariant from domain-model.md).
CREATE UNIQUE INDEX memberships_one_active_owner_idx
    ON memberships (business_id) WHERE role = 'OWNER' AND status = 'ACTIVE';
CREATE INDEX memberships_business_idx ON memberships (business_id);

CREATE TABLE audit_log (
    id            uuid PRIMARY KEY,
    occurred_at   timestamptz NOT NULL DEFAULT now(),
    actor_user_id uuid,
    business_id   uuid,
    action        text        NOT NULL,
    resource_type text,
    resource_id   text,
    ip            inet,
    outcome       text        NOT NULL,
    metadata      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT audit_log_outcome_valid CHECK (outcome IN ('SUCCESS', 'FAILURE', 'DENIED')),
    CONSTRAINT audit_log_action_format CHECK (action ~ '^[a-z][a-z0-9_.]*$')
);
CREATE INDEX audit_log_occurred_idx ON audit_log (occurred_at DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_user_id, occurred_at DESC);
CREATE INDEX audit_log_business_idx ON audit_log (business_id, occurred_at DESC) WHERE business_id IS NOT NULL;

-- +goose Down
DROP TABLE audit_log;
DROP TABLE memberships;
DROP TABLE password_reset_tokens;
DROP TABLE sessions;
DROP TABLE users;
