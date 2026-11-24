-- Businesses (Phase 4, T4.2): the tenant root. Every tenant table references businesses(id).
--
-- Location consistency is enforced by the schema: composite foreign keys guarantee that the city
-- belongs to the division and the division to the country (domain-model.md §4).
-- memberships.business_id gets its foreign key here (RESTRICT: a business with members cannot
-- be deleted). audit_log.business_id deliberately has none: denied attempts are audited with
-- the probed business ID even when it does not exist (decision of 2026-11-23).

-- +goose Up
ALTER TABLE administrative_divisions
    ADD CONSTRAINT administrative_divisions_id_country_unique UNIQUE (id, country_code);
ALTER TABLE cities
    ADD CONSTRAINT cities_id_division_unique UNIQUE (id, division_id);

CREATE TABLE businesses (
    id                            uuid        PRIMARY KEY,
    name                          text        NOT NULL,
    slug                          citext      NOT NULL,
    status                        text        NOT NULL,
    contact_phone                 text        NOT NULL,
    description                   text        NOT NULL DEFAULT '',
    country_code                  char(2)     NOT NULL REFERENCES countries (code),
    division_id                   uuid        NOT NULL,
    city_id                       uuid        NOT NULL,
    address_line                  text        NOT NULL,
    latitude                      double precision,
    longitude                     double precision,
    -- Derived from latitude/longitude so queries never build PostGIS values by hand.
    location                      geography(Point, 4326) GENERATED ALWAYS AS (
        CASE WHEN latitude IS NULL THEN NULL
             ELSE ST_SetSRID(ST_MakePoint(longitude, latitude), 4326)::geography END) STORED,
    time_zone                     text        NOT NULL,
    currency                      char(3)     NOT NULL,
    slot_interval_minutes         smallint    NOT NULL DEFAULT 10,
    buffer_minutes                smallint    NOT NULL DEFAULT 0,
    min_lead_minutes              integer     NOT NULL DEFAULT 60,
    max_advance_days              smallint    NOT NULL DEFAULT 60,
    cancellation_cutoff_hours     smallint    NOT NULL DEFAULT 2,
    reschedule_cutoff_hours       smallint    NOT NULL DEFAULT 2,
    require_customer_confirmation boolean     NOT NULL DEFAULT true,
    reminders_enabled             boolean     NOT NULL DEFAULT true,
    reminder_hours_before         smallint    NOT NULL DEFAULT 24,
    created_at                    timestamptz NOT NULL,
    updated_at                    timestamptz NOT NULL,
    CONSTRAINT businesses_division_in_country
        FOREIGN KEY (division_id, country_code) REFERENCES administrative_divisions (id, country_code),
    CONSTRAINT businesses_city_in_division
        FOREIGN KEY (city_id, division_id) REFERENCES cities (id, division_id),
    -- Both or neither (a CHECK that evaluates to NULL passes, so test nullness explicitly).
    CONSTRAINT businesses_coordinates CHECK (
        (latitude IS NULL) = (longitude IS NULL)
        AND (latitude IS NULL OR (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180))),
    CONSTRAINT businesses_name_length CHECK (char_length(name) BETWEEN 2 AND 120),
    -- citext regex matching ignores case: compare the text form explicitly.
    CONSTRAINT businesses_slug_format CHECK (slug::text ~ '^[a-z0-9]([a-z0-9-]{1,58})[a-z0-9]$' AND slug::text !~ '--'),
    CONSTRAINT businesses_status_valid CHECK (status IN ('PENDING_REVIEW', 'ACTIVE', 'SUSPENDED')),
    CONSTRAINT businesses_phone_e164 CHECK (contact_phone ~ '^\+[1-9][0-9]{6,14}$'),
    CONSTRAINT businesses_description_length CHECK (char_length(description) <= 1000),
    CONSTRAINT businesses_address_length CHECK (char_length(address_line) BETWEEN 1 AND 200),
    CONSTRAINT businesses_time_zone_length CHECK (char_length(time_zone) BETWEEN 1 AND 64),
    CONSTRAINT businesses_currency_format CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT businesses_slot_interval CHECK (slot_interval_minutes IN (5, 10, 15, 20)),
    CONSTRAINT businesses_buffer CHECK (buffer_minutes BETWEEN 0 AND 60),
    CONSTRAINT businesses_min_lead CHECK (min_lead_minutes BETWEEN 0 AND 10080),
    CONSTRAINT businesses_max_advance CHECK (max_advance_days BETWEEN 1 AND 365),
    CONSTRAINT businesses_cancellation_cutoff CHECK (cancellation_cutoff_hours BETWEEN 0 AND 168),
    CONSTRAINT businesses_reschedule_cutoff CHECK (reschedule_cutoff_hours BETWEEN 0 AND 168),
    CONSTRAINT businesses_reminder_hours CHECK (reminder_hours_before BETWEEN 1 AND 72)
);
CREATE UNIQUE INDEX businesses_slug_unique ON businesses (slug);
-- Public search (T4.6) lists active businesses of a city; GIST for future proximity search.
CREATE INDEX businesses_city_status_idx ON businesses (city_id, status);
CREATE INDEX businesses_location_idx ON businesses USING gist (location);

ALTER TABLE memberships
    ADD CONSTRAINT memberships_business_fk FOREIGN KEY (business_id) REFERENCES businesses (id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE memberships DROP CONSTRAINT memberships_business_fk;
DROP TABLE businesses;
ALTER TABLE cities DROP CONSTRAINT cities_id_division_unique;
ALTER TABLE administrative_divisions DROP CONSTRAINT administrative_divisions_id_country_unique;
