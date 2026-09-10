-- Lets conditions and administrations reference an enrollment only together
-- with the subject it belongs to.
ALTER TABLE misko.enrollments ADD CONSTRAINT enrollments_subject_scope_key UNIQUE (subject_id, id);

CREATE TABLE misko.disease_models (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    description text CHECK (char_length(description) BETWEEN 1 AND 5000),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX disease_models_name_key ON misko.disease_models (lower(name));

CREATE TABLE misko.substances (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    description text CHECK (char_length(description) BETWEEN 1 AND 5000),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX substances_name_key ON misko.substances (lower(name));

-- Amounts are fixed-point: amount_micro is the dose multiplied by 1,000,000.
CREATE TABLE misko.intervention_plans (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL REFERENCES misko.experiments (id),
    group_id      uuid NOT NULL,
    phase_id      uuid,
    substance_id  uuid NOT NULL REFERENCES misko.substances (id),
    amount_micro  bigint NOT NULL CHECK (amount_micro > 0),
    unit          text NOT NULL CHECK (unit IN ('mg', 'ug', 'g', 'mL', 'uL', 'IU', 'mg/kg', 'ug/kg', 'IU/kg')),
    route         text NOT NULL CHECK (route IN ('ORAL', 'INTRAPERITONEAL', 'SUBCUTANEOUS', 'INTRAVENOUS', 'INTRAMUSCULAR', 'INTRANASAL', 'INTRACEREBROVENTRICULAR', 'TOPICAL', 'INHALATION')),
    schedule      text NOT NULL CHECK (char_length(schedule) BETWEEN 1 AND 500),
    notes         text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT intervention_plans_scope_key UNIQUE (experiment_id, id),
    CONSTRAINT intervention_plans_group_fkey FOREIGN KEY (experiment_id, group_id) REFERENCES misko.experiment_groups (experiment_id, id),
    CONSTRAINT intervention_plans_phase_fkey FOREIGN KEY (experiment_id, phase_id) REFERENCES misko.experiment_phases (experiment_id, id)
);

-- Measurements, conditions and administrations are append-only records.
-- recorded_by keeps the author's id even if that user is later deleted.
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

-- disease_model_name is the name at recording time, so renaming a model does
-- not rewrite history. Induction and confirmation are separate rows.
CREATE TABLE misko.subject_conditions (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    subject_id         uuid NOT NULL REFERENCES misko.subjects (id),
    disease_model_id   uuid NOT NULL REFERENCES misko.disease_models (id),
    disease_model_name text NOT NULL,
    enrollment_id      uuid,
    status             text NOT NULL CHECK (status IN ('INDUCED', 'CONFIRMED', 'NOT_CONFIRMED', 'RESOLVED')),
    observed_at        timestamptz NOT NULL,
    notes              text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    recorded_by        uuid NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT subject_conditions_enrollment_fkey FOREIGN KEY (subject_id, enrollment_id) REFERENCES misko.enrollments (subject_id, id)
);

CREATE INDEX subject_conditions_subject_idx ON misko.subject_conditions (subject_id, observed_at);
CREATE INDEX subject_conditions_model_idx ON misko.subject_conditions (disease_model_id, observed_at);

-- An actual administration. Dose, route and substance name are copied at
-- recording time; later plan or substance edits never change this row.
CREATE TABLE misko.administrations (
    id                    uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id         uuid NOT NULL,
    enrollment_id         uuid NOT NULL,
    subject_id            uuid NOT NULL,
    substance_id          uuid NOT NULL REFERENCES misko.substances (id),
    substance_name        text NOT NULL,
    plan_id               uuid,
    weight_measurement_id uuid,
    body_milligrams       bigint,
    amount_micro          bigint NOT NULL CHECK (amount_micro > 0),
    unit                  text NOT NULL CHECK (unit IN ('mg', 'ug', 'g', 'mL', 'uL', 'IU', 'mg/kg', 'ug/kg', 'IU/kg')),
    route                 text NOT NULL CHECK (route IN ('ORAL', 'INTRAPERITONEAL', 'SUBCUTANEOUS', 'INTRAVENOUS', 'INTRAMUSCULAR', 'INTRANASAL', 'INTRACEREBROVENTRICULAR', 'TOPICAL', 'INHALATION')),
    administered_at       timestamptz NOT NULL,
    notes                 text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    recorded_by           uuid NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT administrations_enrollment_fkey FOREIGN KEY (experiment_id, enrollment_id) REFERENCES misko.enrollments (experiment_id, id),
    CONSTRAINT administrations_subject_fkey FOREIGN KEY (subject_id, enrollment_id) REFERENCES misko.enrollments (subject_id, id),
    CONSTRAINT administrations_plan_fkey FOREIGN KEY (experiment_id, plan_id) REFERENCES misko.intervention_plans (experiment_id, id),
    CONSTRAINT administrations_weight_fkey FOREIGN KEY (subject_id, weight_measurement_id) REFERENCES misko.weight_measurements (subject_id, id),
    CONSTRAINT administrations_weight_snapshot_check CHECK ((weight_measurement_id IS NULL) = (body_milligrams IS NULL)),
    CONSTRAINT administrations_per_kg_weight_check CHECK (unit NOT LIKE '%/kg' OR weight_measurement_id IS NOT NULL)
);

CREATE INDEX administrations_subject_idx ON misko.administrations (subject_id, administered_at);
CREATE INDEX administrations_substance_idx ON misko.administrations (substance_id, administered_at);
CREATE INDEX administrations_experiment_idx ON misko.administrations (experiment_id, administered_at);
