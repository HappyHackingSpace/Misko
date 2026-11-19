-- Calibration proposals: a worker detects the apparatus on one reference frame
-- the panel uploads and proposes fit points for the calibration form. A
-- proposal is a suggestion, never a calibration: the researcher confirms every
-- point and saves through the normal calibration routes. Proposals are kept
-- for a day, then deleted.
--
-- The frame is a JPEG or PNG of at most 700 KiB, stored inline: it is small,
-- short-lived and read once by the worker that claims it. result is the
-- worker's JSON answer, {"detected": false, "reason": ...} when nothing was
-- found.
CREATE TABLE misko.calibration_proposals (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    paradigm_key     text NOT NULL CHECK (paradigm_key ~ '^[A-Z][A-Z_]{0,39}$'),
    paradigm_version integer NOT NULL CHECK (paradigm_version >= 1),
    parameters       jsonb NOT NULL CHECK (jsonb_typeof(parameters) = 'object'),
    frame            bytea NOT NULL CHECK (octet_length(frame) BETWEEN 1 AND 716800),
    frame_type       text NOT NULL CHECK (frame_type IN ('image/jpeg', 'image/png')),
    status           text NOT NULL CHECK (status IN ('QUEUED', 'RUNNING', 'SUCCEEDED', 'FAILED')),
    attempt          integer NOT NULL DEFAULT 0 CHECK (attempt BETWEEN 0 AND 3),
    worker_id        uuid REFERENCES misko.analysis_workers (id),
    lease_expires_at timestamptz,
    result           jsonb CHECK (jsonb_typeof(result) = 'object'),
    failure_reason   text CHECK (char_length(failure_reason) BETWEEN 1 AND 2000),
    created_by       uuid NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    finished_at      timestamptz,
    CONSTRAINT calibration_proposals_state_check CHECK (
        (status = 'QUEUED' AND worker_id IS NULL AND lease_expires_at IS NULL AND result IS NULL AND finished_at IS NULL)
        OR (status = 'RUNNING' AND worker_id IS NOT NULL AND lease_expires_at IS NOT NULL AND attempt >= 1 AND result IS NULL AND finished_at IS NULL)
        OR (status = 'SUCCEEDED' AND worker_id IS NOT NULL AND attempt >= 1 AND lease_expires_at IS NULL AND result IS NOT NULL AND finished_at IS NOT NULL)
        OR (status = 'FAILED' AND lease_expires_at IS NULL AND result IS NULL AND failure_reason IS NOT NULL AND finished_at IS NOT NULL))
);

CREATE INDEX calibration_proposals_queue_idx ON misko.calibration_proposals (created_at, id) WHERE status IN ('QUEUED', 'RUNNING');
CREATE INDEX calibration_proposals_age_idx ON misko.calibration_proposals (created_at);
