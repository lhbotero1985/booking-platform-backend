-- Businesses module (T4.2). businesses is the tenant root: other tenant tables filter by
-- business_id; here the row itself is the business.

-- name: InsertBusiness :exec
INSERT INTO businesses (
    id, name, slug, status, contact_phone, description, country_code, division_id, city_id,
    address_line, latitude, longitude, time_zone, currency, slot_interval_minutes, buffer_minutes,
    min_lead_minutes, max_advance_days, cancellation_cutoff_hours, reschedule_cutoff_hours,
    require_customer_confirmation, reminders_enabled, reminder_hours_before, max_customer_reschedules,
    created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
        $21, $22, $23, $24, $25, $25);

-- name: UpdateBusiness :exec
-- Everything but id, slug, currency and created_at (slug changes and moves between countries
-- are separate, audited operations).
UPDATE businesses SET
    name = $2, status = $3, contact_phone = $4, description = $5, division_id = $6, city_id = $7,
    address_line = $8, latitude = $9, longitude = $10, time_zone = $11, slot_interval_minutes = $12,
    buffer_minutes = $13, min_lead_minutes = $14, max_advance_days = $15,
    cancellation_cutoff_hours = $16, reschedule_cutoff_hours = $17,
    require_customer_confirmation = $18, reminders_enabled = $19, reminder_hours_before = $20,
    max_customer_reschedules = $21, updated_at = $22
WHERE id = $1;

-- name: BusinessByID :one
SELECT id, name, slug::text AS slug, status, contact_phone, description, country_code, division_id,
       city_id, address_line, latitude, longitude, time_zone, currency, slot_interval_minutes,
       buffer_minutes, min_lead_minutes, max_advance_days, cancellation_cutoff_hours,
       reschedule_cutoff_hours, require_customer_confirmation, reminders_enabled,
       reminder_hours_before, max_customer_reschedules, created_at, updated_at
FROM businesses
WHERE id = $1;

-- name: BusinessBySlug :one
SELECT id, name, slug::text AS slug, status, contact_phone, description, country_code, division_id,
       city_id, address_line, latitude, longitude, time_zone, currency, slot_interval_minutes,
       buffer_minutes, min_lead_minutes, max_advance_days, cancellation_cutoff_hours,
       reschedule_cutoff_hours, require_customer_confirmation, reminders_enabled,
       reminder_hours_before, max_customer_reschedules, created_at, updated_at
FROM businesses
WHERE slug = $1;

-- name: SlugTaken :one
SELECT EXISTS (SELECT 1 FROM businesses WHERE slug = $1) AS taken;

-- name: CountActiveBusinessesInCountry :one
SELECT count(*) FROM businesses WHERE country_code = $1 AND status = 'ACTIVE';

-- name: CountActiveBusinessesInDivision :one
SELECT count(*) FROM businesses WHERE division_id = $1 AND status = 'ACTIVE';

-- name: CountActiveBusinessesInCity :one
SELECT count(*) FROM businesses WHERE city_id = $1 AND status = 'ACTIVE';

-- name: ListBusinesses :many
-- Administrator list: optional status and city filters, keyset pagination by id (UUIDv7, so
-- roughly by creation time).
SELECT id, name, slug::text AS slug, status, contact_phone, description, country_code, division_id,
       city_id, address_line, latitude, longitude, time_zone, currency, slot_interval_minutes,
       buffer_minutes, min_lead_minutes, max_advance_days, cancellation_cutoff_hours,
       reschedule_cutoff_hours, require_customer_confirmation, reminders_enabled,
       reminder_hours_before, max_customer_reschedules, created_at, updated_at
FROM businesses
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(city_id)::uuid IS NULL OR city_id = sqlc.narg(city_id)::uuid)
  AND (sqlc.narg(after)::uuid IS NULL OR id > sqlc.narg(after)::uuid)
ORDER BY id
LIMIT sqlc.arg(max_rows);

-- name: SearchActiveBusinesses :many
-- Public search (T4.6): ACTIVE businesses of a city, optional name filter (the caller escapes
-- LIKE wildcards), keyset pagination after (lower(name), id). Entitlement (demo or payment) is
-- filtered by the subscriptions module. sort_name is the keyset (PostgreSQL's lower()).
SELECT lower(name) AS sort_name, id, name, slug::text AS slug, status, contact_phone, description, country_code, division_id,
       city_id, address_line, latitude, longitude, time_zone, currency, slot_interval_minutes,
       buffer_minutes, min_lead_minutes, max_advance_days, cancellation_cutoff_hours,
       reschedule_cutoff_hours, require_customer_confirmation, reminders_enabled,
       reminder_hours_before, max_customer_reschedules, created_at, updated_at
FROM businesses
WHERE status = 'ACTIVE'
  AND city_id = sqlc.arg(city_id)
  AND (sqlc.narg(name_like)::text IS NULL OR name ILIKE sqlc.narg(name_like)::text ESCAPE '\')
  AND (sqlc.narg(after_name)::text IS NULL OR (lower(name), id) > (sqlc.narg(after_name)::text, sqlc.narg(after_id)::uuid))
ORDER BY lower(name), id
LIMIT sqlc.arg(max_rows);

-- name: ClaimCreationKey :many
INSERT INTO business_creation_keys (user_id, key, fingerprint, created_at)
VALUES (@user_id, @key, @fingerprint, @created_at)
ON CONFLICT (user_id, key) DO NOTHING
RETURNING user_id;

-- name: CreationKeyOf :one
SELECT fingerprint, business_id FROM business_creation_keys WHERE user_id = @user_id AND key = @key;

-- name: SetCreationKeyBusiness :exec
UPDATE business_creation_keys SET business_id = @business_id WHERE user_id = @user_id AND key = @key;
