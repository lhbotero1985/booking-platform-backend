-- name: ListExtensions :many
-- Installed PostgreSQL extensions; used to verify the schema baseline (00001_extensions.sql).
SELECT extname::text AS name, extversion AS version
FROM pg_catalog.pg_extension
ORDER BY extname;
