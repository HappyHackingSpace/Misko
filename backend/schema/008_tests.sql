-- Lets a test copy the paradigm, environment revision and planned trials of
-- its protocol step, enforced by a composite foreign key.
ALTER TABLE misko.protocol_steps
    ADD CONSTRAINT protocol_steps_test_scope_key UNIQUE (protocol_version_id, position, paradigm_key, paradigm_version, environment_revision_id, trials);

-- One session of a subject on one protocol step. Context columns never change;
-- only the status moves forward, with times matching the status.
CREATE TABLE misko.tests (
    id                      uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id           uuid NOT NULL,
    enrollment_id           uuid NOT NULL,
    subject_id              uuid NOT NULL,
    phase_id                uuid,
    group_id                uuid,
    protocol_version_id     uuid NOT NULL,
    step_position           integer NOT NULL,
    paradigm_key            text NOT NULL,
    paradigm_version        integer NOT NULL,
    environment_revision_id uuid NOT NULL,
    planned_trials          integer NOT NULL,
    status                  text NOT NULL DEFAULT 'PLANNED' CHECK (status IN ('PLANNED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED')),
    scheduled_at            timestamptz NOT NULL,
    started_at              timestamptz,
    completed_at            timestamptz,
    cancelled_at            timestamptz,
    cancel_reason           text CHECK (char_length(cancel_reason) BETWEEN 1 AND 2000),
    notes                   text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    created_by              uuid NOT NULL,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tests_enrollment_fkey FOREIGN KEY (experiment_id, enrollment_id) REFERENCES misko.enrollments (experiment_id, id),
    CONSTRAINT tests_subject_fkey FOREIGN KEY (subject_id, enrollment_id) REFERENCES misko.enrollments (subject_id, id),
    CONSTRAINT tests_phase_fkey FOREIGN KEY (experiment_id, phase_id) REFERENCES misko.experiment_phases (experiment_id, id),
    CONSTRAINT tests_group_fkey FOREIGN KEY (experiment_id, group_id) REFERENCES misko.experiment_groups (experiment_id, id),
    CONSTRAINT tests_protocol_version_fkey FOREIGN KEY (experiment_id, protocol_version_id) REFERENCES misko.protocol_versions (experiment_id, id),
    CONSTRAINT tests_step_fkey FOREIGN KEY (protocol_version_id, step_position, paradigm_key, paradigm_version, environment_revision_id, planned_trials)
        REFERENCES misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, environment_revision_id, trials),
    CONSTRAINT tests_status_times_check CHECK (
        (status = 'PLANNED' AND started_at IS NULL AND completed_at IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL)
        OR (status = 'IN_PROGRESS' AND started_at IS NOT NULL AND completed_at IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL)
        OR (status = 'COMPLETED' AND started_at IS NOT NULL AND completed_at >= started_at AND cancelled_at IS NULL AND cancel_reason IS NULL)
        OR (status = 'CANCELLED' AND completed_at IS NULL AND cancelled_at IS NOT NULL AND cancel_reason IS NOT NULL)),
    CONSTRAINT tests_scope_key UNIQUE (experiment_id, id)
);

CREATE INDEX tests_experiment_idx ON misko.tests (experiment_id, scheduled_at, id);
CREATE INDEX tests_subject_idx ON misko.tests (subject_id, scheduled_at);

-- Rejects deletes, context changes and status changes other than
-- PLANNED -> IN_PROGRESS | CANCELLED and IN_PROGRESS -> COMPLETED | CANCELLED.
CREATE FUNCTION misko.check_test_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'tests cannot be deleted'
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'tests_immutable';
    END IF;
    IF ROW(NEW.experiment_id, NEW.enrollment_id, NEW.subject_id, NEW.phase_id, NEW.group_id, NEW.protocol_version_id, NEW.step_position,
           NEW.paradigm_key, NEW.paradigm_version, NEW.environment_revision_id, NEW.planned_trials, NEW.scheduled_at, NEW.notes, NEW.created_by, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.experiment_id, OLD.enrollment_id, OLD.subject_id, OLD.phase_id, OLD.group_id, OLD.protocol_version_id, OLD.step_position,
           OLD.paradigm_key, OLD.paradigm_version, OLD.environment_revision_id, OLD.planned_trials, OLD.scheduled_at, OLD.notes, OLD.created_by, OLD.created_at)
       OR NOT ((OLD.status = 'PLANNED' AND NEW.status IN ('IN_PROGRESS', 'CANCELLED'))
               OR (OLD.status = 'IN_PROGRESS' AND NEW.status IN ('COMPLETED', 'CANCELLED')))
       OR (OLD.started_at IS NOT NULL AND NEW.started_at IS DISTINCT FROM OLD.started_at) THEN
        RAISE EXCEPTION 'test % cannot change from % to %', OLD.id, OLD.status, NEW.status
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'tests_transition';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER tests_change BEFORE UPDATE OR DELETE ON misko.tests
    FOR EACH ROW EXECUTE FUNCTION misko.check_test_change();

-- Trials are appended; a repeated repetition gets the next attempt.
CREATE TABLE misko.trials (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    test_id     uuid NOT NULL REFERENCES misko.tests (id),
    number      integer NOT NULL CHECK (number >= 1),
    repetition  integer NOT NULL CHECK (repetition >= 1),
    attempt     integer NOT NULL CHECK (attempt >= 1),
    started_at  timestamptz NOT NULL,
    ended_at    timestamptz,
    notes       text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    recorded_by uuid NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT trials_end_check CHECK (ended_at IS NULL OR ended_at > started_at),
    CONSTRAINT trials_number_key UNIQUE (test_id, number),
    CONSTRAINT trials_attempt_key UNIQUE (test_id, repetition, attempt)
);

CREATE TRIGGER trials_immutable BEFORE UPDATE OR DELETE ON misko.trials
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- A trial needs a test in progress and a planned repetition. The shared row
-- lock waits for a concurrent completion or cancellation to finish.
CREATE FUNCTION misko.check_trial_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    test_status text;
    planned     integer;
BEGIN
    SELECT t.status, t.planned_trials INTO test_status, planned FROM misko.tests t WHERE t.id = NEW.test_id FOR SHARE;
    IF test_status IS DISTINCT FROM 'IN_PROGRESS' OR NEW.repetition > planned THEN
        RAISE EXCEPTION 'trials need a test in progress and a planned repetition'
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'trials_test_in_progress';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trials_test_in_progress BEFORE INSERT ON misko.trials
    FOR EACH ROW EXECUTE FUNCTION misko.check_trial_insert();

-- author_id keeps the author's id even if that user is later deleted.
CREATE TABLE misko.test_comments (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    test_id    uuid NOT NULL REFERENCES misko.tests (id),
    author_id  uuid NOT NULL,
    body       text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX test_comments_test_idx ON misko.test_comments (test_id, created_at, id);
