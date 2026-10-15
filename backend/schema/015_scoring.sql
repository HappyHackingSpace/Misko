-- Observer scoring. For paradigms a person scores on the video (rotarod,
-- pole, treadmill, novel object), the observations of one animal of a test
-- are kept on a scoring sheet. Completing the sheet computes its metrics with
-- the engine and stores them as a SCORED analysis run, which feeds
-- subject_results and the reports exactly like a run a worker analysed.

-- Analysis runs are AUTOMATED (a worker tracked the video) or SCORED (a
-- person scored it). A scored run has no worker, lease, model version,
-- calibration or video pair; it records who scored it, is created RUNNING and
-- becomes SUCCEEDED in the same transaction.
ALTER TABLE misko.analysis_runs
    ADD COLUMN method text NOT NULL DEFAULT 'AUTOMATED' CONSTRAINT analysis_runs_method_check CHECK (method IN ('AUTOMATED', 'SCORED')),
    ADD COLUMN scored_by uuid;

ALTER TABLE misko.analysis_runs DROP CONSTRAINT analysis_runs_state_check;
ALTER TABLE misko.analysis_runs ADD CONSTRAINT analysis_runs_state_check CHECK (
    (method = 'AUTOMATED' AND scored_by IS NULL AND (
        (status = 'QUEUED' AND worker_id IS NULL AND lease_expires_at IS NULL AND finished_at IS NULL)
        OR (status = 'RUNNING' AND worker_id IS NOT NULL AND lease_expires_at IS NOT NULL AND attempt >= 1 AND finished_at IS NULL)
        OR (status = 'SUCCEEDED' AND worker_id IS NOT NULL AND model_version IS NOT NULL AND attempt >= 1 AND lease_expires_at IS NULL AND finished_at IS NOT NULL)
        OR (status = 'FAILED' AND lease_expires_at IS NULL AND failure_reason IS NOT NULL AND finished_at IS NOT NULL)))
    OR (method = 'SCORED' AND scored_by IS NOT NULL AND trigger = 'MANUAL' AND worker_id IS NULL AND lease_expires_at IS NULL
        AND model_version IS NULL AND calibration_id IS NULL AND failure_reason IS NULL AND attempt = 1 AND max_attempts = 1
        AND ((status = 'RUNNING' AND finished_at IS NULL) OR (status = 'SUCCEEDED' AND finished_at IS NOT NULL))));

-- The method and the scorer of a run never change.
CREATE FUNCTION misko.check_analysis_run_method() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.method IS DISTINCT FROM OLD.method OR NEW.scored_by IS DISTINCT FROM OLD.scored_by THEN
        RAISE EXCEPTION 'analysis run % cannot change its method or scorer', OLD.id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_runs_transition';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER analysis_runs_method_fixed BEFORE UPDATE ON misko.analysis_runs
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_run_method();

