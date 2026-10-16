-- Laboratory animals. A subject belongs to one group of one experiment through
-- its allocation (006_experiments.sql); it can be registered before that.
CREATE TABLE misko.subjects (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    code       text NOT NULL CHECK (code ~ '^[A-Za-z0-9._/-]{1,64}$'),
    species    text NOT NULL CHECK (species IN ('MOUSE', 'RAT')),
    sex        text NOT NULL CHECK (sex IN ('FEMALE', 'MALE', 'UNKNOWN')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX subjects_code_key ON misko.subjects (lower(code));
CREATE TRIGGER subjects_touch BEFORE UPDATE ON misko.subjects FOR EACH ROW EXECUTE FUNCTION misko.touch();

-- Body weights belong to the animal, not to a study, and never change.
CREATE TABLE misko.weight_measurements (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    subject_id      uuid NOT NULL REFERENCES misko.subjects (id),
    body_milligrams bigint NOT NULL CHECK (body_milligrams BETWEEN 1 AND 5000000),
    measured_at     timestamptz NOT NULL,
    recorded_by     uuid NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT weight_measurements_subject_scope_key UNIQUE (subject_id, id)
);

CREATE INDEX weight_measurements_subject_idx ON misko.weight_measurements (subject_id, measured_at);
CREATE TRIGGER weight_measurements_immutable BEFORE UPDATE OR DELETE ON misko.weight_measurements
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
