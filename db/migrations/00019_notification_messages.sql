-- Notifications (Phase 7, T7.1), owned by the notifications module. One row per message to send
-- for a change of an appointment (notifications.md): queued in the same transaction as the
-- change, sent by cmd/worker. UNIQUE (event_id, channel, recipient_role) makes every message
-- exist once whatever retries happen. professional_id and start_at are what the message
-- announces: at send time they are compared with the appointment to drop stale messages.

-- +goose Up
CREATE TABLE notification_messages (
    id                  uuid        PRIMARY KEY,
    business_id         uuid        NOT NULL,
    appointment_id      uuid        NOT NULL,
    event_id            uuid        NOT NULL REFERENCES appointment_events (id),
    kind                text        NOT NULL,
    channel             text        NOT NULL,
    recipient_role      text        NOT NULL,
    recipient           text        NOT NULL,
    professional_id     uuid        NOT NULL,
    start_at            timestamptz NOT NULL,
    status              text        NOT NULL,
    skip_reason         text,
    error_code          text,
    provider_message_id text,
    attempts            integer     NOT NULL DEFAULT 0,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    sent_at             timestamptz,
    CONSTRAINT notification_messages_appointment_fk FOREIGN KEY (business_id, appointment_id)
        REFERENCES appointments (business_id, id),
    CONSTRAINT notification_messages_once UNIQUE (event_id, channel, recipient_role),
    CONSTRAINT notification_messages_kind_valid CHECK (kind IN
        ('APPOINTMENT_CREATED', 'APPOINTMENT_CANCELLED', 'APPOINTMENT_RESCHEDULED', 'APPOINTMENT_REASSIGNED')),
    CONSTRAINT notification_messages_channel_valid CHECK (channel IN ('WHATSAPP', 'PUSH')),
    CONSTRAINT notification_messages_recipient_role_valid CHECK (recipient_role IN ('CUSTOMER', 'PROFESSIONAL')),
    CONSTRAINT notification_messages_recipient_length CHECK (char_length(recipient) BETWEEN 1 AND 200),
    CONSTRAINT notification_messages_status_valid CHECK (status IN
        ('PENDING', 'SENT', 'DELIVERED', 'READ', 'FAILED', 'SKIPPED')),
    CONSTRAINT notification_messages_skip_reason CHECK ((status = 'SKIPPED') = (skip_reason IS NOT NULL)),
    CONSTRAINT notification_messages_attempts_valid CHECK (attempts >= 0)
);
CREATE INDEX notification_messages_appointment_idx ON notification_messages (business_id, appointment_id, created_at);

-- +goose Down
DROP TABLE notification_messages;
