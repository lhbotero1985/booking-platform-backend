-- Several OWNERs per business (Phase 4, T4.5, decision of 2026-11-24): the "responsable" of a
-- business is the OWNER role and an OWNER may add more. The invariant becomes "at least one
-- active OWNER", enforced by the identity use cases (they lock the OWNER rows of the business
-- before removing or demoting one).

-- +goose Up
DROP INDEX memberships_one_active_owner_idx;
CREATE INDEX memberships_business_role_idx ON memberships (business_id, role) WHERE status = 'ACTIVE';

-- +goose Down
DROP INDEX memberships_business_role_idx;
-- Fails if a business has several active OWNERs: reduce them first.
CREATE UNIQUE INDEX memberships_one_active_owner_idx
    ON memberships (business_id) WHERE role = 'OWNER' AND status = 'ACTIVE';
