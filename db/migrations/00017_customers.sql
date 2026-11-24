-- Customers of a business (Phase 6, T6.4), owned by the scheduling module: the people the
-- business books, with or without an account. The mobile number identifies the person inside a
-- business (unique), so a customer created by the staff and the same person booking from the
-- app are one record (user_id links the account when it books).

-- +goose Up
CREATE TABLE customers (
    id          uuid        PRIMARY KEY,
    business_id uuid        NOT NULL REFERENCES businesses (id),
    full_name   text        NOT NULL,
    phone       text        NOT NULL,
    email       text,
    user_id     uuid        REFERENCES users (id),
    created_by  uuid        NOT NULL REFERENCES users (id),
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    CONSTRAINT customers_business_id_unique UNIQUE (business_id, id),
    CONSTRAINT customers_phone_unique UNIQUE (business_id, phone),
    CONSTRAINT customers_full_name_length CHECK (char_length(full_name) BETWEEN 1 AND 120),
    CONSTRAINT customers_phone_e164 CHECK (phone ~ '^\+[1-9][0-9]{6,14}$'),
    CONSTRAINT customers_email_valid CHECK (email IS NULL OR (char_length(email) <= 254 AND email ~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$'))
);
CREATE INDEX customers_business_name_idx ON customers (business_id, lower(full_name));

ALTER TABLE appointments ADD COLUMN customer_id uuid;
ALTER TABLE appointments ADD CONSTRAINT appointments_customer_fk
    FOREIGN KEY (business_id, customer_id) REFERENCES customers (business_id, id);
CREATE INDEX appointments_customer_idx ON appointments (business_id, customer_id, start_at);

-- +goose Down
DROP INDEX appointments_customer_idx;
ALTER TABLE appointments DROP CONSTRAINT appointments_customer_fk;
ALTER TABLE appointments DROP COLUMN customer_id;
DROP TABLE customers;
