-- ADR-0004: a booking waits at most 3 s for the professional's lock (503 retryable instead of
-- hanging). is_local = true: the settings last until the end of the transaction.
-- name: SetBookingTimeouts :exec
SELECT set_config('lock_timeout', '3s', true), set_config('statement_timeout', '5s', true);

-- Waits for a concurrent transaction holding the same key; returns no row if it already exists.
-- name: ClaimIdempotencyKey :many
INSERT INTO idempotency_keys (user_id, key, fingerprint, created_at)
VALUES (@user_id, @key, @fingerprint, @created_at)
ON CONFLICT (user_id, key) DO NOTHING
RETURNING user_id;

-- name: IdempotencyKeyOf :one
SELECT fingerprint, appointment_id FROM idempotency_keys WHERE user_id = @user_id AND key = @key;

-- name: SetIdempotencyAppointment :exec
UPDATE idempotency_keys SET appointment_id = @appointment_id WHERE user_id = @user_id AND key = @key;

-- name: InsertAppointment :exec
INSERT INTO appointments (id, business_id, professional_id, service_id, customer_id, customer_user_id,
    customer_name, customer_phone, start_at, end_at, buffer_minutes, occupied_until, status, service_name,
    duration_minutes, price_minor, currency, version, created_by, created_at, updated_at,
    cancelled_at, cancel_reason, cancelled_by)
VALUES (@id, @business_id, @professional_id, @service_id, @customer_id, @customer_user_id,
    @customer_name, @customer_phone, @start_at, @end_at, @buffer_minutes, @occupied_until, @status, @service_name,
    @duration_minutes, @price_minor, @currency, @version, @created_by, @created_at, @updated_at,
    @cancelled_at, @cancel_reason, @cancelled_by);

-- name: InsertAppointmentEvent :exec
INSERT INTO appointment_events (id, business_id, appointment_id, type, status, professional_id, start_at,
    actor_user_id, occurred_at, actor_role, from_status, previous_start_at, previous_professional_id, reason)
VALUES (@id, @business_id, @appointment_id, @type, @status, @professional_id, @start_at, @actor_user_id,
    @occurred_at, @actor_role, @from_status, @previous_start_at, @previous_professional_id, @reason);

-- name: AppointmentByID :one
SELECT * FROM appointments WHERE id = @id;

-- Active appointments of a professional whose occupied range [start_at, occupied_until)
-- overlaps [from_at, to_at).
-- name: ActiveAppointmentsOverlapping :many
SELECT id, start_at, end_at, occupied_until FROM appointments
WHERE business_id = @business_id AND professional_id = @professional_id
  AND status IN ('PENDING_CONFIRMATION', 'CONFIRMED', 'IN_PROGRESS')
  AND start_at < @to_at AND occupied_until > @from_at
ORDER BY start_at;

-- name: InsertCustomer :exec
INSERT INTO customers (id, business_id, full_name, phone, email, user_id, created_by, created_at, updated_at)
VALUES (@id, @business_id, @full_name, @phone, @email, @user_id, @created_by, @created_at, @updated_at);

-- name: CustomerByID :one
SELECT * FROM customers WHERE business_id = @business_id AND id = @id;

-- name: CustomerByPhone :one
SELECT * FROM customers WHERE business_id = @business_id AND phone = @phone;

-- name: UpdateCustomer :exec
UPDATE customers SET full_name = @full_name, phone = @phone, email = @email, user_id = @user_id, updated_at = @updated_at
WHERE business_id = @business_id AND id = @id;

