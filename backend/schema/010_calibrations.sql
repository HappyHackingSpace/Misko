-- Pixel-to-centimetre calibrations. A calibration without a test subject is
-- the default of an environment revision, entered once when the rig is set up;
-- its crop is that arena's region of the fixed camera frame, so several arenas
-- filmed together each have their own. A calibration with a test subject is a
-- manual override for that animal's arena in one video, used when the camera
-- or the arena moved. Rows are immutable; a correction supersedes the latest
-- calibration of the same chain (the environment revision, or the test
-- subject).
CREATE TABLE misko.calibrations (
    id                      uuid PRIMARY KEY DEFAULT uuidv7(),
    environment_revision_id uuid NOT NULL REFERENCES misko.environment_revisions (id),
    test_subject_id         uuid REFERENCES misko.test_subjects (id),
    recording_id            uuid REFERENCES misko.test_recordings (id),
    chain_id                uuid NOT NULL GENERATED ALWAYS AS (coalesce(test_subject_id, environment_revision_id)) STORED,
    supersedes_id           uuid,
    camera_id               text NOT NULL CHECK (char_length(camera_id) BETWEEN 1 AND 120),
    frame_width             integer NOT NULL CHECK (frame_width BETWEEN 1 AND 16384),
    frame_height            integer NOT NULL CHECK (frame_height BETWEEN 1 AND 16384),
    crop_x                  integer NOT NULL,
    crop_y                  integer NOT NULL,
    crop_width              integer NOT NULL,
    crop_height             integer NOT NULL,
    reference_frame_us      bigint CHECK (reference_frame_us >= 0),
    measurement_plane       text NOT NULL CHECK (measurement_plane IN ('ARENA_FLOOR', 'WATER_SURFACE', 'APPARATUS_TOP')),
    fit_points              jsonb NOT NULL CHECK (jsonb_typeof(fit_points) = 'array' AND jsonb_array_length(fit_points) BETWEEN 4 AND 100),
    check_points            jsonb NOT NULL CHECK (jsonb_typeof(check_points) = 'array' AND jsonb_array_length(check_points) BETWEEN 3 AND 100),
    transform               double precision[] NOT NULL CHECK (cardinality(transform) = 9),
    fit_rms_error_cm        double precision NOT NULL CHECK (fit_rms_error_cm >= 0),
    check_rms_error_cm      double precision NOT NULL CHECK (check_rms_error_cm >= 0),
    check_max_error_cm      double precision NOT NULL CHECK (check_max_error_cm >= 0),
    tolerance_cm            double precision NOT NULL CHECK (tolerance_cm > 0),
    algorithm_version       text NOT NULL CHECK (char_length(algorithm_version) BETWEEN 1 AND 80),
    status                  text NOT NULL CHECK (status IN ('VALID', 'REJECTED')),
    rejection_reason        text CHECK (rejection_reason IN ('EXCESSIVE_CHECK_ERROR')),
    created_by              uuid NOT NULL,
    created_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT calibrations_override_check CHECK ((test_subject_id IS NULL) = (recording_id IS NULL)
        AND (recording_id IS NULL) = (reference_frame_us IS NULL)),
    CONSTRAINT calibrations_crop_check CHECK (crop_x >= 0 AND crop_y >= 0 AND crop_width > 0 AND crop_height > 0
        AND crop_x + crop_width <= frame_width AND crop_y + crop_height <= frame_height),
    CONSTRAINT calibrations_result_check CHECK (
        (status = 'VALID' AND rejection_reason IS NULL AND check_max_error_cm <= tolerance_cm + 1e-9)
        OR (status = 'REJECTED' AND rejection_reason IS NOT NULL)),
    CONSTRAINT calibrations_chain_key UNIQUE (chain_id, id),
    CONSTRAINT calibrations_supersedes_fkey FOREIGN KEY (chain_id, supersedes_id) REFERENCES misko.calibrations (chain_id, id),
    CONSTRAINT calibrations_supersedes_key UNIQUE (supersedes_id)
);

-- Only the first calibration of a chain supersedes nothing.
CREATE UNIQUE INDEX calibrations_first_key ON misko.calibrations (chain_id) WHERE supersedes_id IS NULL;
CREATE INDEX calibrations_revision_idx ON misko.calibrations (environment_revision_id) WHERE test_subject_id IS NULL;
CREATE INDEX calibrations_test_subject_idx ON misko.calibrations (test_subject_id) WHERE test_subject_id IS NOT NULL;

CREATE TRIGGER calibrations_immutable BEFORE UPDATE OR DELETE ON misko.calibrations
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- An override uses the arena of its test subject and a video of the same test,
-- so its bounds and tolerance were checked against the same measurements.
CREATE FUNCTION misko.check_calibration_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.test_subject_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM misko.test_subjects s JOIN misko.test_recordings r ON r.test_id = s.test_id
        WHERE s.id = NEW.test_subject_id AND r.id = NEW.recording_id AND s.environment_revision_id = NEW.environment_revision_id) THEN
        RAISE EXCEPTION 'calibration of test subject % must use its arena and a video of its test', NEW.test_subject_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'calibrations_scope';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER calibrations_scope BEFORE INSERT ON misko.calibrations
    FOR EACH ROW EXECUTE FUNCTION misko.check_calibration_scope();

-- The calibration each animal of a test is analysed with: its own latest
-- override for the test's current video when there is one (a REJECTED one
-- leaves the animal waiting), otherwise the latest calibration of its arena.
-- calibration_id is NULL while the animal waits; source is NULL until anything
-- was calibrated.
CREATE VIEW misko.effective_calibrations AS
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
    WHERE c.environment_revision_id = s.environment_revision_id AND c.test_subject_id IS NULL
      AND NOT EXISTS (SELECT 1 FROM misko.calibrations n WHERE n.supersedes_id = c.id)
) arena ON true;

CREATE TRIGGER effective_calibrations_read_only INSTEAD OF INSERT OR UPDATE OR DELETE ON misko.effective_calibrations
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
