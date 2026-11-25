-- T7.4: reminders. One change can now produce two messages to the same recipient (the notice
-- and the reminder), so a message is unique per event, kind, channel and recipient role.
-- send_at is when its delivery is due (the reminder time; the change itself otherwise).

-- +goose Up
ALTER TABLE notification_messages
    ADD COLUMN send_at timestamptz,
    DROP CONSTRAINT notification_messages_once,
    ADD CONSTRAINT notification_messages_once UNIQUE (event_id, kind, channel, recipient_role),
    DROP CONSTRAINT notification_messages_kind_valid,
    ADD CONSTRAINT notification_messages_kind_valid CHECK (kind IN
        ('APPOINTMENT_CREATED', 'APPOINTMENT_CREATED_BY_BUSINESS', 'APPOINTMENT_CANCELLED',
         'APPOINTMENT_RESCHEDULED', 'APPOINTMENT_REASSIGNED', 'APPOINTMENT_REMINDER'));
UPDATE notification_messages SET send_at = created_at;
ALTER TABLE notification_messages ALTER COLUMN send_at SET NOT NULL;

-- +goose Down
DELETE FROM notification_messages WHERE kind = 'APPOINTMENT_REMINDER';
ALTER TABLE notification_messages
    DROP COLUMN send_at,
    DROP CONSTRAINT notification_messages_once,
    ADD CONSTRAINT notification_messages_once UNIQUE (event_id, channel, recipient_role),
    DROP CONSTRAINT notification_messages_kind_valid,
    ADD CONSTRAINT notification_messages_kind_valid CHECK (kind IN
        ('APPOINTMENT_CREATED', 'APPOINTMENT_CREATED_BY_BUSINESS', 'APPOINTMENT_CANCELLED',
         'APPOINTMENT_RESCHEDULED', 'APPOINTMENT_REASSIGNED'));
