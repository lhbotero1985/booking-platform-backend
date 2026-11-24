-- Subscriptions (Phase 4, T4.3b, ADR-0018): the right to use the service, separate from the
-- moderation status of the business. Every business has exactly one subscription.
-- TRIALING (7-day demo) → EXPIRED when the demo ends without payment; ACTIVE/PAST_DUE/CANCELED
-- arrive with payments (phase 11B). A business is visible only if businesses.status = ACTIVE
-- and its subscription is TRIALING, ACTIVE or PAST_DUE.

-- +goose Up
CREATE TABLE subscriptions (
    business_id        uuid        PRIMARY KEY REFERENCES businesses (id) ON DELETE CASCADE,
    status             text        NOT NULL,
    trial_ends_at      timestamptz,
    expired_at         timestamptz,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    CONSTRAINT subscriptions_status_valid
        CHECK (status IN ('TRIALING', 'ACTIVE', 'PAST_DUE', 'EXPIRED', 'CANCELED')),
    CONSTRAINT subscriptions_trial_has_end CHECK (status <> 'TRIALING' OR trial_ends_at IS NOT NULL),
    CONSTRAINT subscriptions_expired_has_date CHECK (status <> 'EXPIRED' OR expired_at IS NOT NULL)
);
-- The expiry job scans demos by end date.
CREATE INDEX subscriptions_trial_end_idx ON subscriptions (trial_ends_at) WHERE status = 'TRIALING';

-- Businesses created before this migration (QA test data) start a fresh 7-day demo.
INSERT INTO subscriptions (business_id, status, trial_ends_at, created_at, updated_at)
SELECT id, 'TRIALING', now() + interval '7 days', now(), now() FROM businesses;

-- +goose Down
DROP TABLE subscriptions;
