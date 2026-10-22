-- Study design: the experiment, its groups and what each group receives, its
-- timepoints, protocols and endpoints, and which animal is in which group.
--
-- Lifecycle: DRAFT -> RUNNING -> COMPLETED. The design (groups, regimens,
-- timepoints, endpoints) can change only in DRAFT; starting sets day 0 and
-- freezes it. Animals can be allocated in DRAFT and RUNNING. A COMPLETED
-- experiment accepts no new rows anywhere.
CREATE TABLE misko.experiments (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    code          text NOT NULL CHECK (code ~ '^[A-Za-z0-9._/-]{1,64}$'),
    title         text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description   text CHECK (char_length(description) BETWEEN 1 AND 5000),
    hypothesis    text CHECK (char_length(hypothesis) BETWEEN 1 AND 2000),
    randomisation text CHECK (char_length(randomisation) BETWEEN 1 AND 500),
    blinded       boolean NOT NULL DEFAULT false,
    status        text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'RUNNING', 'COMPLETED')),
    day_zero      date,
    started_at    timestamptz,
    completed_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT experiments_lifecycle_check CHECK (
        (status = 'DRAFT' AND day_zero IS NULL AND started_at IS NULL AND completed_at IS NULL)
        OR (status = 'RUNNING' AND day_zero IS NOT NULL AND started_at IS NOT NULL AND completed_at IS NULL)
        OR (status = 'COMPLETED' AND day_zero IS NOT NULL AND started_at IS NOT NULL AND completed_at >= started_at))
);

CREATE UNIQUE INDEX experiments_code_key ON misko.experiments (lower(code));

-- Status moves only forward, day 0 never changes once set, a completed
-- experiment never changes, and only a draft can be deleted.
CREATE FUNCTION misko.check_experiment_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status <> 'DRAFT' THEN
            RAISE EXCEPTION 'experiment % is % and cannot be deleted', OLD.id, OLD.status
                USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'experiments_transition';
        END IF;
        RETURN OLD;
    END IF;
    IF OLD.status = 'COMPLETED'
       OR NOT (NEW.status = OLD.status
               OR (OLD.status = 'DRAFT' AND NEW.status = 'RUNNING')
               OR (OLD.status = 'RUNNING' AND NEW.status = 'COMPLETED'))
       OR (OLD.day_zero IS NOT NULL AND NEW.day_zero IS DISTINCT FROM OLD.day_zero)
       OR (OLD.started_at IS NOT NULL AND NEW.started_at IS DISTINCT FROM OLD.started_at)
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'experiment % cannot change from % to %', OLD.id, OLD.status, NEW.status
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'experiments_transition';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER experiments_change BEFORE UPDATE OR DELETE ON misko.experiments
    FOR EACH ROW EXECUTE FUNCTION misko.check_experiment_change();

-- Reads an experiment's status under a share lock, so a concurrent start or
-- completion waits for the writing transaction and the reverse.
CREATE FUNCTION misko.experiment_status(experiment uuid) RETURNS text LANGUAGE sql AS $$
    SELECT status FROM misko.experiments WHERE id = experiment FOR SHARE
$$;

-- Design rows change only while their experiment is a draft.
CREATE FUNCTION misko.check_design_editable() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    experiment uuid := CASE WHEN TG_OP = 'DELETE' THEN OLD.experiment_id ELSE NEW.experiment_id END;
BEGIN
    IF misko.experiment_status(experiment) IS DISTINCT FROM 'DRAFT'
       OR (TG_OP = 'UPDATE' AND NEW.experiment_id IS DISTINCT FROM OLD.experiment_id) THEN
        RAISE EXCEPTION '% of experiment % can change only while it is a draft', TG_TABLE_NAME, experiment
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = TG_TABLE_NAME || '_frozen';
    END IF;
    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;

