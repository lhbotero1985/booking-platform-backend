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
