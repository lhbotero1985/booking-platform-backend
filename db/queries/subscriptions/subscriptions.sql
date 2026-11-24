-- Subscriptions module (T4.3b, ADR-0018). One row per business.

-- name: InsertSubscription :exec
INSERT INTO subscriptions (business_id, status, trial_ends_at, expired_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5);

-- name: SubscriptionByBusiness :one
SELECT business_id, status, trial_ends_at, expired_at, created_at, updated_at
FROM subscriptions
WHERE business_id = $1;

-- name: SubscriptionByBusinessForUpdate :one
SELECT business_id, status, trial_ends_at, expired_at, created_at, updated_at
FROM subscriptions
WHERE business_id = $1
FOR UPDATE;

-- name: UpdateSubscription :exec
UPDATE subscriptions
SET status = $2, trial_ends_at = $3, expired_at = $4, updated_at = $5
WHERE business_id = $1;

-- name: ExpireDueTrials :many
-- Batch for the hourly job; SKIP LOCKED lets two workers run without blocking each other.
UPDATE subscriptions s
SET status = 'EXPIRED', expired_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE s.business_id IN (
    SELECT business_id FROM subscriptions
    WHERE status = 'TRIALING' AND trial_ends_at <= sqlc.arg(now)::timestamptz
    ORDER BY trial_ends_at
    LIMIT sqlc.arg(max_rows)
    FOR UPDATE SKIP LOCKED)
RETURNING s.business_id;

-- name: SubscriptionsOf :many
SELECT business_id, status, trial_ends_at, expired_at, created_at, updated_at
FROM subscriptions
WHERE business_id = ANY(sqlc.arg(business_ids)::uuid[]);
