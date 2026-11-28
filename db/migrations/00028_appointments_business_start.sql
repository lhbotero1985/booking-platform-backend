-- P-12: the statistics read the appointments of a business by start time (any professional).

-- +goose Up
CREATE INDEX appointments_business_start_idx ON appointments (business_id, start_at);

-- +goose Down
DROP INDEX appointments_business_start_idx;
