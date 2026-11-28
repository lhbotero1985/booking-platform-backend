-- Reviews (2026-11-28, decisions of the user): the customer of a COMPLETED appointment rates the
-- business and the professional (1 to 5 each) with an optional comment, once per appointment and
-- up to 30 days after it; the owner replies in public; only a platform administrator hides one.
-- After the appointment is completed the customer gets an email asking for the review.

-- +goose Up
CREATE TABLE appointment_reviews (
    id                  uuid        PRIMARY KEY,
    business_id         uuid        NOT NULL,
    appointment_id      uuid        NOT NULL,
    professional_id     uuid        NOT NULL,
    customer_user_id    uuid        NOT NULL REFERENCES users (id),
    author_name         text        NOT NULL,
    service_name        text        NOT NULL,
    business_rating     smallint    NOT NULL,
    professional_rating smallint    NOT NULL,
    comment             text        NOT NULL DEFAULT '',
    reply               text        NOT NULL DEFAULT '',
    replied_at          timestamptz,
    replied_by          uuid        REFERENCES users (id),
    hidden_at           timestamptz,
    hidden_by           uuid        REFERENCES users (id),
    hidden_reason       text        NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    CONSTRAINT appointment_reviews_once UNIQUE (appointment_id),
    CONSTRAINT appointment_reviews_appointment_fk FOREIGN KEY (business_id, appointment_id)
        REFERENCES appointments (business_id, id) ON DELETE CASCADE,
    CONSTRAINT appointment_reviews_professional_fk FOREIGN KEY (business_id, professional_id)
        REFERENCES professionals (business_id, id),
    CONSTRAINT appointment_reviews_business_rating_range CHECK (business_rating BETWEEN 1 AND 5),
    CONSTRAINT appointment_reviews_professional_rating_range CHECK (professional_rating BETWEEN 1 AND 5),
    CONSTRAINT appointment_reviews_author_length CHECK (char_length(author_name) BETWEEN 1 AND 80),
    CONSTRAINT appointment_reviews_comment_length CHECK (char_length(comment) <= 1000),
    CONSTRAINT appointment_reviews_reply_length CHECK (char_length(reply) <= 1000),
    CONSTRAINT appointment_reviews_reply_consistent CHECK ((reply = '') = (replied_at IS NULL) AND (replied_at IS NULL) = (replied_by IS NULL)),
    CONSTRAINT appointment_reviews_hidden_consistent CHECK ((hidden_at IS NULL) = (hidden_by IS NULL)),
    CONSTRAINT appointment_reviews_hidden_reason_length CHECK (char_length(hidden_reason) <= 200)
);
CREATE INDEX appointment_reviews_business_idx ON appointment_reviews (business_id, created_at DESC, id DESC);
CREATE INDEX appointment_reviews_professional_idx
    ON appointment_reviews (business_id, professional_id, created_at DESC, id DESC);

ALTER TABLE notification_messages
    DROP CONSTRAINT notification_messages_kind_valid,
    ADD CONSTRAINT notification_messages_kind_valid CHECK (kind IN
        ('APPOINTMENT_CREATED', 'APPOINTMENT_CREATED_BY_BUSINESS', 'APPOINTMENT_CANCELLED',
         'APPOINTMENT_CANCELLED_BY_CUSTOMER', 'APPOINTMENT_RESCHEDULED', 'APPOINTMENT_REASSIGNED',
         'APPOINTMENT_REMINDER', 'APPOINTMENT_REVIEW_REQUEST'));

-- +goose Down
DELETE FROM notification_messages WHERE kind = 'APPOINTMENT_REVIEW_REQUEST';
ALTER TABLE notification_messages
    DROP CONSTRAINT notification_messages_kind_valid,
    ADD CONSTRAINT notification_messages_kind_valid CHECK (kind IN
        ('APPOINTMENT_CREATED', 'APPOINTMENT_CREATED_BY_BUSINESS', 'APPOINTMENT_CANCELLED',
         'APPOINTMENT_CANCELLED_BY_CUSTOMER', 'APPOINTMENT_RESCHEDULED', 'APPOINTMENT_REASSIGNED',
         'APPOINTMENT_REMINDER'));
DROP TABLE appointment_reviews;
