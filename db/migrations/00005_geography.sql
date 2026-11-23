-- Geography reference data (Phase 4, T4.1): countries, administrative divisions and cities.
-- Managed only by PLATFORM_ADMIN; read publicly. Each city carries the IANA time zone that
-- businesses in it default to (ADR-0005).
--
-- Seed: pilot country Colombia (P5, decision of 2026-11-23) with Valle del Cauca (Cali, Yumbo)
-- and Antioquia (Medellín, Itagüí). Fixed UUIDv7 identifiers so every environment shares them.

-- +goose Up
CREATE TABLE countries (
    code             char(2)     PRIMARY KEY,
    name             text        NOT NULL,
    phone_code       text        NOT NULL,
    default_currency char(3)     NOT NULL,
    active           boolean     NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    CONSTRAINT countries_code_format CHECK (code ~ '^[A-Z]{2}$'),
    CONSTRAINT countries_name_length CHECK (char_length(name) BETWEEN 1 AND 100),
    CONSTRAINT countries_phone_code_format CHECK (phone_code ~ '^\+[1-9][0-9]{0,3}$'),
    CONSTRAINT countries_currency_format CHECK (default_currency ~ '^[A-Z]{3}$')
);

CREATE TABLE administrative_divisions (
    id           uuid        PRIMARY KEY,
    country_code char(2)     NOT NULL REFERENCES countries (code),
    code         text,
    name         text        NOT NULL,
    type         text        NOT NULL,
    active       boolean     NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    CONSTRAINT administrative_divisions_code_format CHECK (code ~ '^[A-Z]{2}-[A-Z0-9]{1,3}$'),
    CONSTRAINT administrative_divisions_code_country CHECK (code IS NULL OR left(code, 2) = country_code),
    CONSTRAINT administrative_divisions_name_length CHECK (char_length(name) BETWEEN 1 AND 100),
    CONSTRAINT administrative_divisions_type_valid
        CHECK (type IN ('DEPARTMENT', 'STATE', 'PROVINCE', 'REGION', 'DISTRICT'))
);
CREATE UNIQUE INDEX administrative_divisions_code_unique ON administrative_divisions (code);
CREATE UNIQUE INDEX administrative_divisions_name_unique ON administrative_divisions (country_code, lower(name));

CREATE TABLE cities (
    id          uuid        PRIMARY KEY,
    division_id uuid        NOT NULL REFERENCES administrative_divisions (id),
    name        text        NOT NULL,
    time_zone   text        NOT NULL,
    active      boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    CONSTRAINT cities_name_length CHECK (char_length(name) BETWEEN 1 AND 100),
    -- Full IANA validation happens in the application (time.LoadLocation).
    CONSTRAINT cities_time_zone_length CHECK (char_length(time_zone) BETWEEN 1 AND 64)
);
CREATE UNIQUE INDEX cities_name_unique ON cities (division_id, lower(name));

INSERT INTO countries (code, name, phone_code, default_currency, created_at, updated_at)
VALUES ('CO', 'Colombia', '+57', 'COP', now(), now());

INSERT INTO administrative_divisions (id, country_code, code, name, type, created_at, updated_at)
VALUES
    ('01a20aae-7e56-796c-9de9-5409ce51e7d4', 'CO', 'CO-VAC', 'Valle del Cauca', 'DEPARTMENT', now(), now()),
    ('01a20aae-7e5a-7f3e-953c-9ec690399d8d', 'CO', 'CO-ANT', 'Antioquia', 'DEPARTMENT', now(), now());

INSERT INTO cities (id, division_id, name, time_zone, created_at, updated_at)
VALUES
    ('01a20aae-7e5d-73d7-b7d3-a11c5afb9655', '01a20aae-7e56-796c-9de9-5409ce51e7d4', 'Cali', 'America/Bogota', now(), now()),
    ('01a20aae-7e5f-7d08-843b-f2502126e8d7', '01a20aae-7e56-796c-9de9-5409ce51e7d4', 'Yumbo', 'America/Bogota', now(), now()),
    ('01a20aae-7e61-7c3f-898e-2a023921bf54', '01a20aae-7e5a-7f3e-953c-9ec690399d8d', 'Medellín', 'America/Bogota', now(), now()),
    ('01a20aae-7e63-7700-a138-6ae738ef0f2f', '01a20aae-7e5a-7f3e-953c-9ec690399d8d', 'Itagüí', 'America/Bogota', now(), now());

-- +goose Down
DROP TABLE cities;
DROP TABLE administrative_divisions;
DROP TABLE countries;
