-- Per-video calibration. Rows are immutable; a correction supersedes the latest
-- calibration of the same recording, forming one chain per recording.
CREATE TABLE misko.calibrations (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    recording_id       uuid NOT NULL REFERENCES misko.test_recordings (id),
    supersedes_id      uuid,
    camera_id          text NOT NULL CHECK (char_length(camera_id) BETWEEN 1 AND 120),
    frame_width        integer NOT NULL CHECK (frame_width BETWEEN 1 AND 16384),
    frame_height       integer NOT NULL CHECK (frame_height BETWEEN 1 AND 16384),
    crop_x             integer NOT NULL,
    crop_y             integer NOT NULL,
    crop_width         integer NOT NULL,
    crop_height        integer NOT NULL,
    reference_frame_us bigint NOT NULL CHECK (reference_frame_us >= 0),
    measurement_plane  text NOT NULL CHECK (measurement_plane IN ('ARENA_FLOOR', 'WATER_SURFACE', 'APPARATUS_TOP')),
    fit_points         jsonb NOT NULL CHECK (jsonb_typeof(fit_points) = 'array' AND jsonb_array_length(fit_points) BETWEEN 4 AND 100),
    check_points       jsonb NOT NULL CHECK (jsonb_typeof(check_points) = 'array' AND jsonb_array_length(check_points) BETWEEN 3 AND 100),
    transform          double precision[] NOT NULL CHECK (cardinality(transform) = 9),
    fit_rms_error_cm   double precision NOT NULL CHECK (fit_rms_error_cm >= 0),
    check_rms_error_cm double precision NOT NULL CHECK (check_rms_error_cm >= 0),
    check_max_error_cm double precision NOT NULL CHECK (check_max_error_cm >= 0),
    tolerance_cm       double precision NOT NULL CHECK (tolerance_cm > 0),
    algorithm_version  text NOT NULL CHECK (char_length(algorithm_version) BETWEEN 1 AND 80),
    status             text NOT NULL CHECK (status IN ('VALID', 'REJECTED')),
    rejection_reason   text CHECK (rejection_reason IN ('EXCESSIVE_CHECK_ERROR')),
    created_by         uuid NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT calibrations_crop_check CHECK (crop_x >= 0 AND crop_y >= 0 AND crop_width > 0 AND crop_height > 0
        AND crop_x + crop_width <= frame_width AND crop_y + crop_height <= frame_height),
    CONSTRAINT calibrations_result_check CHECK (
        (status = 'VALID' AND rejection_reason IS NULL AND check_max_error_cm <= tolerance_cm + 1e-9)
        OR (status = 'REJECTED' AND rejection_reason IS NOT NULL)),
    CONSTRAINT calibrations_scope_key UNIQUE (recording_id, id),
    CONSTRAINT calibrations_supersedes_fkey FOREIGN KEY (recording_id, supersedes_id) REFERENCES misko.calibrations (recording_id, id),
    CONSTRAINT calibrations_supersedes_key UNIQUE (supersedes_id)
);

-- Only the first calibration of a recording supersedes nothing.
CREATE UNIQUE INDEX calibrations_first_key ON misko.calibrations (recording_id) WHERE supersedes_id IS NULL;

CREATE TRIGGER calibrations_immutable BEFORE UPDATE OR DELETE ON misko.calibrations
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
