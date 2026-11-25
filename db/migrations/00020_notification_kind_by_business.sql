-- T7.3: an appointment booked by the staff for the customer has its own message (P40: it
-- carries the link to the app), so its own kind.

-- +goose Up
ALTER TABLE notification_messages
    DROP CONSTRAINT notification_messages_kind_valid,
    ADD CONSTRAINT notification_messages_kind_valid CHECK (kind IN
        ('APPOINTMENT_CREATED', 'APPOINTMENT_CREATED_BY_BUSINESS', 'APPOINTMENT_CANCELLED',
         'APPOINTMENT_RESCHEDULED', 'APPOINTMENT_REASSIGNED'));

-- +goose Down
DELETE FROM notification_messages WHERE kind = 'APPOINTMENT_CREATED_BY_BUSINESS';
ALTER TABLE notification_messages
    DROP CONSTRAINT notification_messages_kind_valid,
    ADD CONSTRAINT notification_messages_kind_valid CHECK (kind IN
        ('APPOINTMENT_CREATED', 'APPOINTMENT_CANCELLED', 'APPOINTMENT_RESCHEDULED', 'APPOINTMENT_REASSIGNED'));