-- A scoring sheet: the observations of one animal of a test, scored on the
-- test's current video. A sheet is a DRAFT while it is being scored and
-- COMPLETED once its scored event types and recording length are fixed and
-- its SCORED run is published; the video it was scored on is pinned at
-- completion. Reopening a completed sheet starts the next version; only the
-- newest version may be a draft, and completed sheets never change.
CREATE TABLE misko.scoring_sheets (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id   uuid NOT NULL,
    test_id         uuid NOT NULL,
    test_subject_id uuid NOT NULL,
    version         integer NOT NULL CHECK (version >= 1),
    status          text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'COMPLETED')),
    scored_types    text[],
    duration_us     bigint CHECK (duration_us > 0),
    recording_id    uuid,
    run_id          uuid,
    created_by      uuid NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    completed_by    uuid,
    completed_at    timestamptz,
    CONSTRAINT scoring_sheets_test_fkey FOREIGN KEY (experiment_id, test_id) REFERENCES misko.tests (experiment_id, id),
    CONSTRAINT scoring_sheets_test_subject_fkey FOREIGN KEY (test_id, test_subject_id) REFERENCES misko.test_subjects (test_id, id),
    CONSTRAINT scoring_sheets_recording_fkey FOREIGN KEY (test_id, recording_id) REFERENCES misko.test_recordings (test_id, id),
    CONSTRAINT scoring_sheets_run_fkey FOREIGN KEY (test_subject_id, run_id) REFERENCES misko.analysis_runs (test_subject_id, id),
    CONSTRAINT scoring_sheets_version_key UNIQUE (test_subject_id, version),
    CONSTRAINT scoring_sheets_run_key UNIQUE (run_id),
    CONSTRAINT scoring_sheets_scope_key UNIQUE (test_subject_id, id),
    CONSTRAINT scoring_sheets_types_check CHECK (scored_types IS NULL
        OR (cardinality(scored_types) <= 20 AND array_position(scored_types, NULL) IS NULL)),
    CONSTRAINT scoring_sheets_state_check CHECK (
        (status = 'DRAFT' AND scored_types IS NULL AND duration_us IS NULL AND recording_id IS NULL AND run_id IS NULL
            AND completed_by IS NULL AND completed_at IS NULL)
        OR (status = 'COMPLETED' AND scored_types IS NOT NULL AND recording_id IS NOT NULL AND run_id IS NOT NULL
            AND completed_by IS NOT NULL AND completed_at IS NOT NULL))
);

CREATE UNIQUE INDEX scoring_sheets_draft_key ON misko.scoring_sheets (test_subject_id) WHERE status = 'DRAFT';

-- A sheet is started on a test in progress or completed, as the next version
-- after a completed one. It changes only from DRAFT to COMPLETED, with a
-- SCORED run of its own animal in the video it pins, and is never deleted.
CREATE FUNCTION misko.check_scoring_sheet_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'scoring sheets cannot be deleted'
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'scoring_sheets_transition';
    END IF;
    IF TG_OP = 'INSERT' THEN
        IF (SELECT t.status FROM misko.tests t WHERE t.id = NEW.test_id FOR SHARE) NOT IN ('IN_PROGRESS', 'COMPLETED') THEN
            RAISE EXCEPTION 'scoring needs test % in progress or completed', NEW.test_id
                USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'scoring_sheets_test_started';
        END IF;
        IF NEW.status <> 'DRAFT' OR NEW.version <> coalesce(
            (SELECT max(s.version) FROM misko.scoring_sheets s WHERE s.test_subject_id = NEW.test_subject_id), 0) + 1 THEN
            RAISE EXCEPTION 'a scoring sheet of test subject % starts as the next draft version', NEW.test_subject_id
                USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'scoring_sheets_transition';
        END IF;
        RETURN NEW;
    END IF;
    IF OLD.status <> 'DRAFT' OR NEW.status <> 'COMPLETED'
       OR ROW(NEW.id, NEW.experiment_id, NEW.test_id, NEW.test_subject_id, NEW.version, NEW.created_by, NEW.created_at)
          IS DISTINCT FROM ROW(OLD.id, OLD.experiment_id, OLD.test_id, OLD.test_subject_id, OLD.version, OLD.created_by, OLD.created_at) THEN
        RAISE EXCEPTION 'scoring sheet % cannot change from % to %', OLD.id, OLD.status, NEW.status
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'scoring_sheets_transition';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM misko.analysis_runs r
                   WHERE r.id = NEW.run_id AND r.test_subject_id = NEW.test_subject_id AND r.recording_id = NEW.recording_id
                     AND r.method = 'SCORED') THEN
        RAISE EXCEPTION 'scoring sheet % needs a scored run of its animal in its video', OLD.id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'scoring_sheets_run_scope';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER scoring_sheets_change BEFORE INSERT OR UPDATE OR DELETE ON misko.scoring_sheets
    FOR EACH ROW EXECUTE FUNCTION misko.check_scoring_sheet_change();