-- Customers of a business, optionally filtered by name (case-insensitive, contains) or phone
-- (contains), by name.
-- name: ListCustomers :many
SELECT * FROM customers
WHERE business_id = @business_id
  AND (@q::text = '' OR lower(full_name) LIKE '%' || lower(@q::text) || '%' ESCAPE '\' OR phone LIKE '%' || @q::text || '%' ESCAPE '\')
ORDER BY lower(full_name), id
LIMIT @max_rows;

-- Agenda of a business in [from_at, to_at) (T6.5), every status, by start; optionally only some
-- professionals; keyset pagination after (after_start, after_id).
-- name: AgendaAppointments :many
SELECT * FROM appointments
WHERE business_id = @business_id
  AND (@all_professionals::bool OR professional_id = ANY(@professional_ids::uuid[]))
  AND start_at >= @from_at AND start_at < @to_at
  AND (NOT @has_cursor::bool OR (start_at, id) > (@after_start::timestamptz, @after_id::uuid))
ORDER BY start_at, id
LIMIT @max_rows;

-- Appointments of a customer account from now on, in every business, by start.
-- name: CustomerUpcomingAppointments :many
SELECT * FROM appointments
WHERE customer_user_id = @user_id AND start_at >= @now_at
  AND (NOT @has_cursor::bool OR (start_at, id) > (@after_start::timestamptz, @after_id::uuid))
ORDER BY start_at, id
LIMIT @max_rows;

-- Past appointments of a customer account, in every business, newest first.
-- name: CustomerPastAppointments :many
SELECT * FROM appointments
WHERE customer_user_id = @user_id AND start_at < @now_at
  AND (NOT @has_cursor::bool OR (start_at, id) < (@after_start::timestamptz, @after_id::uuid))
ORDER BY start_at DESC, id DESC
LIMIT @max_rows;

-- Appointments of a customer of the business from now on (P-10), optionally only of some
-- professionals, by start.
-- name: BusinessCustomerUpcomingAppointments :many
SELECT * FROM appointments
WHERE business_id = @business_id AND customer_id = @customer_id AND start_at >= @now_at
  AND (@all_professionals::bool OR professional_id = ANY(@professional_ids::uuid[]))
  AND (NOT @has_cursor::bool OR (start_at, id) > (@after_start::timestamptz, @after_id::uuid))
ORDER BY start_at, id
LIMIT @max_rows;

-- Past appointments of a customer of the business (P-10), optionally only of some
-- professionals, newest first.
-- name: BusinessCustomerPastAppointments :many
SELECT * FROM appointments
WHERE business_id = @business_id AND customer_id = @customer_id AND start_at < @now_at
  AND (@all_professionals::bool OR professional_id = ANY(@professional_ids::uuid[]))
  AND (NOT @has_cursor::bool OR (start_at, id) < (@after_start::timestamptz, @after_id::uuid))
ORDER BY start_at DESC, id DESC
LIMIT @max_rows;

-- Serializes the changes of one appointment (T6.6).
-- name: LockAppointment :one
SELECT * FROM appointments WHERE id = @id FOR UPDATE;

-- Writes a change; the version guard makes a lost update impossible (0 rows = changed meanwhile).
-- name: UpdateAppointment :execrows
UPDATE appointments
SET professional_id = @professional_id, start_at = @start_at, end_at = @end_at, occupied_until = @occupied_until,
    status = @status, cancelled_at = @cancelled_at, cancel_reason = @cancel_reason, cancelled_by = @cancelled_by,
    customer_reschedules = @customer_reschedules, version = version + 1, updated_at = @updated_at
WHERE id = @id AND version = @version;

-- name: AppointmentEvents :many
SELECT * FROM appointment_events WHERE appointment_id = @appointment_id ORDER BY occurred_at, id;

-- Active appointments of a professional whose service [start_at, end_at) overlaps
-- [from_at, to_at), by start (T6.7: conflicts of an absence or a schedule).
-- name: ActiveAppointmentsDuring :many
SELECT * FROM appointments
WHERE business_id = @business_id AND professional_id = @professional_id
  AND status IN ('PENDING_CONFIRMATION', 'CONFIRMED', 'IN_PROGRESS')
  AND start_at < @to_at AND end_at > @from_at
ORDER BY start_at, id;

-- name: StatsAppointments :many
-- P-12: the appointments of a business that start in [from_at, to_at), every status.
SELECT start_at, professional_id, service_id, customer_id, service_name, status, price_minor, duration_minutes
FROM appointments
WHERE business_id = @business_id AND start_at >= @from_at AND start_at < @to_at
ORDER BY start_at, id;

-- name: FirstCompletedVisits :many
-- P-12: the first COMPLETED appointment in the business of each customer, when it starts in
-- [from_at, to_at).
SELECT a.customer_id::uuid AS customer_id, a.professional_id, a.service_id, a.start_at
FROM appointments a
WHERE a.business_id = @business_id AND a.status = 'COMPLETED' AND a.customer_id IS NOT NULL
  AND a.start_at >= @from_at AND a.start_at < @to_at
  AND NOT EXISTS (
    SELECT 1 FROM appointments e
    WHERE e.business_id = a.business_id AND e.customer_id = a.customer_id AND e.status = 'COMPLETED'
      AND (e.start_at, e.id) < (a.start_at, a.id));
