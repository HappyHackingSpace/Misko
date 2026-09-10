-- btree_gist lets the exclusion constraint compare uuid equality alongside ranges.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE misko.experiments (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    code             text NOT NULL CHECK (code ~ '^[A-Za-z0-9._/-]{1,64}$'),
    title            text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description      text CHECK (char_length(description) BETWEEN 1 AND 5000),
    requires_control boolean NOT NULL DEFAULT false,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX experiments_code_key ON misko.experiments (lower(code));

-- (experiment_id, id) keys let child records reference a phase or group only
-- together with its own experiment.
CREATE TABLE misko.experiment_phases (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL REFERENCES misko.experiments (id),
    name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    position      integer NOT NULL CHECK (position BETWEEN 1 AND 1000),
    description   text CHECK (char_length(description) BETWEEN 1 AND 5000),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT experiment_phases_scope_key UNIQUE (experiment_id, id),
    CONSTRAINT experiment_phases_position_key UNIQUE (experiment_id, position)
);

CREATE UNIQUE INDEX experiment_phases_name_key ON misko.experiment_phases (experiment_id, lower(name));

CREATE TABLE misko.experiment_groups (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL REFERENCES misko.experiments (id),
    name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    role          text NOT NULL CHECK (role IN ('CONTROL', 'TREATMENT')),
    target_size   integer CHECK (target_size BETWEEN 1 AND 100000),
    description   text CHECK (char_length(description) BETWEEN 1 AND 5000),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT experiment_groups_scope_key UNIQUE (experiment_id, id)
);

CREATE UNIQUE INDEX experiment_groups_name_key ON misko.experiment_groups (experiment_id, lower(name));

CREATE TABLE misko.enrollments (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL REFERENCES misko.experiments (id),
    subject_id    uuid NOT NULL REFERENCES misko.subjects (id),
    enrolled_at   timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT enrollments_subject_key UNIQUE (experiment_id, subject_id),
    CONSTRAINT enrollments_scope_key UNIQUE (experiment_id, id)
);

CREATE INDEX enrollments_subject_idx ON misko.enrollments (subject_id);

-- Dated arm membership. Composite foreign keys keep the enrollment and group in
-- the same experiment; the exclusion constraint forbids overlapping periods,
-- including under concurrent writes.
CREATE TABLE misko.group_assignments (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL,
    enrollment_id uuid NOT NULL,
    group_id      uuid NOT NULL,
    valid_from    timestamptz NOT NULL,
    valid_to      timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT group_assignments_enrollment_fkey FOREIGN KEY (experiment_id, enrollment_id) REFERENCES misko.enrollments (experiment_id, id),
    CONSTRAINT group_assignments_group_fkey FOREIGN KEY (experiment_id, group_id) REFERENCES misko.experiment_groups (experiment_id, id),
    CONSTRAINT group_assignments_period_check CHECK (valid_to IS NULL OR valid_to > valid_from),
    CONSTRAINT group_assignments_no_overlap EXCLUDE USING gist (enrollment_id WITH =, tstzrange(valid_from, valid_to, '[)') WITH &&)
);

CREATE INDEX group_assignments_group_idx ON misko.group_assignments (group_id);
