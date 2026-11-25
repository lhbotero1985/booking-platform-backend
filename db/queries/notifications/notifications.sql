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
-- Records the outcome of one delivery attempt and the recipient it went to (an email address is
-- resolved when sent); a message that is no longer pending is left alone.
UPDATE notification_messages
SET status = @status, recipient = @recipient, skip_reason = sqlc.narg(skip_reason), error_code = sqlc.narg(error_code),
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

-- name: UseActionToken :exec
-- T7.7: a confirmation from an email spends only its own link (the email's cancel link still works).
UPDATE appointment_action_tokens SET used_at = @used_at WHERE token_hash = @token_hash AND used_at IS NULL;

-- T7.9: preferences and the WhatsApp cap.

-- name: CustomerPreferences :one
SELECT whatsapp, email, reminders FROM customer_notification_preferences WHERE user_id = @user_id;

-- name: SetCustomerPreferences :exec
INSERT INTO customer_notification_preferences (user_id, whatsapp, email, reminders, updated_at)
VALUES (@user_id, @whatsapp, @email, @reminders, @updated_at)
ON CONFLICT (user_id) DO UPDATE
SET whatsapp = EXCLUDED.whatsapp, email = EXCLUDED.email, reminders = EXCLUDED.reminders, updated_at = EXCLUDED.updated_at;

-- name: ProfessionalEmailEnabled :one
SELECT email FROM professional_notification_preferences
WHERE business_id = @business_id AND professional_id = @professional_id;

-- name: SetProfessionalEmailEnabled :exec
INSERT INTO professional_notification_preferences (business_id, professional_id, email, updated_at)
VALUES (@business_id, @professional_id, @email, @updated_at)
ON CONFLICT (business_id, professional_id) DO UPDATE SET email = EXCLUDED.email, updated_at = EXCLUDED.updated_at;

-- name: WhatsAppMonthlyLimit :one
SELECT monthly_limit FROM business_whatsapp_limits WHERE business_id = @business_id;

-- name: SetWhatsAppMonthlyLimit :exec
INSERT INTO business_whatsapp_limits (business_id, monthly_limit, updated_at)
VALUES (@business_id, @monthly_limit, @updated_at)
ON CONFLICT (business_id) DO UPDATE SET monthly_limit = EXCLUDED.monthly_limit, updated_at = EXCLUDED.updated_at;

-- WhatsApp messages of a business sent in [from_at, to_at) (any later status counts: the
-- message was paid for).
-- name: CountWhatsAppSentByBusiness :one
SELECT count(*) FROM notification_messages
WHERE business_id = @business_id AND channel = 'WHATSAPP' AND sent_at >= @from_at AND sent_at < @to_at;

-- name: CountWhatsAppSent :one
SELECT count(*) FROM notification_messages
WHERE channel = 'WHATSAPP' AND sent_at >= @from_at AND sent_at < @to_at;
