-- Geography module (T4.1). Reference data: not scoped by business_id.
-- Uniqueness is enforced by the indexes; the store maps unique violations to ErrConflict.

-- name: ListActiveCountries :many
SELECT code, name, phone_code, default_currency, active
FROM countries
WHERE active
ORDER BY name;

-- name: CountryByCode :one
SELECT code, name, phone_code, default_currency, active
FROM countries
WHERE code = $1;

-- name: InsertCountry :exec
INSERT INTO countries (code, name, phone_code, default_currency, active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $6);

-- name: UpdateCountry :exec
UPDATE countries
SET name = $2, phone_code = $3, default_currency = $4, active = $5, updated_at = $6
WHERE code = $1;

-- name: ListActiveDivisionsByCountry :many
SELECT id, country_code, code, name, type, active
FROM administrative_divisions
WHERE country_code = $1 AND active
ORDER BY name;

-- name: DivisionByID :one
SELECT id, country_code, code, name, type, active
FROM administrative_divisions
WHERE id = $1;

-- name: InsertDivision :exec
INSERT INTO administrative_divisions (id, country_code, code, name, type, active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $7);

-- name: UpdateDivision :exec
UPDATE administrative_divisions
SET code = $2, name = $3, type = $4, active = $5, updated_at = $6
WHERE id = $1;

-- name: ListActiveCitiesByDivision :many
SELECT id, division_id, name, time_zone, active
FROM cities
WHERE division_id = $1 AND active
ORDER BY name;

-- name: CityByID :one
SELECT id, division_id, name, time_zone, active
FROM cities
WHERE id = $1;

-- name: InsertCity :exec
INSERT INTO cities (id, division_id, name, time_zone, active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $6);

-- name: UpdateCity :exec
UPDATE cities
SET name = $2, time_zone = $3, active = $4, updated_at = $5
WHERE id = $1;
