-- T7.7: the links of the appointment emails use the same single-use codes as the WhatsApp
-- buttons, with a third action: reschedule (the customer picks another time in the app).

-- +goose Up
ALTER TABLE appointment_action_tokens
    DROP CONSTRAINT appointment_action_tokens_action_valid,
    ADD CONSTRAINT appointment_action_tokens_action_valid CHECK (action IN ('CONFIRM', 'CANCEL', 'RESCHEDULE'));

-- +goose Down
DELETE FROM appointment_action_tokens WHERE action = 'RESCHEDULE';
ALTER TABLE appointment_action_tokens
    DROP CONSTRAINT appointment_action_tokens_action_valid,
    ADD CONSTRAINT appointment_action_tokens_action_valid CHECK (action IN ('CONFIRM', 'CANCEL'));
