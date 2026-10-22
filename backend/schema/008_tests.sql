-- A test is one session of a running experiment: one timepoint, one protocol
-- step, one video. The animals filmed in it are its test subjects, each in its
-- own arena (an environment revision of the step's paradigm). Usually one
-- animal; sometimes several arenas share the camera frame.
CREATE TABLE misko.tests (
    id                  uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id       uuid NOT NULL,
    timepoint_id        uuid NOT NULL,
    protocol_version_id uuid NOT NULL,
    step_position       integer NOT NULL,
    paradigm_key        text NOT NULL,
    paradigm_version    integer NOT NULL,
    planned_trials      integer NOT NULL,
    status              text NOT NULL DEFAULT 'PLANNED' CHECK (status IN ('PLANNED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED')),
    scheduled_at        timestamptz NOT NULL,
    started_at          timestamptz,
    completed_at        timestamptz,
    cancelled_at        timestamptz,
    cancel_reason       text CHECK (char_length(cancel_reason) BETWEEN 1 AND 2000),
    notes               text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    created_by          uuid NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tests_timepoint_fkey FOREIGN KEY (experiment_id, timepoint_id) REFERENCES misko.timepoints (experiment_id, id),
    CONSTRAINT tests_protocol_version_fkey FOREIGN KEY (experiment_id, protocol_version_id) REFERENCES misko.protocol_versions (experiment_id, id),
    CONSTRAINT tests_step_fkey FOREIGN KEY (protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials)
        REFERENCES misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trials),
    CONSTRAINT tests_status_times_check CHECK (
        (status = 'PLANNED' AND started_at IS NULL AND completed_at IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL)
        OR (status = 'IN_PROGRESS' AND started_at IS NOT NULL AND completed_at IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL)
        OR (status = 'COMPLETED' AND started_at IS NOT NULL AND completed_at >= started_at AND cancelled_at IS NULL AND cancel_reason IS NULL)
        OR (status = 'CANCELLED' AND completed_at IS NULL AND cancelled_at IS NOT NULL AND cancel_reason IS NOT NULL)),
    CONSTRAINT tests_scope_key UNIQUE (experiment_id, id),
    CONSTRAINT tests_paradigm_scope_key UNIQUE (id, paradigm_key, paradigm_version)
);

CREATE INDEX tests_experiment_idx ON misko.tests (experiment_id, scheduled_at, id);
CREATE INDEX tests_timepoint_idx ON misko.tests (timepoint_id);
CREATE INDEX tests_schedule_idx ON misko.tests (scheduled_at, id) WHERE status IN ('PLANNED', 'IN_PROGRESS');
CREATE TRIGGER tests_running BEFORE INSERT ON misko.tests
    FOR EACH ROW EXECUTE FUNCTION misko.check_experiment_running();

-- Rejects deletes, context changes and status changes other than
-- PLANNED -> IN_PROGRESS | CANCELLED and IN_PROGRESS -> COMPLETED | CANCELLED.
CREATE FUNCTION misko.check_test_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'tests cannot be deleted'
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'tests_immutable';
    END IF;
    IF ROW(NEW.experiment_id, NEW.timepoint_id, NEW.protocol_version_id, NEW.step_position, NEW.paradigm_key, NEW.paradigm_version,
           NEW.planned_trials, NEW.scheduled_at, NEW.notes, NEW.created_by, NEW.created_at)
       IS DISTINCT FROM
       ROW(OLD.experiment_id, OLD.timepoint_id, OLD.protocol_version_id, OLD.step_position, OLD.paradigm_key, OLD.paradigm_version,
           OLD.planned_trials, OLD.scheduled_at, OLD.notes, OLD.created_by, OLD.created_at)
       OR NOT ((OLD.status = 'PLANNED' AND NEW.status IN ('IN_PROGRESS', 'CANCELLED'))
               OR (OLD.status = 'IN_PROGRESS' AND NEW.status IN ('COMPLETED', 'CANCELLED')))
       OR (OLD.started_at IS NOT NULL AND NEW.started_at IS DISTINCT FROM OLD.started_at) THEN
        RAISE EXCEPTION 'test % cannot change from % to %', OLD.id, OLD.status, NEW.status
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'tests_transition';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER tests_change BEFORE UPDATE OR DELETE ON misko.tests
    FOR EACH ROW EXECUTE FUNCTION misko.check_test_change();

