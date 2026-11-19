-- Extensions required by the data model (docs/architecture/database.md):
--   postgis    : business location (geography Point)
--   btree_gist : exclusion constraints mixing "=" and "&&" (double booking guard, ADR-0004)
--   citext     : case-insensitive emails and slugs

-- +goose Up
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose Down
-- postgis is intentionally NOT dropped: the postgis/postgis image (and managed providers)
-- pre-install it together with dependent extensions (postgis_topology,
-- postgis_tiger_geocoder), so dropping it would fail or remove objects this migration does
-- not own.
DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS btree_gist;
