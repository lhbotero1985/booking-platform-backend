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