-- New rows of an experiment's protocols, tests and records need the
-- experiment open (draft or running) or running.
CREATE FUNCTION misko.check_experiment_open() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF misko.experiment_status(NEW.experiment_id) NOT IN ('DRAFT', 'RUNNING') THEN
        RAISE EXCEPTION '% cannot be added to a completed experiment', TG_TABLE_NAME
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = TG_TABLE_NAME || '_closed';
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION misko.check_experiment_running() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF misko.experiment_status(NEW.experiment_id) IS DISTINCT FROM 'RUNNING' THEN
        RAISE EXCEPTION '% needs a running experiment', TG_TABLE_NAME
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = TG_TABLE_NAME || '_not_running';
    END IF;
    RETURN NEW;
END;
$$;

-- A group is defined by its role and disease model; what it receives is its
-- regimen. Only a healthy control has no disease model.
CREATE TABLE misko.experiment_groups (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id    uuid NOT NULL REFERENCES misko.experiments (id),
    code             text NOT NULL CHECK (code ~ '^[A-Za-z0-9._-]{1,16}$'),
    name             text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    role             text NOT NULL CHECK (role IN ('HEALTHY_CONTROL', 'SHAM', 'DISEASE_CONTROL', 'POSITIVE_CONTROL', 'TREATMENT')),
    disease_model_id uuid REFERENCES misko.disease_models (id),
    target_size      integer CHECK (target_size BETWEEN 1 AND 100000),
    description      text CHECK (char_length(description) BETWEEN 1 AND 5000),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT experiment_groups_disease_check CHECK ((role = 'HEALTHY_CONTROL') = (disease_model_id IS NULL)),
    CONSTRAINT experiment_groups_scope_key UNIQUE (experiment_id, id)
);

CREATE UNIQUE INDEX experiment_groups_code_key ON misko.experiment_groups (experiment_id, lower(code));
CREATE UNIQUE INDEX experiment_groups_name_key ON misko.experiment_groups (experiment_id, lower(name));
CREATE INDEX experiment_groups_disease_idx ON misko.experiment_groups (disease_model_id) WHERE disease_model_id IS NOT NULL;
CREATE TRIGGER experiment_groups_editable BEFORE INSERT OR UPDATE OR DELETE ON misko.experiment_groups
    FOR EACH ROW EXECUTE FUNCTION misko.check_design_editable();
CREATE TRIGGER experiment_groups_touch BEFORE UPDATE ON misko.experiment_groups FOR EACH ROW EXECUTE FUNCTION misko.touch();

-- What a group receives: the disease induction, its vehicle or its treatment.
-- Days are study days relative to day 0. A group has at most one treatment
-- item, so every treatment group has one substance at one dose level.
CREATE TABLE misko.regimen_items (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL,
    group_id      uuid NOT NULL,
    purpose       text NOT NULL CHECK (purpose IN ('INDUCTION', 'VEHICLE', 'TREATMENT')),
    substance_id  uuid NOT NULL REFERENCES misko.substances (id),
    dose_micro    bigint NOT NULL CHECK (dose_micro > 0),
    unit          text NOT NULL CHECK (unit IN ('mg/kg', 'ug/kg', 'mL/kg', 'IU/kg', 'mg', 'ug', 'g', 'mL', 'uL', 'IU')),
    route         text NOT NULL CHECK (route IN ('ORAL', 'INTRAPERITONEAL', 'SUBCUTANEOUS', 'INTRAVENOUS', 'INTRAMUSCULAR', 'INTRANASAL',
                                                 'INTRACEREBROVENTRICULAR', 'TOPICAL', 'INHALATION')),
    frequency     text NOT NULL CHECK (frequency IN ('ONCE', 'DAILY', 'TWICE_DAILY', 'WEEKLY', 'EVERY_N_DAYS')),
    interval_days integer CHECK (interval_days BETWEEN 2 AND 365),
    start_day     integer NOT NULL CHECK (start_day BETWEEN -365 AND 3650),
    end_day       integer NOT NULL CHECK (end_day BETWEEN -365 AND 3650),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT regimen_items_group_fkey FOREIGN KEY (experiment_id, group_id) REFERENCES misko.experiment_groups (experiment_id, id),
    CONSTRAINT regimen_items_interval_check CHECK ((frequency = 'EVERY_N_DAYS') = (interval_days IS NOT NULL)),
    CONSTRAINT regimen_items_days_check CHECK (end_day >= start_day AND (frequency <> 'ONCE' OR end_day = start_day)),
    CONSTRAINT regimen_items_group_scope_key UNIQUE (group_id, id)
);

