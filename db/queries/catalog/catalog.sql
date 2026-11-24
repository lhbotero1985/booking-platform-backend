-- Catalog module (T5.5). Every query is scoped by business_id (ADR-0007).

-- name: InsertCategory :exec
INSERT INTO service_categories (id, business_id, name, status, sort_order, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $6);

-- name: UpdateCategory :exec
UPDATE service_categories
SET name = $3, status = $4, sort_order = $5, updated_at = $6
WHERE business_id = $1 AND id = $2;

-- name: DeleteCategory :exec
DELETE FROM service_categories WHERE business_id = $1 AND id = $2;

-- name: CategoryByID :one
SELECT id, business_id, name, status, sort_order, created_at, updated_at
FROM service_categories
WHERE business_id = $1 AND id = $2;

-- name: ListCategories :many
SELECT id, business_id, name, status, sort_order, created_at, updated_at
FROM service_categories
WHERE business_id = $1
ORDER BY sort_order, name, id;

-- name: InsertService :exec
INSERT INTO services (id, business_id, category_id, name, description, duration_minutes, price_minor,
                      currency, status, sort_order, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11);

-- name: UpdateService :exec
UPDATE services
SET category_id = $3, name = $4, description = $5, duration_minutes = $6, price_minor = $7,
    status = $8, sort_order = $9, updated_at = $10
WHERE business_id = $1 AND id = $2;

-- name: ServiceByID :one
SELECT id, business_id, category_id, name, description, duration_minutes, price_minor, currency,
       status, sort_order, created_at, updated_at
FROM services
WHERE business_id = $1 AND id = $2;

-- name: ListServices :many
SELECT id, business_id, category_id, name, description, duration_minutes, price_minor, currency,
       status, sort_order, created_at, updated_at
FROM services
WHERE business_id = $1
ORDER BY sort_order, name, id;

-- Services per professional (T5.6).

-- LockProfessionalServices serializes replacements of the same professional's list
-- (transaction-scoped advisory lock).
-- name: LockProfessionalServices :exec
SELECT pg_advisory_xact_lock(hashtextextended('service_professionals:' || sqlc.arg(professional_id)::text, 0));

-- name: DeleteProfessionalServices :exec
DELETE FROM service_professionals WHERE business_id = $1 AND professional_id = $2;

-- name: InsertProfessionalService :exec
INSERT INTO service_professionals (business_id, service_id, professional_id, created_at)
VALUES ($1, $2, $3, $4);

-- name: ServicesOfProfessional :many
SELECT s.id, s.business_id, s.category_id, s.name, s.description, s.duration_minutes, s.price_minor,
       s.currency, s.status, s.sort_order, s.created_at, s.updated_at
FROM service_professionals sp
JOIN services s ON s.business_id = sp.business_id AND s.id = sp.service_id
WHERE sp.business_id = $1 AND sp.professional_id = $2
ORDER BY s.sort_order, s.name, s.id;

-- name: ServicesByIDs :many
SELECT id FROM services WHERE business_id = $1 AND id = ANY(sqlc.arg(ids)::uuid[]);
