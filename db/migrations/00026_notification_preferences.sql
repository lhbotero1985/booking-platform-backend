-- T7.9: who wants which messages, and what WhatsApp may cost.
-- * The customer chooses per account (every business): WhatsApp and/or email (at least one) and
--   whether it wants reminders. No row = everything on.
-- * Each specialist profile turns its emails on or off (the specialist or the owner). No row = on.
-- * The platform caps the WhatsApp messages of each business per month (P16: the platform pays).
--   No row = the default of the environment (500).

-- +goose Up
CREATE TABLE customer_notification_preferences (
    user_id    uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    whatsapp   boolean     NOT NULL,
    email      boolean     NOT NULL,
    reminders  boolean     NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT customer_notification_preferences_one_channel CHECK (whatsapp OR email)
);

CREATE TABLE professional_notification_preferences (
    business_id     uuid        NOT NULL,
    professional_id uuid        NOT NULL,
    email           boolean     NOT NULL,
    updated_at      timestamptz NOT NULL,
    PRIMARY KEY (business_id, professional_id),
    CONSTRAINT professional_notification_preferences_professional_fk FOREIGN KEY (business_id, professional_id)
        REFERENCES professionals (business_id, id) ON DELETE CASCADE
);

CREATE TABLE business_whatsapp_limits (
    business_id   uuid        PRIMARY KEY REFERENCES businesses (id) ON DELETE CASCADE,
    monthly_limit integer     NOT NULL,
    updated_at    timestamptz NOT NULL,
    CONSTRAINT business_whatsapp_limits_range CHECK (monthly_limit BETWEEN 0 AND 100000)
);

-- Counting the WhatsApp messages sent by a business in a month (the cap) and by the platform in
-- a day (the global cap).
CREATE INDEX notification_messages_whatsapp_sent ON notification_messages (business_id, sent_at)
    WHERE channel = 'WHATSAPP' AND sent_at IS NOT NULL;

-- +goose Down
DROP INDEX notification_messages_whatsapp_sent;
DROP TABLE business_whatsapp_limits;
DROP TABLE professional_notification_preferences;
DROP TABLE customer_notification_preferences;
