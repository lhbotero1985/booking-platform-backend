-- Catalog (Phase 5, T5.5): optional service categories and the services of a business. A
-- service sets its own duration (the same for every professional) and price in the business's
-- currency, in minor units without cents (decision of 2026-11-24). A combo such as
-- "corte + barba" is an ordinary service with its own price and duration. No global catalog.

-- +goose Up
CREATE TABLE service_categories (
    id          uuid        PRIMARY KEY,
    business_id uuid        NOT NULL REFERENCES businesses (id) ON DELETE CASCADE,
    name        text        NOT NULL,
    status      text        NOT NULL,
    sort_order  integer     NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    CONSTRAINT service_categories_business_id_unique UNIQUE (business_id, id),
    CONSTRAINT service_categories_name_length CHECK (char_length(name) BETWEEN 2 AND 60),
    CONSTRAINT service_categories_status_valid CHECK (status IN ('ACTIVE', 'INACTIVE')),
    CONSTRAINT service_categories_sort_order_range CHECK (sort_order BETWEEN 0 AND 1000)
);
CREATE UNIQUE INDEX service_categories_name_unique ON service_categories (business_id, lower(name));

CREATE TABLE services (
    id               uuid        PRIMARY KEY,
    business_id      uuid        NOT NULL REFERENCES businesses (id) ON DELETE CASCADE,
    category_id      uuid,
    name             text        NOT NULL,
    description      text        NOT NULL DEFAULT '',
    duration_minutes integer     NOT NULL,
    price_minor      bigint      NOT NULL,
    currency         char(3)     NOT NULL,
    status           text        NOT NULL,
    sort_order       integer     NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL,
    CONSTRAINT services_business_id_unique UNIQUE (business_id, id), -- composite FKs (service_professionals, appointments)
    -- Deleting a category leaves its services uncategorized.
    CONSTRAINT services_category_fk FOREIGN KEY (business_id, category_id)
        REFERENCES service_categories (business_id, id) ON DELETE SET NULL (category_id),
    CONSTRAINT services_name_length CHECK (char_length(name) BETWEEN 2 AND 80),
    CONSTRAINT services_description_length CHECK (char_length(description) <= 500),
    CONSTRAINT services_duration_valid CHECK (duration_minutes BETWEEN 5 AND 480 AND duration_minutes % 5 = 0),
    CONSTRAINT services_price_valid CHECK (price_minor BETWEEN 0 AND 1000000000 AND price_minor % 100 = 0),
    CONSTRAINT services_currency_format CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT services_status_valid CHECK (status IN ('ACTIVE', 'INACTIVE')),
    CONSTRAINT services_sort_order_range CHECK (sort_order BETWEEN 0 AND 1000)
);
CREATE UNIQUE INDEX services_name_unique ON services (business_id, lower(name));
CREATE INDEX services_business_order_idx ON services (business_id, sort_order, name);

-- +goose Down
DROP TABLE services;
DROP TABLE service_categories;
