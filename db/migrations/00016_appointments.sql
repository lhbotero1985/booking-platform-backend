-- Appointments (Phase 6, T6.3), owned by the scheduling module. ADR-0004: the exclusion
-- constraint is the safety net under the per-professional row lock; it guarantees that two
-- active appointments of a professional never overlap, buffer included, whatever the code does.
-- occupied_until = end_at + buffer is stored because timestamptz + interval is not immutable and
-- cannot be indexed.

-- +goose Up
CREATE TABLE appointments (
    id               uuid        PRIMARY KEY,
    business_id      uuid        NOT NULL,
    professional_id  uuid        NOT NULL,
    service_id       uuid        NOT NULL,
    customer_user_id uuid        REFERENCES users (id),
    customer_name    text        NOT NULL,
    customer_phone   text        NOT NULL,
    start_at         timestamptz NOT NULL,
    end_at           timestamptz NOT NULL,
    buffer_minutes   smallint    NOT NULL,
    occupied_until   timestamptz NOT NULL,
    status           text        NOT NULL,
    -- Snapshot of the service when booked (booking-lifecycle.md §5.2).
    service_name     text        NOT NULL,
    duration_minutes smallint    NOT NULL,
    price_minor      bigint      NOT NULL,
    currency         char(3)     NOT NULL,
    version          integer     NOT NULL DEFAULT 1,
    created_by       uuid        NOT NULL REFERENCES users (id),
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    CONSTRAINT appointments_business_id_unique UNIQUE (business_id, id),
    CONSTRAINT appointments_professional_fk FOREIGN KEY (business_id, professional_id)
        REFERENCES professionals (business_id, id),
    CONSTRAINT appointments_service_fk FOREIGN KEY (business_id, service_id)
        REFERENCES services (business_id, id),
    CONSTRAINT appointments_status_valid CHECK (status IN
        ('PENDING_CONFIRMATION', 'CONFIRMED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED', 'NO_SHOW')),
    CONSTRAINT appointments_period_valid CHECK (start_at < end_at),
    CONSTRAINT appointments_buffer_valid CHECK (buffer_minutes BETWEEN 0 AND 60),
    CONSTRAINT appointments_occupied_valid CHECK (occupied_until >= end_at),
    CONSTRAINT appointments_duration_valid CHECK (duration_minutes BETWEEN 5 AND 480),
    CONSTRAINT appointments_price_valid CHECK (price_minor >= 0),
    CONSTRAINT appointments_customer_name_length CHECK (char_length(customer_name) BETWEEN 1 AND 120),
    CONSTRAINT appointments_customer_phone_e164 CHECK (customer_phone ~ '^\+[1-9][0-9]{6,14}$'),
    CONSTRAINT appointments_version_valid CHECK (version >= 1),
    CONSTRAINT appointments_no_overlap EXCLUDE USING gist (
        professional_id WITH =,
        tstzrange(start_at, occupied_until, '[)') WITH &&
    ) WHERE (status IN ('PENDING_CONFIRMATION', 'CONFIRMED', 'IN_PROGRESS'))
);
CREATE INDEX appointments_professional_start_idx ON appointments (business_id, professional_id, start_at);
CREATE INDEX appointments_customer_start_idx ON appointments (customer_user_id, start_at);

-- History of every change of an appointment; Phase 7 sends notifications from it.
CREATE TABLE appointment_events (
    id              uuid        PRIMARY KEY,
    business_id     uuid        NOT NULL,
    appointment_id  uuid        NOT NULL,
    type            text        NOT NULL,
    status          text        NOT NULL,
    professional_id uuid        NOT NULL,
    start_at        timestamptz NOT NULL,
    actor_user_id   uuid        REFERENCES users (id),
    occurred_at     timestamptz NOT NULL,
    CONSTRAINT appointment_events_appointment_fk FOREIGN KEY (business_id, appointment_id)
        REFERENCES appointments (business_id, id),
    CONSTRAINT appointment_events_type_valid CHECK (type IN ('CREATED', 'CONFIRMED', 'CANCELLED',
        'RESCHEDULED', 'REASSIGNED', 'STARTED', 'COMPLETED', 'NO_SHOW'))
);
CREATE INDEX appointment_events_appointment_idx ON appointment_events (appointment_id, occurred_at);

-- Idempotency-Key of booking requests: one appointment per (account, key).
CREATE TABLE idempotency_keys (
    user_id        uuid        NOT NULL REFERENCES users (id),
    key            text        NOT NULL,
    fingerprint    text        NOT NULL,
    appointment_id uuid        REFERENCES appointments (id),
    created_at     timestamptz NOT NULL,
    PRIMARY KEY (user_id, key),
    CONSTRAINT idempotency_keys_key_valid CHECK (key ~ '^[A-Za-z0-9_-]{8,100}$')
);

-- +goose Down
DROP TABLE idempotency_keys;
DROP TABLE appointment_events;
DROP TABLE appointments;
