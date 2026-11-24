-- Businesses module (T4.2). businesses is the tenant root: other tenant tables filter by
-- business_id; here the row itself is the business.

-- name: InsertBusiness :exec
INSERT INTO businesses (
    id, name, slug, status, contact_phone, description, country_code, division_id, city_id,
    address_line, latitude, longitude, time_zone, currency, slot_interval_minutes, buffer_minutes,
    min_lead_minutes, max_advance_days, cancellation_cutoff_hours, reschedule_cutoff_hours,
    require_customer_confirmation, reminders_enabled, reminder_hours_before, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
        $21, $22, $23, $24, $24);

-- name: UpdateBusiness :exec
-- Everything but id, slug, currency and created_at (slug changes and moves between countries
-- are separate, audited operations).
UPDATE businesses SET
    name = $2, status = $3, contact_phone = $4, description = $5, division_id = $6, city_id = $7,
    address_line = $8, latitude = $9, longitude = $10, time_zone = $11, slot_interval_minutes = $12,
    buffer_minutes = $13, min_lead_minutes = $14, max_advance_days = $15,
    cancellation_cutoff_hours = $16, reschedule_cutoff_hours = $17,
    require_customer_confirmation = $18, reminders_enabled = $19, reminder_hours_before = $20,
    updated_at = $21
WHERE id = $1;

-- name: BusinessByID :one
SELECT id, name, slug::text AS slug, status, contact_phone, description, country_code, division_id,
       city_id, address_line, latitude, longitude, time_zone, currency, slot_interval_minutes,
       buffer_minutes, min_lead_minutes, max_advance_days, cancellation_cutoff_hours,
       reschedule_cutoff_hours, require_customer_confirmation, reminders_enabled,
       reminder_hours_before, created_at, updated_at
FROM businesses
WHERE id = $1;

-- name: BusinessBySlug :one
SELECT id, name, slug::text AS slug, status, contact_phone, description, country_code, division_id,
       city_id, address_line, latitude, longitude, time_zone, currency, slot_interval_minutes,
       buffer_minutes, min_lead_minutes, max_advance_days, cancellation_cutoff_hours,
       reschedule_cutoff_hours, require_customer_confirmation, reminders_enabled,
       reminder_hours_before, created_at, updated_at
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
