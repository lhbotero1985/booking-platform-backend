-- Roles of a business (Phase 5, T5.1, decision of 2026-11-24): the actors are the platform
-- administrator (users.is_platform_admin), the person in charge of the business (OWNER), the
-- barbers/stylists (PROFESSIONAL) and the customers (no membership). The MANAGER role is removed;
-- any existing MANAGER membership (QA test data only) becomes PROFESSIONAL.

-- +goose Up
UPDATE memberships SET role = 'PROFESSIONAL', updated_at = now() WHERE role = 'MANAGER';
ALTER TABLE memberships DROP CONSTRAINT memberships_role_valid;
ALTER TABLE memberships ADD CONSTRAINT memberships_role_valid CHECK (role IN ('OWNER', 'PROFESSIONAL'));

-- +goose Down
ALTER TABLE memberships DROP CONSTRAINT memberships_role_valid;
ALTER TABLE memberships ADD CONSTRAINT memberships_role_valid CHECK (role IN ('OWNER', 'MANAGER', 'PROFESSIONAL'));
