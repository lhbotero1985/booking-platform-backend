-- T7.6 (ADR-0019): appointment emails. A new channel, a new kind (the customer canceled: an
-- email receipt to the customer and a notice to the professional) and the recipient of an email
-- is resolved when it is sent, so it is empty until then (and an address may be 254 long).

-- +goose Up
ALTER TABLE notification_messages
    DROP CONSTRAINT notification_messages_channel_valid,
    ADD CONSTRAINT notification_messages_channel_valid CHECK (channel IN ('WHATSAPP', 'PUSH', 'EMAIL')),
    DROP CONSTRAINT notification_messages_kind_valid,
    ADD CONSTRAINT notification_messages_kind_valid CHECK (kind IN
        ('APPOINTMENT_CREATED', 'APPOINTMENT_CREATED_BY_BUSINESS', 'APPOINTMENT_CANCELLED',
         'APPOINTMENT_CANCELLED_BY_CUSTOMER', 'APPOINTMENT_RESCHEDULED', 'APPOINTMENT_REASSIGNED',
         'APPOINTMENT_REMINDER')),
    DROP CONSTRAINT notification_messages_recipient_length,
    ADD CONSTRAINT notification_messages_recipient_length CHECK (
        char_length(recipient) <= 254 AND (channel = 'EMAIL' OR char_length(recipient) >= 1));

-- +goose Down
DELETE FROM notification_messages WHERE channel = 'EMAIL' OR kind = 'APPOINTMENT_CANCELLED_BY_CUSTOMER';
ALTER TABLE notification_messages
    DROP CONSTRAINT notification_messages_channel_valid,
    ADD CONSTRAINT notification_messages_channel_valid CHECK (channel IN ('WHATSAPP', 'PUSH')),
    DROP CONSTRAINT notification_messages_kind_valid,
    ADD CONSTRAINT notification_messages_kind_valid CHECK (kind IN
        ('APPOINTMENT_CREATED', 'APPOINTMENT_CREATED_BY_BUSINESS', 'APPOINTMENT_CANCELLED',
         'APPOINTMENT_RESCHEDULED', 'APPOINTMENT_REASSIGNED', 'APPOINTMENT_REMINDER')),
    DROP CONSTRAINT notification_messages_recipient_length,
    ADD CONSTRAINT notification_messages_recipient_length CHECK (char_length(recipient) BETWEEN 1 AND 200);
