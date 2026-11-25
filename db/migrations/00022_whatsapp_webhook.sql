-- WhatsApp webhook (T7.5, notifications.md §3). webhook_events: every item Meta sends (a delivery
-- status, an answer) is processed once, whatever retries Meta does. appointment_action_tokens:
-- the payload of each button of a reminder is an opaque single-use token (only its SHA-256 is
-- stored), valid for the number the reminder went to and until the appointment starts.

-- +goose Up
CREATE TABLE webhook_events (
    provider    text        NOT NULL,
    event_id    text        NOT NULL,
    received_at timestamptz NOT NULL,
    PRIMARY KEY (provider, event_id),
    CONSTRAINT webhook_events_provider_valid CHECK (provider IN ('whatsapp')),
    CONSTRAINT webhook_events_event_id_length CHECK (char_length(event_id) BETWEEN 1 AND 300)
);

CREATE TABLE appointment_action_tokens (
    token_hash     bytea       PRIMARY KEY,
    message_id     uuid        NOT NULL REFERENCES notification_messages (id),
    business_id    uuid        NOT NULL,
    appointment_id uuid        NOT NULL,
    action         text        NOT NULL,
    recipient      text        NOT NULL,
    expires_at     timestamptz NOT NULL,
    used_at        timestamptz,
    created_at     timestamptz NOT NULL,
    CONSTRAINT appointment_action_tokens_appointment_fk FOREIGN KEY (business_id, appointment_id)
        REFERENCES appointments (business_id, id),
    CONSTRAINT appointment_action_tokens_action_valid CHECK (action IN ('CONFIRM', 'CANCEL')),
    CONSTRAINT appointment_action_tokens_hash_length CHECK (octet_length(token_hash) = 32)
);
CREATE INDEX appointment_action_tokens_message_idx ON appointment_action_tokens (message_id);

CREATE UNIQUE INDEX notification_messages_provider_id_idx ON notification_messages (provider_message_id)
    WHERE provider_message_id IS NOT NULL;

-- +goose Down
DROP INDEX notification_messages_provider_id_idx;
DROP TABLE appointment_action_tokens;
DROP TABLE webhook_events;
