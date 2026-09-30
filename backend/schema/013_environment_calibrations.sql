-- Calibrations gain a second scope. A calibration with a recording is a manual
-- override for that video; one without is the default of an environment
-- revision, entered once when the rig is set up. Rows stay immutable and every
-- chain stays linear and inside one scope.
--
-- Like every schema file this installs into a fresh database.

ALTER TABLE misko.analysis_runs DROP CONSTRAINT analysis_runs_calibration_fkey;
ALTER TABLE misko.calibrations DROP CONSTRAINT calibrations_supersedes_fkey;
ALTER TABLE misko.calibrations DROP CONSTRAINT calibrations_scope_key;
DROP INDEX misko.calibrations_first_key;

ALTER TABLE misko.calibrations
    ADD COLUMN environment_revision_id uuid NOT NULL REFERENCES misko.environment_revisions (id),
    ALTER COLUMN recording_id DROP NOT NULL,
    -- An environment calibration has no video, so no reference frame time.
    ALTER COLUMN reference_frame_us DROP NOT NULL,
    -- The chain a row belongs to: its recording, or its environment revision.
    -- Both are UUIDv7, so the two kinds never collide.
    ADD COLUMN chain_id uuid GENERATED ALWAYS AS (coalesce(recording_id, environment_revision_id)) STORED,
    ADD CONSTRAINT calibrations_reference_frame_check CHECK (recording_id IS NULL OR reference_frame_us IS NOT NULL);

ALTER TABLE misko.calibrations
    ADD CONSTRAINT calibrations_chain_key UNIQUE (chain_id, id),
    ADD CONSTRAINT calibrations_supersedes_fkey FOREIGN KEY (chain_id, supersedes_id) REFERENCES misko.calibrations (chain_id, id);

-- Only the first calibration of a chain supersedes nothing.
CREATE UNIQUE INDEX calibrations_first_key ON misko.calibrations (chain_id) WHERE supersedes_id IS NULL;
CREATE INDEX calibrations_revision_idx ON misko.calibrations (environment_revision_id) WHERE recording_id IS NULL;

-- A recording calibration belongs to the environment revision of its test, so
-- its bounds and tolerance were checked against the same measurements.
CREATE FUNCTION misko.check_calibration_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.recording_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM misko.test_recordings r JOIN misko.tests t ON t.id = r.test_id
        WHERE r.id = NEW.recording_id AND t.environment_revision_id = NEW.environment_revision_id) THEN
        RAISE EXCEPTION 'calibration of recording % must use the environment revision of its test', NEW.recording_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'calibrations_scope';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER calibrations_scope BEFORE INSERT ON misko.calibrations
    FOR EACH ROW EXECUTE FUNCTION misko.check_calibration_scope();

-- A run pins a VALID calibration of its own recording, or the default of its own
-- environment revision.
ALTER TABLE misko.analysis_runs
    ADD CONSTRAINT analysis_runs_calibration_fkey FOREIGN KEY (calibration_id) REFERENCES misko.calibrations (id);

CREATE FUNCTION misko.check_analysis_run_calibration() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.calibration_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM misko.calibrations c
        WHERE c.id = NEW.calibration_id AND c.status = 'VALID' AND c.environment_revision_id = NEW.environment_revision_id
          AND (c.recording_id IS NULL OR c.recording_id = NEW.recording_id)) THEN
        RAISE EXCEPTION 'analysis run of recording % cannot pin calibration %', NEW.recording_id, NEW.calibration_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_runs_calibration_scope';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER analysis_runs_calibration_scope BEFORE INSERT ON misko.analysis_runs
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_run_calibration();

-- The calibration each recording is analyzed with: its own latest calibration
-- when it has one (a REJECTED one leaves the recording waiting), otherwise the
-- latest calibration of its environment revision. calibration_id is NULL while
-- the recording waits; source is NULL until something was calibrated. Analysis
-- and the dashboard read this view, mirroring calibration/domain.Effective.
CREATE VIEW misko.effective_calibrations AS
SELECT r.id AS recording_id,
       CASE WHEN own.id IS NOT NULL THEN 'RECORDING' WHEN env.id IS NOT NULL THEN 'ENVIRONMENT' END AS source,
       CASE WHEN own.id IS NOT NULL THEN CASE WHEN own.status = 'VALID' THEN own.id END
            WHEN env.status = 'VALID' THEN env.id END AS calibration_id
FROM misko.test_recordings r
JOIN misko.tests t ON t.id = r.test_id
LEFT JOIN LATERAL (
    SELECT c.id, c.status FROM misko.calibrations c
    WHERE c.recording_id = r.id AND NOT EXISTS (SELECT 1 FROM misko.calibrations n WHERE n.supersedes_id = c.id)
) own ON true
LEFT JOIN LATERAL (
    SELECT c.id, c.status FROM misko.calibrations c
    WHERE c.environment_revision_id = t.environment_revision_id AND c.recording_id IS NULL
      AND NOT EXISTS (SELECT 1 FROM misko.calibrations n WHERE n.supersedes_id = c.id)
) env ON true;
