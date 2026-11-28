-- P-11: an owner may create more businesses itself (OWNED_BUSINESS_LIMIT). The optional
-- Idempotency-Key of POST /api/v1/businesses: one business per (account, key); a repeated request
-- with the same key and data returns that business instead of creating another.

-- +goose Up
CREATE TABLE business_creation_keys (
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    key         text        NOT NULL,
    fingerprint text        NOT NULL,
    business_id uuid        REFERENCES businesses (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL,
    PRIMARY KEY (user_id, key),
    CONSTRAINT business_creation_keys_key_valid CHECK (key ~ '^[A-Za-z0-9_-]{8,100}$')
);

-- +goose Down
DROP TABLE business_creation_keys;