CREATE UNIQUE INDEX regimen_items_treatment_key ON misko.regimen_items (group_id) WHERE purpose = 'TREATMENT';
CREATE INDEX regimen_items_substance_idx ON misko.regimen_items (substance_id);
CREATE TRIGGER regimen_items_editable BEFORE INSERT OR UPDATE OR DELETE ON misko.regimen_items
    FOR EACH ROW EXECUTE FUNCTION misko.check_design_editable();
CREATE TRIGGER regimen_items_touch BEFORE UPDATE ON misko.regimen_items FOR EACH ROW EXECUTE FUNCTION misko.touch();

-- When animals are tested, in study days relative to day 0 (baseline days are
-- negative or zero).
CREATE TABLE misko.timepoints (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL REFERENCES misko.experiments (id),
    name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    study_day     integer NOT NULL CHECK (study_day BETWEEN -365 AND 3650),
    position      integer NOT NULL CHECK (position BETWEEN 1 AND 1000),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT timepoints_position_key UNIQUE (experiment_id, position),
    CONSTRAINT timepoints_scope_key UNIQUE (experiment_id, id)
);

CREATE UNIQUE INDEX timepoints_name_key ON misko.timepoints (experiment_id, lower(name));
CREATE TRIGGER timepoints_editable BEFORE INSERT OR UPDATE OR DELETE ON misko.timepoints
    FOR EACH ROW EXECUTE FUNCTION misko.check_design_editable();
CREATE TRIGGER timepoints_touch BEFORE UPDATE ON misko.timepoints FOR EACH ROW EXECUTE FUNCTION misko.touch();

-- The test battery of an experiment. Versions and steps are immutable; a step
-- names a paradigm version, how many trials of which type to run and its
-- session settings. The arena (environment) is chosen per animal in each test.
CREATE TABLE misko.protocols (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL REFERENCES misko.experiments (id),
    name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    description   text CHECK (char_length(description) BETWEEN 1 AND 5000),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT protocols_scope_key UNIQUE (experiment_id, id)
);

CREATE UNIQUE INDEX protocols_name_key ON misko.protocols (experiment_id, lower(name));
CREATE TRIGGER protocols_touch BEFORE UPDATE ON misko.protocols FOR EACH ROW EXECUTE FUNCTION misko.touch();

CREATE TABLE misko.protocol_versions (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id uuid NOT NULL,
    protocol_id   uuid NOT NULL,
    number        integer NOT NULL CHECK (number >= 1),
    step_count    integer NOT NULL CHECK (step_count BETWEEN 1 AND 50),
    notes         text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    created_by    uuid NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT protocol_versions_protocol_fkey FOREIGN KEY (experiment_id, protocol_id) REFERENCES misko.protocols (experiment_id, id),
    CONSTRAINT protocol_versions_number_key UNIQUE (protocol_id, number),
    CONSTRAINT protocol_versions_scope_key UNIQUE (experiment_id, id)
);