-- The animals of a test, one per arena. The arena must run the test's
-- paradigm version and the animal must be allocated in the test's experiment.
CREATE TABLE misko.test_subjects (
    id                      uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id           uuid NOT NULL,
    test_id                 uuid NOT NULL,
    allocation_id           uuid NOT NULL,
    environment_revision_id uuid NOT NULL,
    paradigm_key            text NOT NULL,
    paradigm_version        integer NOT NULL,
    created_by              uuid NOT NULL,
    created_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT test_subjects_test_fkey FOREIGN KEY (experiment_id, test_id) REFERENCES misko.tests (experiment_id, id),
    CONSTRAINT test_subjects_paradigm_fkey FOREIGN KEY (test_id, paradigm_key, paradigm_version) REFERENCES misko.tests (id, paradigm_key, paradigm_version),
    CONSTRAINT test_subjects_allocation_fkey FOREIGN KEY (experiment_id, allocation_id) REFERENCES misko.allocations (experiment_id, id),
    CONSTRAINT test_subjects_arena_fkey FOREIGN KEY (environment_revision_id, paradigm_key, paradigm_version)
        REFERENCES misko.environment_revisions (id, paradigm_key, paradigm_version),
    CONSTRAINT test_subjects_animal_key UNIQUE (test_id, allocation_id),
    CONSTRAINT test_subjects_arena_key UNIQUE (test_id, environment_revision_id),
    CONSTRAINT test_subjects_scope_key UNIQUE (test_id, id)
);

CREATE INDEX test_subjects_allocation_idx ON misko.test_subjects (allocation_id);
CREATE INDEX test_subjects_arena_idx ON misko.test_subjects (environment_revision_id);

-- Animals join or leave a test only while it is planned, and an excluded
-- animal cannot join. Rows are never edited.
CREATE FUNCTION misko.check_test_subject_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    row_test uuid := CASE WHEN TG_OP = 'DELETE' THEN OLD.test_id ELSE NEW.test_id END;
BEGIN
    IF TG_OP = 'UPDATE'
       OR (SELECT t.status FROM misko.tests t WHERE t.id = row_test FOR SHARE) IS DISTINCT FROM 'PLANNED'
       OR (TG_OP = 'INSERT' AND EXISTS (SELECT 1 FROM misko.allocations a WHERE a.id = NEW.allocation_id AND a.excluded_at IS NOT NULL)) THEN
        RAISE EXCEPTION 'animals of test % change only while it is planned, and excluded animals cannot join', row_test
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'test_subjects_frozen';
    END IF;
    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;

CREATE TRIGGER test_subjects_change BEFORE INSERT OR UPDATE OR DELETE ON misko.test_subjects
    FOR EACH ROW EXECUTE FUNCTION misko.check_test_subject_change();

-- Trials of one animal in a test, appended; a repeated repetition gets the
-- next attempt.
CREATE TABLE misko.trials (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    test_id         uuid NOT NULL,
    test_subject_id uuid NOT NULL,
    number          integer NOT NULL CHECK (number >= 1),
    repetition      integer NOT NULL CHECK (repetition >= 1),
    attempt         integer NOT NULL CHECK (attempt >= 1),
    started_at      timestamptz NOT NULL,
    ended_at        timestamptz,
    notes           text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    recorded_by     uuid NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT trials_test_subject_fkey FOREIGN KEY (test_id, test_subject_id) REFERENCES misko.test_subjects (test_id, id),
    CONSTRAINT trials_end_check CHECK (ended_at IS NULL OR ended_at > started_at),
    CONSTRAINT trials_number_key UNIQUE (test_subject_id, number),
    CONSTRAINT trials_attempt_key UNIQUE (test_subject_id, repetition, attempt),
    CONSTRAINT trials_scope_key UNIQUE (test_subject_id, id)
);

CREATE TRIGGER trials_immutable BEFORE UPDATE OR DELETE ON misko.trials
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- A trial needs its test in progress and a planned repetition. The shared row
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
CREATE TRIGGER test_comments_touch BEFORE UPDATE ON misko.test_comments FOR EACH ROW EXECUTE FUNCTION misko.touch();
