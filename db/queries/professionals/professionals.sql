-- Professionals module (T5.2). Every query is scoped by business_id (ADR-0007).

-- name: InsertProfessional :exec
INSERT INTO professionals (id, business_id, user_id, display_name, bio, status, sort_order, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8);

-- name: UpdateProfessional :exec
UPDATE professionals
SET display_name = $3, bio = $4, status = $5, sort_order = $6, updated_at = $7
WHERE business_id = $1 AND id = $2;

-- name: ProfessionalByID :one
SELECT id, business_id, user_id, display_name, bio, status, sort_order, created_at, updated_at
FROM professionals
WHERE business_id = $1 AND id = $2;

-- name: ListProfessionals :many
SELECT id, business_id, user_id, display_name, bio, status, sort_order, created_at, updated_at
FROM professionals
WHERE business_id = $1
ORDER BY sort_order, display_name, id;
