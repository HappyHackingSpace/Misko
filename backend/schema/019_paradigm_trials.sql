-- Paradigm trials: a paradigm version tried on one video in one arena (an
-- environment revision), outside any experiment. A trial has no groups, no
-- review and no report; its analysis jobs never reach an experiment.
--
-- A calibration is now owned by an environment revision (the default), a test
-- subject (an override on one video of its test) or a paradigm trial (on the
-- trial's own video, never changing the environment).

-- video_asset_id is the trial's current video; a replacement upload takes its
-- place and earlier jobs keep the video they analysed. A deleted trial is
-- kept, hidden and frozen, because its jobs never change.
CREATE TABLE misko.paradigm_trials (
    id                      uuid PRIMARY KEY DEFAULT uuidv7(),
    name                    text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    paradigm_key            text NOT NULL,
    paradigm_version        integer NOT NULL,
    environment_revision_id uuid NOT NULL,
    video_asset_id          uuid REFERENCES misko.video_assets (id),
    clip_start_us           bigint NOT NULL DEFAULT 0 CHECK (clip_start_us >= 0),
    clip_end_us             bigint,
    notes                   text CHECK (char_length(notes) BETWEEN 1 AND 2000),
    created_by              uuid NOT NULL,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    deleted_at              timestamptz,
    CONSTRAINT paradigm_trials_arena_fkey FOREIGN KEY (environment_revision_id, paradigm_key, paradigm_version)
        REFERENCES misko.environment_revisions (id, paradigm_key, paradigm_version),
    CONSTRAINT paradigm_trials_clip_check CHECK (clip_end_us IS NULL OR clip_end_us > clip_start_us),
    CONSTRAINT paradigm_trials_video_key UNIQUE (video_asset_id)
);

CREATE INDEX paradigm_trials_list_idx ON misko.paradigm_trials (created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX paradigm_trials_arena_idx ON misko.paradigm_trials (environment_revision_id);

-- A trial keeps its paradigm, arena and author; only the video, its clip, the
-- name and the notes change, and nothing changes once it is deleted. Rows are
-- never removed.
CREATE FUNCTION misko.check_paradigm_trial_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE'
       OR OLD.deleted_at IS NOT NULL
       OR ROW(NEW.paradigm_key, NEW.paradigm_version, NEW.environment_revision_id, NEW.created_by, NEW.created_at)
          IS DISTINCT FROM ROW(OLD.paradigm_key, OLD.paradigm_version, OLD.environment_revision_id, OLD.created_by, OLD.created_at) THEN
        RAISE EXCEPTION 'paradigm trial % cannot change this way', OLD.id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'paradigm_trials_frozen';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER paradigm_trials_change BEFORE UPDATE OR DELETE ON misko.paradigm_trials
    FOR EACH ROW EXECUTE FUNCTION misko.check_paradigm_trial_change();

-- Which job analysed which trial, from which of its videos, with which
-- effective calibration and which ANALYSIS overrides.
CREATE TABLE misko.paradigm_trial_jobs (
    job_id         uuid PRIMARY KEY REFERENCES misko.analysis_jobs (id),
    trial_id       uuid NOT NULL REFERENCES misko.paradigm_trials (id),
    video_asset_id uuid NOT NULL REFERENCES misko.video_assets (id),
    calibration_id uuid,
    trigger        text NOT NULL CHECK (trigger IN ('AUTOMATIC', 'MANUAL')),
    overrides      jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(overrides) = 'object'),
    created_by     uuid,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- One automatic job per trial video.
CREATE UNIQUE INDEX paradigm_trial_jobs_automatic_key ON misko.paradigm_trial_jobs (trial_id, video_asset_id) WHERE trigger = 'AUTOMATIC';
CREATE INDEX paradigm_trial_jobs_trial_idx ON misko.paradigm_trial_jobs (trial_id, created_at);

CREATE FUNCTION misko.check_paradigm_trial_job() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM misko.analysis_jobs j
                   WHERE j.id = NEW.job_id AND j.requester_kind = 'TRIAL' AND j.requester_id = NEW.trial_id) THEN
        RAISE EXCEPTION 'job % was not requested by paradigm trial %', NEW.job_id, NEW.trial_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'paradigm_trial_jobs_requester';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER paradigm_trial_jobs_requester BEFORE INSERT ON misko.paradigm_trial_jobs
    FOR EACH ROW EXECUTE FUNCTION misko.check_paradigm_trial_job();
CREATE TRIGGER paradigm_trial_jobs_immutable BEFORE UPDATE OR DELETE ON misko.paradigm_trial_jobs
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- Calibration owners. A trial calibration names the trial video it was read
-- from, as an override names its test recording.
ALTER TABLE misko.calibrations
    ADD COLUMN trial_id uuid CONSTRAINT calibrations_trial_fkey REFERENCES misko.paradigm_trials (id),
    ADD COLUMN video_asset_id uuid CONSTRAINT calibrations_video_asset_fkey REFERENCES misko.video_assets (id);
ALTER TABLE misko.calibrations DROP CONSTRAINT calibrations_override_check;
ALTER TABLE misko.calibrations ADD CONSTRAINT calibrations_owner_check CHECK (
    (test_subject_id IS NULL AND trial_id IS NULL AND recording_id IS NULL AND video_asset_id IS NULL AND reference_frame_us IS NULL)
    OR (test_subject_id IS NOT NULL AND trial_id IS NULL AND recording_id IS NOT NULL AND video_asset_id IS NULL AND reference_frame_us IS NOT NULL)
    OR (trial_id IS NOT NULL AND test_subject_id IS NULL AND recording_id IS NULL AND video_asset_id IS NOT NULL AND reference_frame_us IS NOT NULL));

-- The chain of a calibration is its owner. Dropping the generated column
-- drops the keys built on it; they are created again on the new one.
ALTER TABLE misko.calibrations DROP COLUMN chain_id CASCADE;
ALTER TABLE misko.calibrations
    ADD COLUMN chain_id uuid NOT NULL GENERATED ALWAYS AS (coalesce(test_subject_id, trial_id, environment_revision_id)) STORED;
ALTER TABLE misko.calibrations ADD CONSTRAINT calibrations_chain_key UNIQUE (chain_id, id);
ALTER TABLE misko.calibrations ADD CONSTRAINT calibrations_supersedes_fkey
    FOREIGN KEY (chain_id, supersedes_id) REFERENCES misko.calibrations (chain_id, id);
CREATE UNIQUE INDEX calibrations_first_key ON misko.calibrations (chain_id) WHERE supersedes_id IS NULL;

DROP INDEX misko.calibrations_revision_idx;
CREATE INDEX calibrations_revision_idx ON misko.calibrations (environment_revision_id) WHERE test_subject_id IS NULL AND trial_id IS NULL;
CREATE INDEX calibrations_trial_idx ON misko.calibrations (trial_id) WHERE trial_id IS NOT NULL;

-- An override uses the arena of its test subject and a video of the same
-- test; a trial calibration uses the trial's arena and its current video.
CREATE OR REPLACE FUNCTION misko.check_calibration_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.test_subject_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM misko.test_subjects s JOIN misko.test_recordings r ON r.test_id = s.test_id
        WHERE s.id = NEW.test_subject_id AND r.id = NEW.recording_id AND s.environment_revision_id = NEW.environment_revision_id) THEN
        RAISE EXCEPTION 'calibration of test subject % must use its arena and a video of its test', NEW.test_subject_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'calibrations_scope';
    END IF;
    IF NEW.trial_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM misko.paradigm_trials t
        WHERE t.id = NEW.trial_id AND t.deleted_at IS NULL AND t.video_asset_id = NEW.video_asset_id
          AND t.environment_revision_id = NEW.environment_revision_id) THEN
        RAISE EXCEPTION 'calibration of paradigm trial % must use its arena and its current video', NEW.trial_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'calibrations_scope';
    END IF;
    RETURN NEW;