CREATE TABLE misko.protocol_steps (
    protocol_version_id    uuid NOT NULL REFERENCES misko.protocol_versions (id),
    position               integer NOT NULL CHECK (position BETWEEN 1 AND 50),
    paradigm_key           text NOT NULL CHECK (paradigm_key ~ '^[A-Z][A-Z_]{0,39}$'),
    paradigm_version       integer NOT NULL CHECK (paradigm_version >= 1),
    trial_type             text NOT NULL CHECK (trial_type ~ '^[A-Z][A-Z_]{0,39}$'),
    trials                 integer NOT NULL CHECK (trials BETWEEN 1 AND 1000),
    inter_trial_interval_s integer NOT NULL DEFAULT 0 CHECK (inter_trial_interval_s BETWEEN 0 AND 86400),
    session                jsonb NOT NULL CHECK (jsonb_typeof(session) = 'object'),
    notes                  text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    CONSTRAINT protocol_steps_pkey PRIMARY KEY (protocol_version_id, position),
    -- Lets a test copy the paradigm and trial count of its step.
    CONSTRAINT protocol_steps_test_scope_key UNIQUE (protocol_version_id, position, paradigm_key, paradigm_version, trials)
);

CREATE TRIGGER protocols_open BEFORE INSERT ON misko.protocols
    FOR EACH ROW EXECUTE FUNCTION misko.check_experiment_open();
CREATE TRIGGER protocol_versions_open BEFORE INSERT ON misko.protocol_versions
    FOR EACH ROW EXECUTE FUNCTION misko.check_experiment_open();
CREATE TRIGGER protocol_versions_immutable BEFORE UPDATE OR DELETE ON misko.protocol_versions
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER protocol_steps_immutable BEFORE UPDATE OR DELETE ON misko.protocol_steps
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- At commit, a version must have exactly the positions 1 to step_count. Steps
-- added to a version stored by an earlier transaction therefore fail.
CREATE FUNCTION misko.check_protocol_steps() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    version_id uuid;
BEGIN
    IF TG_TABLE_NAME = 'protocol_versions' THEN
        version_id := NEW.id;
    ELSE
        version_id := NEW.protocol_version_id;
    END IF;
    IF (SELECT count(*) FROM misko.protocol_steps s WHERE s.protocol_version_id = version_id)
           IS DISTINCT FROM (SELECT v.step_count FROM misko.protocol_versions v WHERE v.id = version_id)
       OR EXISTS (SELECT 1 FROM misko.protocol_steps s JOIN misko.protocol_versions v ON v.id = s.protocol_version_id
                  WHERE s.protocol_version_id = version_id AND s.position > v.step_count) THEN
        RAISE EXCEPTION 'protocol version % must have steps 1 to its step count', version_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'protocol_steps_complete';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER protocol_versions_steps_complete AFTER INSERT ON misko.protocol_versions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION misko.check_protocol_steps();
CREATE CONSTRAINT TRIGGER protocol_steps_complete AFTER INSERT ON misko.protocol_steps
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION misko.check_protocol_steps();

-- What the study is judged on: one metric of one paradigm at one timepoint,
-- optionally as change from a baseline timepoint. Exactly one is primary once
-- the study starts (checked by the application when starting).
CREATE TABLE misko.endpoints (
    id                    uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id         uuid NOT NULL,
    timepoint_id          uuid NOT NULL,
    baseline_timepoint_id uuid,
    paradigm_key          text NOT NULL CHECK (paradigm_key ~ '^[A-Z][A-Z_]{0,39}$'),
    metric_key            text NOT NULL CHECK (metric_key ~ '^[a-z][a-z0-9_]{0,79}$'),
    kind                  text NOT NULL CHECK (kind IN ('PRIMARY', 'SECONDARY')),
    direction             text NOT NULL CHECK (direction IN ('HIGHER_IS_BETTER', 'LOWER_IS_BETTER')),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT endpoints_timepoint_fkey FOREIGN KEY (experiment_id, timepoint_id) REFERENCES misko.timepoints (experiment_id, id),
    CONSTRAINT endpoints_baseline_fkey FOREIGN KEY (experiment_id, baseline_timepoint_id) REFERENCES misko.timepoints (experiment_id, id),
    CONSTRAINT endpoints_baseline_check CHECK (baseline_timepoint_id IS DISTINCT FROM timepoint_id),
    CONSTRAINT endpoints_measure_key UNIQUE (experiment_id, timepoint_id, paradigm_key, metric_key)
);

