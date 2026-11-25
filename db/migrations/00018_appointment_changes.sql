-- Changes of an appointment (Phase 6, T6.6): cancellation data on the appointment and a fuller
-- history (previous state, time and professional, reason, who acted as customer or business).

-- +goose Up
ALTER TABLE appointments
    ADD COLUMN cancelled_at timestamptz,
    ADD COLUMN cancel_reason text,
    ADD COLUMN cancelled_by text,
    ADD CONSTRAINT appointments_cancelled_by_valid CHECK (cancelled_by IS NULL OR cancelled_by IN ('CUSTOMER', 'BUSINESS')),
    ADD CONSTRAINT appointments_cancel_reason_length CHECK (cancel_reason IS NULL OR char_length(cancel_reason) <= 300),
    ADD CONSTRAINT appointments_cancellation_consistent CHECK ((status = 'CANCELLED') = (cancelled_at IS NOT NULL));

ALTER TABLE appointment_events
    ADD COLUMN actor_role text,
    ADD COLUMN from_status text,
    ADD COLUMN previous_start_at timestamptz,
    ADD COLUMN previous_professional_id uuid,
    ADD COLUMN reason text,
    ADD CONSTRAINT appointment_events_actor_role_valid CHECK (actor_role IS NULL OR actor_role IN ('CUSTOMER', 'BUSINESS')),
    ADD CONSTRAINT appointment_events_reason_length CHECK (reason IS NULL OR char_length(reason) <= 300);

-- +goose Down
ALTER TABLE appointment_events
    DROP CONSTRAINT appointment_events_reason_length,
    DROP CONSTRAINT appointment_events_actor_role_valid,
    DROP COLUMN reason,
    DROP COLUMN previous_professional_id,
    DROP COLUMN previous_start_at,
    DROP COLUMN from_status,
    DROP COLUMN actor_role;
ALTER TABLE appointments
    DROP CONSTRAINT appointments_cancellation_consistent,
    DROP CONSTRAINT appointments_cancel_reason_length,
    DROP CONSTRAINT appointments_cancelled_by_valid,
    DROP COLUMN cancelled_by,
    DROP COLUMN cancel_reason,
    DROP COLUMN cancelled_at;
