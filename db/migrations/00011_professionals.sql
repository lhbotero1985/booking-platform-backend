-- Professionals (Phase 5, T5.2): the bookable profile of a barber/stylist. Every professional
-- is the account of a member of the business (decision of 2026-11-24): a PROFESSIONAL, or an
-- OWNER who also serves customers. One profile per account and business.

-- +goose Up
CREATE TABLE professionals (
    id           uuid        PRIMARY KEY,
    business_id  uuid        NOT NULL REFERENCES businesses (id) ON DELETE CASCADE,
    user_id      uuid        NOT NULL REFERENCES users (id),
    display_name text        NOT NULL,
    bio          text        NOT NULL DEFAULT '',
    status       text        NOT NULL,
    sort_order   integer     NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    CONSTRAINT professionals_business_id_unique UNIQUE (business_id, id), -- composite FKs (schedules, services)
    CONSTRAINT professionals_business_user_unique UNIQUE (business_id, user_id),
    CONSTRAINT professionals_display_name_length CHECK (char_length(display_name) BETWEEN 2 AND 60),
    CONSTRAINT professionals_bio_length CHECK (char_length(bio) <= 500),
    CONSTRAINT professionals_status_valid CHECK (status IN ('ACTIVE', 'INACTIVE')),
    CONSTRAINT professionals_sort_order_range CHECK (sort_order BETWEEN 0 AND 1000)
);
CREATE INDEX professionals_business_order_idx ON professionals (business_id, sort_order, display_name);
CREATE INDEX professionals_user_idx ON professionals (user_id);

-- +goose Down
DROP TABLE professionals;
