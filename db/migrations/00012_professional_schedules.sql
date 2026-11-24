-- Weekly schedule of a professional (Phase 5, T5.3): working ranges per weekday and recurring
-- breaks, in the business's local time (weekday 0 = Sunday … 6 = Saturday, like Go's
-- time.Weekday). The composite FK keeps every row inside the professional's business. Overlaps
-- and "break inside a working range" are validated by the domain while the professional row is
-- locked (the whole week is replaced in one transaction).

-- +goose Up
CREATE TABLE professional_working_hours (
    id              uuid     PRIMARY KEY,
    business_id     uuid     NOT NULL,
    professional_id uuid     NOT NULL,
    weekday         smallint NOT NULL,
    start_time      time     NOT NULL,
    end_time        time     NOT NULL,
    CONSTRAINT professional_working_hours_professional_fk FOREIGN KEY (business_id, professional_id)
        REFERENCES professionals (business_id, id) ON DELETE CASCADE,
    CONSTRAINT professional_working_hours_weekday_valid CHECK (weekday BETWEEN 0 AND 6),
    CONSTRAINT professional_working_hours_range_valid CHECK (start_time < end_time)
);
CREATE INDEX professional_working_hours_professional_idx
    ON professional_working_hours (business_id, professional_id, weekday, start_time);

CREATE TABLE professional_breaks (
    id              uuid     PRIMARY KEY,
    business_id     uuid     NOT NULL,
    professional_id uuid     NOT NULL,
    weekday         smallint NOT NULL,
    start_time      time     NOT NULL,
    end_time        time     NOT NULL,
    CONSTRAINT professional_breaks_professional_fk FOREIGN KEY (business_id, professional_id)
        REFERENCES professionals (business_id, id) ON DELETE CASCADE,
    CONSTRAINT professional_breaks_weekday_valid CHECK (weekday BETWEEN 0 AND 6),
    CONSTRAINT professional_breaks_range_valid CHECK (start_time < end_time)
);
CREATE INDEX professional_breaks_professional_idx
    ON professional_breaks (business_id, professional_id, weekday, start_time);

-- +goose Down
DROP TABLE professional_breaks;
DROP TABLE professional_working_hours;