-- One observation on a sheet, in microseconds from the clip start of the
-- test's current video (the trial time the engine consumes). Point events have end_us equal to start_us; intervals cover
-- [start_us, end_us). The label names the object or side when the paradigm's
-- event has labels (novel, familiar).
CREATE TABLE misko.scored_observations (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    sheet_id        uuid NOT NULL,
    test_subject_id uuid NOT NULL,
    event_type      text NOT NULL CHECK (event_type ~ '^[a-z][a-z0-9_]{0,79}$'),
    kind            text NOT NULL CHECK (kind IN ('INTERVAL', 'POINT')),
    label           text CHECK (label ~ '^[a-z][a-z0-9_]{0,39}$'),
    start_us        bigint NOT NULL CHECK (start_us >= 0),
    end_us          bigint NOT NULL,
    scored_by       uuid NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT scored_observations_sheet_fkey FOREIGN KEY (test_subject_id, sheet_id) REFERENCES misko.scoring_sheets (test_subject_id, id),
    CONSTRAINT scored_observations_time_check CHECK ((kind = 'POINT' AND end_us = start_us) OR (kind = 'INTERVAL' AND end_us > start_us))
);

CREATE INDEX scored_observations_sheet_idx ON misko.scored_observations (sheet_id, start_us);

-- Observations are added, changed and removed only while their sheet is a
-- draft; the shared row lock waits for a concurrent completion. An
-- observation never moves to another sheet.
CREATE FUNCTION misko.check_scored_observation_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    row_sheet uuid := CASE WHEN TG_OP = 'DELETE' THEN OLD.sheet_id ELSE NEW.sheet_id END;
BEGIN
    IF (SELECT s.status FROM misko.scoring_sheets s WHERE s.id = row_sheet FOR SHARE) IS DISTINCT FROM 'DRAFT'
       OR (TG_OP = 'UPDATE' AND ROW(NEW.id, NEW.sheet_id, NEW.test_subject_id, NEW.created_at)
                                IS DISTINCT FROM ROW(OLD.id, OLD.sheet_id, OLD.test_subject_id, OLD.created_at)) THEN
        RAISE EXCEPTION 'observations of scoring sheet % change only while it is a draft', row_sheet
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'scored_observations_frozen';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER scored_observations_change BEFORE INSERT OR UPDATE OR DELETE ON misko.scored_observations
    FOR EACH ROW EXECUTE FUNCTION misko.check_scored_observation_change();

-- A run becomes SUCCEEDED only with its outputs in the same transaction: an
-- automated run with its video pair and analysed video, a scored run with the
-- completed sheet it was computed from.
CREATE OR REPLACE FUNCTION misko.check_analysis_published() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status = 'SUCCEEDED' AND NEW.method = 'SCORED' THEN
        IF NOT EXISTS (SELECT 1 FROM misko.scoring_sheets s WHERE s.run_id = NEW.id AND s.status = 'COMPLETED') THEN
            RAISE EXCEPTION 'scored run % succeeded without its completed sheet', NEW.id
                USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_runs_published';
        END IF;
    ELSIF NEW.status = 'SUCCEEDED' AND NOT EXISTS (
        SELECT 1 FROM misko.analysis_video_pairs p
        JOIN misko.analysis_artifacts a ON a.run_id = p.run_id AND a.video_asset_id = p.analyzed_asset_id
        WHERE p.run_id = NEW.id AND p.attempt = NEW.attempt) THEN
        RAISE EXCEPTION 'run % succeeded without its video pair', NEW.id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_runs_published';
    END IF;
    RETURN NULL;
END;
$$;

-- A scored run never stays running: it succeeds in the transaction that
-- created it.
CREATE FUNCTION misko.check_scored_run_finished() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (SELECT r.status FROM misko.analysis_runs r WHERE r.id = NEW.id) IS DISTINCT FROM 'SUCCEEDED' THEN
        RAISE EXCEPTION 'scored run % must succeed in the transaction that creates it', NEW.id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_runs_published';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER analysis_runs_scored_finished AFTER INSERT ON misko.analysis_runs
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.method = 'SCORED') EXECUTE FUNCTION misko.check_scored_run_finished();
