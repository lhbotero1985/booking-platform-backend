-- name: CreateUser :one
INSERT INTO users (id, email, phone, password_hash, full_name, status, locale)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
-- citext: comparison is case-insensitive.
SELECT * FROM users WHERE email = $1;

-- name: UpdateUserPasswordHash :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: UpdateUserStatus :exec
UPDATE users SET status = $2, updated_at = now() WHERE id = $1;
