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
    duration_minutes, price_minor, currency, version, created_by, created_at, updated_at)
VALUES (@id, @business_id, @professional_id, @service_id, @customer_id, @customer_user_id,
    @customer_name, @customer_phone, @start_at, @end_at, @buffer_minutes, @occupied_until, @status, @service_name,
    @duration_minutes, @price_minor, @currency, @version, @created_by, @created_at, @updated_at);

-- name: InsertAppointmentEvent :exec
INSERT INTO appointment_events (id, business_id, appointment_id, type, status, professional_id, start_at,
    actor_user_id, occurred_at)
VALUES (@id, @business_id, @appointment_id, @type, @status, @professional_id, @start_at, @actor_user_id,
    @occurred_at);

-- name: AppointmentByID :one
SELECT * FROM appointments WHERE id = @id;

-- Active appointments of a professional whose occupied range [start_at, occupied_until)
-- overlaps [from_at, to_at).
-- name: ActiveAppointmentsOverlapping :many
SELECT start_at, end_at, occupied_until FROM appointments
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