CREATE UNIQUE INDEX endpoints_primary_key ON misko.endpoints (experiment_id) WHERE kind = 'PRIMARY';
CREATE TRIGGER endpoints_editable BEFORE INSERT OR UPDATE OR DELETE ON misko.endpoints
    FOR EACH ROW EXECUTE FUNCTION misko.check_design_editable();
CREATE TRIGGER endpoints_touch BEFORE UPDATE ON misko.endpoints FOR EACH ROW EXECUTE FUNCTION misko.touch();

-- Which group an animal is in. An animal belongs to exactly one group, ever.
-- It is excluded, never deleted, once its study runs.
CREATE TABLE misko.allocations (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id    uuid NOT NULL,
    group_id         uuid NOT NULL,
    subject_id       uuid NOT NULL REFERENCES misko.subjects (id),
    allocated_at     timestamptz NOT NULL,
    cage             text CHECK (char_length(cage) BETWEEN 1 AND 32),
    excluded_at      timestamptz,
    exclusion_reason text CHECK (exclusion_reason IN ('DIED', 'HUMANE_ENDPOINT', 'INDUCTION_FAILED', 'TECHNICAL', 'OTHER')),
    exclusion_note   text CHECK (char_length(exclusion_note) BETWEEN 1 AND 2000),
    created_by       uuid NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT allocations_group_fkey FOREIGN KEY (experiment_id, group_id) REFERENCES misko.experiment_groups (experiment_id, id),
    CONSTRAINT allocations_subject_key UNIQUE (subject_id),
    CONSTRAINT allocations_exclusion_check CHECK (
        (excluded_at IS NULL AND exclusion_reason IS NULL AND exclusion_note IS NULL)
        OR (excluded_at IS NOT NULL AND exclusion_reason IS NOT NULL AND excluded_at >= allocated_at)),
    CONSTRAINT allocations_scope_key UNIQUE (experiment_id, id),
    CONSTRAINT allocations_group_scope_key UNIQUE (id, group_id),
    CONSTRAINT allocations_subject_scope_key UNIQUE (id, subject_id)
);

CREATE INDEX allocations_group_idx ON misko.allocations (group_id);

-- Allocating needs a draft or running study. In a draft an animal may move or
-- leave; once the study runs its group is fixed and only the cage and a single
-- exclusion can be recorded.
CREATE FUNCTION misko.check_allocation_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    status text := misko.experiment_status(CASE WHEN TG_OP = 'DELETE' THEN OLD.experiment_id ELSE NEW.experiment_id END);
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF status NOT IN ('DRAFT', 'RUNNING') THEN
            RAISE EXCEPTION 'animals can be allocated only to a draft or running experiment'
                USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'allocations_frozen';
        END IF;
        RETURN NEW;
    END IF;
    IF status = 'DRAFT' AND (TG_OP = 'DELETE' OR NEW.experiment_id = OLD.experiment_id) THEN
        RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
    END IF;
    IF TG_OP = 'DELETE' OR status <> 'RUNNING'
       OR ROW(NEW.experiment_id, NEW.group_id, NEW.subject_id, NEW.allocated_at, NEW.created_by, NEW.created_at)
          IS DISTINCT FROM ROW(OLD.experiment_id, OLD.group_id, OLD.subject_id, OLD.allocated_at, OLD.created_by, OLD.created_at)
       OR (OLD.excluded_at IS NOT NULL AND ROW(NEW.excluded_at, NEW.exclusion_reason, NEW.exclusion_note)
           IS DISTINCT FROM ROW(OLD.excluded_at, OLD.exclusion_reason, OLD.exclusion_note)) THEN
        RAISE EXCEPTION 'allocation % cannot change while its experiment is %', OLD.id, status
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'allocations_frozen';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER allocations_change BEFORE INSERT OR UPDATE OR DELETE ON misko.allocations
    FOR EACH ROW EXECUTE FUNCTION misko.check_allocation_change();
