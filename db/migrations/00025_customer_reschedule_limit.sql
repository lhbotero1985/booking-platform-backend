-- T7.8: each business limits how many times the customer may move one appointment (by the
-- app, the API or the email's "Reagendar" link). 0 = no limit; 1–10 = that many times. Moves by
-- the business do not count. The default is 2 (user's decision), also for existing businesses.

-- +goose Up
ALTER TABLE businesses
    ADD COLUMN max_customer_reschedules smallint NOT NULL DEFAULT 2,
    ADD CONSTRAINT businesses_max_customer_reschedules CHECK (max_customer_reschedules BETWEEN 0 AND 10);

-- Times the customer moved the appointment, kept on the row so the limit is checked under the
-- appointment's lock.
ALTER TABLE appointments
    ADD COLUMN customer_reschedules smallint NOT NULL DEFAULT 0,
    ADD CONSTRAINT appointments_customer_reschedules CHECK (customer_reschedules >= 0);

-- Appointments moved before this migration: count what their history says.
UPDATE appointments a
SET customer_reschedules = e.n
FROM (SELECT appointment_id, count(*) AS n
      FROM appointment_events
      WHERE type = 'RESCHEDULED' AND actor_role = 'CUSTOMER'
      GROUP BY appointment_id) e
WHERE e.appointment_id = a.id;

-- +goose Down
ALTER TABLE appointments
    DROP CONSTRAINT appointments_customer_reschedules,
    DROP COLUMN customer_reschedules;
ALTER TABLE businesses
    DROP CONSTRAINT businesses_max_customer_reschedules,
    DROP COLUMN max_customer_reschedules;
