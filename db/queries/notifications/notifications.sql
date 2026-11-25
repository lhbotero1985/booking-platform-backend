-- name: InsertMessage :many
-- Returns the row only when it was inserted (a repeated event inserts nothing).
INSERT INTO notification_messages (
    id, business_id, appointment_id, event_id, kind, channel, recipient_role, recipient,
    professional_id, start_at, status, send_at, created_at, updated_at
) VALUES (
    @id, @business_id, @appointment_id, @event_id, @kind, @channel, @recipient_role, @recipient,
    @professional_id, @start_at, 'PENDING', @send_at, @created_at, @created_at
)
ON CONFLICT ON CONSTRAINT notification_messages_once DO NOTHING
RETURNING id;

-- name: MessageByID :one
SELECT * FROM notification_messages WHERE id = @id;

-- name: FinishMessage :exec
-- Records the outcome of one delivery attempt; a message that is no longer pending is left alone.
UPDATE notification_messages
SET status = @status, skip_reason = sqlc.narg(skip_reason), error_code = sqlc.narg(error_code),
    provider_message_id = sqlc.narg(provider_message_id), sent_at = sqlc.narg(sent_at),
    attempts = attempts + @attempted::integer, updated_at = @updated_at
WHERE id = @id AND status = 'PENDING';

-- name: AppointmentMessages :many
SELECT * FROM notification_messages
WHERE business_id = @business_id AND appointment_id = @appointment_id
ORDER BY created_at, id;

-- name: ClaimWebhookEvent :many
-- Returns the row only the first time the event arrives.
INSERT INTO webhook_events (provider, event_id, received_at)
VALUES (@provider, @event_id, @received_at)
ON CONFLICT DO NOTHING
RETURNING event_id;

-- name: UpdateDeliveryStatus :execrows
-- A status from Meta only moves a sent message forward: SENT → DELIVERED → READ, or SENT /
-- DELIVERED → FAILED. Late or repeated statuses change nothing.
UPDATE notification_messages
SET status = @status, error_code = COALESCE(sqlc.narg(error_code), error_code), updated_at = @updated_at
WHERE provider_message_id = @provider_message_id
  AND ((@status = 'DELIVERED' AND status = 'SENT')
    OR (@status = 'READ' AND status IN ('SENT', 'DELIVERED'))
    OR (@status = 'FAILED' AND status IN ('SENT', 'DELIVERED')));

-- name: InsertActionToken :exec
INSERT INTO appointment_action_tokens (
    token_hash, message_id, business_id, appointment_id, action, recipient, expires_at, created_at
) VALUES (
    @token_hash, @message_id, @business_id, @appointment_id, @action, @recipient, @expires_at, @created_at
);

-- name: ActionTokenByHash :one
SELECT * FROM appointment_action_tokens WHERE token_hash = @token_hash;

-- name: LockMessageTokens :many
-- Locks every token of a message: its buttons answer once, in order.
SELECT token_hash FROM appointment_action_tokens WHERE message_id = @message_id ORDER BY token_hash FOR UPDATE;

-- name: UseMessageTokens :exec
UPDATE appointment_action_tokens SET used_at = @used_at WHERE message_id = @message_id AND used_at IS NULL;
