-- Services each professional performs (Phase 5, T5.6). Owned by the catalog module (the
-- "enabled professionals" of a service, domain-model.md §6). Composite FKs keep the service and
-- the professional inside the same business.

-- +goose Up
CREATE TABLE service_professionals (
    business_id     uuid        NOT NULL,
    service_id      uuid        NOT NULL,
    professional_id uuid        NOT NULL,
    created_at      timestamptz NOT NULL,
    PRIMARY KEY (service_id, professional_id),
    CONSTRAINT service_professionals_service_fk FOREIGN KEY (business_id, service_id)
        REFERENCES services (business_id, id) ON DELETE CASCADE,
    CONSTRAINT service_professionals_professional_fk FOREIGN KEY (business_id, professional_id)
        REFERENCES professionals (business_id, id) ON DELETE CASCADE
);
CREATE INDEX service_professionals_professional_idx ON service_professionals (business_id, professional_id);

-- +goose Down
DROP TABLE service_professionals;