END;
$$;

-- As in 010_calibrations.sql; an arena's default ignores trial calibrations.
CREATE OR REPLACE VIEW misko.effective_calibrations AS
SELECT s.id AS test_subject_id, r.id AS recording_id,
       CASE WHEN own.id IS NOT NULL THEN 'OVERRIDE' WHEN arena.id IS NOT NULL THEN 'ENVIRONMENT' END AS source,
       CASE WHEN own.id IS NOT NULL THEN CASE WHEN own.status = 'VALID' THEN own.id END
            WHEN arena.status = 'VALID' THEN arena.id END AS calibration_id
FROM misko.test_subjects s
JOIN misko.current_recordings r ON r.test_id = s.test_id
LEFT JOIN LATERAL (
    SELECT c.id, c.status FROM misko.calibrations c
    WHERE c.test_subject_id = s.id AND c.recording_id = r.id
      AND NOT EXISTS (SELECT 1 FROM misko.calibrations n WHERE n.supersedes_id = c.id)
) own ON true
LEFT JOIN LATERAL (
    SELECT c.id, c.status FROM misko.calibrations c
    WHERE c.environment_revision_id = s.environment_revision_id AND c.test_subject_id IS NULL AND c.trial_id IS NULL
      AND NOT EXISTS (SELECT 1 FROM misko.calibrations n WHERE n.supersedes_id = c.id)
) arena ON true;

-- The calibration each trial is analysed with: its own latest calibration on
-- its current video when there is one (a REJECTED one leaves it waiting),
-- otherwise the latest default of its arena. Columns as in
-- effective_calibrations, with source TRIAL for the trial's own calibration.
CREATE VIEW misko.trial_effective_calibrations AS
SELECT t.id AS trial_id, t.video_asset_id,
       CASE WHEN own.id IS NOT NULL THEN 'TRIAL' WHEN arena.id IS NOT NULL THEN 'ENVIRONMENT' END AS source,
       CASE WHEN own.id IS NOT NULL THEN CASE WHEN own.status = 'VALID' THEN own.id END
            WHEN arena.status = 'VALID' THEN arena.id END AS calibration_id
FROM misko.paradigm_trials t
LEFT JOIN LATERAL (
    SELECT c.id, c.status FROM misko.calibrations c
    WHERE c.trial_id = t.id AND c.video_asset_id = t.video_asset_id
      AND NOT EXISTS (SELECT 1 FROM misko.calibrations n WHERE n.supersedes_id = c.id)
) own ON true
LEFT JOIN LATERAL (
    SELECT c.id, c.status FROM misko.calibrations c
    WHERE c.environment_revision_id = t.environment_revision_id AND c.test_subject_id IS NULL AND c.trial_id IS NULL
      AND NOT EXISTS (SELECT 1 FROM misko.calibrations n WHERE n.supersedes_id = c.id)
) arena ON true
WHERE t.deleted_at IS NULL;

CREATE TRIGGER trial_effective_calibrations_read_only INSTEAD OF INSERT OR UPDATE OR DELETE ON misko.trial_effective_calibrations
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
