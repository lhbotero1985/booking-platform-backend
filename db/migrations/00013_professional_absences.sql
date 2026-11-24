-- Absences of a professional (Phase 5, T5.4): vacation, sick leave, errands. Absolute periods
-- [starts_at, ends_at) in UTC. Two absences of the same professional never overlap, even under
-- concurrent inserts: exclusion constraint over tstzrange (btree_gist, migration 00001).

-- +goose Up
CREATE TABLE professional_absences (
    id              uuid        PRIMARY KEY,
    business_id     uuid        NOT NULL,
    professional_id uuid        NOT NULL,
    starts_at       timestamptz NOT NULL,
    ends_at         timestamptz NOT NULL,
    reason          text        NOT NULL DEFAULT '',
    created_by      uuid        NOT NULL REFERENCES users (id),
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    CONSTRAINT professional_absences_professional_fk FOREIGN KEY (business_id, professional_id)
        REFERENCES professionals (business_id, id) ON DELETE CASCADE,
    CONSTRAINT professional_absences_period_valid CHECK (starts_at < ends_at),
    CONSTRAINT professional_absences_period_max CHECK (ends_at - starts_at <= interval '366 days'),
    CONSTRAINT professional_absences_reason_length CHECK (char_length(reason) <= 200),
    CONSTRAINT professional_absences_no_overlap EXCLUDE USING gist (
        professional_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    )
);
CREATE INDEX professional_absences_business_idx ON professional_absences (business_id, professional_id, starts_at);

-- +goose Down
DROP TABLE professional_absences;
