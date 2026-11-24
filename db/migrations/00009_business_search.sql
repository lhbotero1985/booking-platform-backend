-- Public search of businesses (Phase 4, T4.6): active businesses of a city ordered by name
-- (keyset pagination on lower(name), id).

-- +goose Up
CREATE INDEX businesses_public_search_idx ON businesses (city_id, lower(name), id) WHERE status = 'ACTIVE';

-- +goose Down
DROP INDEX businesses_public_search_idx;
