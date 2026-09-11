-- Scope keys so analysis rows can reference a recording's own video and a
-- test's own trials.
ALTER TABLE misko.test_recordings ADD CONSTRAINT test_recordings_asset_scope_key UNIQUE (id, video_asset_id);
ALTER TABLE misko.trials ADD CONSTRAINT trials_scope_key UNIQUE (test_id, id);

-- Analysis workers are service identities, separate from users and roles.
-- Only a SHA-256 hash of the token is stored.
CREATE TABLE misko.analysis_workers (
    id            uuid PRIMARY KEY DEFAULT uuidv7(),
    name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    model_version text NOT NULL CHECK (char_length(model_version) BETWEEN 1 AND 120),
    token_sha256  bytea NOT NULL CHECK (octet_length(token_sha256) = 32),
    disabled_at   timestamptz,
    created_by    uuid NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT analysis_workers_token_key UNIQUE (token_sha256)
);

CREATE UNIQUE INDEX analysis_workers_name_key ON misko.analysis_workers (lower(name));

CREATE TABLE misko.worker_capabilities (
    worker_id        uuid NOT NULL REFERENCES misko.analysis_workers (id),
    paradigm_key     text NOT NULL CHECK (paradigm_key ~ '^[A-Z][A-Z_]{0,39}$'),
    paradigm_version integer NOT NULL CHECK (paradigm_version >= 1),
    CONSTRAINT worker_capabilities_pkey PRIMARY KEY (worker_id, paradigm_key, paradigm_version)
);

-- A run is the job and its record. Inputs are pinned when the run is created;
-- attempt is the fencing token of the current lease.
CREATE TABLE misko.analysis_runs (
    id                      uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id           uuid NOT NULL,
    test_id                 uuid NOT NULL,
    recording_id            uuid NOT NULL,
    source_asset_id         uuid NOT NULL,
    source_generation       bigint NOT NULL CHECK (source_generation > 0),
    source_crc32c           bigint NOT NULL CHECK (source_crc32c BETWEEN 0 AND 4294967295),
    clip_start_us           bigint NOT NULL CHECK (clip_start_us >= 0),
    clip_end_us             bigint,
    calibration_id          uuid,
    paradigm_key            text NOT NULL,
    paradigm_version        integer NOT NULL,
    metric_engine_version   integer NOT NULL,
    result_schema_version   integer NOT NULL,
    environment_revision_id uuid NOT NULL REFERENCES misko.environment_revisions (id),
    protocol_version_id     uuid NOT NULL,
    parameters              jsonb NOT NULL CHECK (jsonb_typeof(parameters) = 'object'),
    trigger                 text NOT NULL CHECK (trigger IN ('AUTOMATIC', 'MANUAL')),
    status                  text NOT NULL CHECK (status IN ('QUEUED', 'RUNNING', 'SUCCEEDED', 'FAILED')),
    attempt                 integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts            integer NOT NULL CHECK (max_attempts BETWEEN 1 AND 20),
    worker_id               uuid REFERENCES misko.analysis_workers (id),
    lease_expires_at        timestamptz,
    available_at            timestamptz NOT NULL,
    model_version           text CHECK (char_length(model_version) BETWEEN 1 AND 120),
    failure_reason          text CHECK (char_length(failure_reason) BETWEEN 1 AND 2000),
    created_by              uuid,
    created_at              timestamptz NOT NULL DEFAULT now(),
    finished_at             timestamptz,
    CONSTRAINT analysis_runs_test_fkey FOREIGN KEY (experiment_id, test_id) REFERENCES misko.tests (experiment_id, id),
    CONSTRAINT analysis_runs_recording_fkey FOREIGN KEY (test_id, recording_id) REFERENCES misko.test_recordings (test_id, id),
    CONSTRAINT analysis_runs_source_fkey FOREIGN KEY (recording_id, source_asset_id) REFERENCES misko.test_recordings (id, video_asset_id),
    CONSTRAINT analysis_runs_calibration_fkey FOREIGN KEY (recording_id, calibration_id) REFERENCES misko.calibrations (recording_id, id),
    CONSTRAINT analysis_runs_protocol_fkey FOREIGN KEY (experiment_id, protocol_version_id) REFERENCES misko.protocol_versions (experiment_id, id),
    CONSTRAINT analysis_runs_attempts_check CHECK (attempt <= max_attempts),
    CONSTRAINT analysis_runs_state_check CHECK (
        (status = 'QUEUED' AND worker_id IS NULL AND lease_expires_at IS NULL AND finished_at IS NULL)
        OR (status = 'RUNNING' AND worker_id IS NOT NULL AND lease_expires_at IS NOT NULL AND attempt >= 1 AND finished_at IS NULL)
        OR (status = 'SUCCEEDED' AND worker_id IS NOT NULL AND model_version IS NOT NULL AND attempt >= 1 AND lease_expires_at IS NULL AND finished_at IS NOT NULL)
        OR (status = 'FAILED' AND lease_expires_at IS NULL AND failure_reason IS NOT NULL AND finished_at IS NOT NULL))
);

-- Concurrent schedulers create one automatic run per source generation,
-- calibration and paradigm version.
CREATE UNIQUE INDEX analysis_runs_automatic_key ON misko.analysis_runs
    (recording_id, source_generation, paradigm_key, paradigm_version, coalesce(calibration_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE trigger = 'AUTOMATIC';
CREATE INDEX analysis_runs_queue_idx ON misko.analysis_runs (available_at, id) WHERE status = 'QUEUED';
CREATE INDEX analysis_runs_lease_idx ON misko.analysis_runs (lease_expires_at) WHERE status = 'RUNNING';
CREATE INDEX analysis_runs_test_idx ON misko.analysis_runs (test_id, created_at);

-- Pinned inputs never change, finished runs never change, and the status and
-- attempt move only forward.
CREATE FUNCTION misko.check_analysis_run_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE'
       OR OLD.status IN ('SUCCEEDED', 'FAILED')
       OR ROW(NEW.experiment_id, NEW.test_id, NEW.recording_id, NEW.source_asset_id, NEW.source_generation, NEW.source_crc32c,
              NEW.clip_start_us, NEW.clip_end_us, NEW.calibration_id, NEW.paradigm_key, NEW.paradigm_version, NEW.metric_engine_version,
              NEW.result_schema_version, NEW.environment_revision_id, NEW.protocol_version_id, NEW.parameters, NEW.trigger,
              NEW.max_attempts, NEW.created_by, NEW.created_at)
          IS DISTINCT FROM
          ROW(OLD.experiment_id, OLD.test_id, OLD.recording_id, OLD.source_asset_id, OLD.source_generation, OLD.source_crc32c,
              OLD.clip_start_us, OLD.clip_end_us, OLD.calibration_id, OLD.paradigm_key, OLD.paradigm_version, OLD.metric_engine_version,
              OLD.result_schema_version, OLD.environment_revision_id, OLD.protocol_version_id, OLD.parameters, OLD.trigger,
              OLD.max_attempts, OLD.created_by, OLD.created_at)
       OR NOT ((OLD.status = 'QUEUED' AND NEW.status = 'RUNNING' AND NEW.attempt = OLD.attempt + 1)
               OR (OLD.status = 'RUNNING' AND NEW.status IN ('RUNNING', 'QUEUED', 'SUCCEEDED', 'FAILED') AND NEW.attempt = OLD.attempt)) THEN
        RAISE EXCEPTION 'analysis run % cannot change from % to %', OLD.id, OLD.status, NEW.status
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_runs_transition';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER analysis_runs_change BEFORE UPDATE OR DELETE ON misko.analysis_runs
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_run_change();

-- Output rows may be written only by the current attempt of a running run.
CREATE FUNCTION misko.check_analysis_output_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    run_status  text;
    run_attempt integer;
BEGIN
    SELECT r.status, r.attempt INTO run_status, run_attempt FROM misko.analysis_runs r WHERE r.id = NEW.run_id FOR SHARE;
    IF run_status IS DISTINCT FROM 'RUNNING' OR run_attempt IS DISTINCT FROM NEW.attempt THEN
        RAISE EXCEPTION 'outputs of run % need its current running attempt', NEW.run_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_outputs_fenced';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TABLE misko.analysis_output_uploads (
    run_id       uuid NOT NULL REFERENCES misko.analysis_runs (id),
    attempt      integer NOT NULL CHECK (attempt >= 1),
    object_name  text NOT NULL CHECK (char_length(object_name) BETWEEN 1 AND 1024),
    kind         text NOT NULL CHECK (kind IN ('ANALYZED_VIDEO', 'TRAJECTORY', 'THUMBNAIL')),
    content_type text NOT NULL CHECK (char_length(content_type) BETWEEN 1 AND 100),
    size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
    crc32c       bigint NOT NULL CHECK (crc32c BETWEEN 0 AND 4294967295),
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT analysis_output_uploads_pkey PRIMARY KEY (run_id, object_name)
);

CREATE TABLE misko.analysis_artifacts (
    run_id         uuid NOT NULL REFERENCES misko.analysis_runs (id),
    attempt        integer NOT NULL,
    object_name    text NOT NULL,
    kind           text NOT NULL CHECK (kind IN ('ANALYZED_VIDEO', 'TRAJECTORY', 'THUMBNAIL')),
    bucket         text NOT NULL,
    generation     bigint NOT NULL CHECK (generation > 0),
    size_bytes     bigint NOT NULL CHECK (size_bytes > 0),
    crc32c         bigint NOT NULL CHECK (crc32c BETWEEN 0 AND 4294967295),
    content_type   text NOT NULL,
    video_asset_id uuid REFERENCES misko.video_assets (id),
    CONSTRAINT analysis_artifacts_pkey PRIMARY KEY (run_id, object_name),
    CONSTRAINT analysis_artifacts_upload_fkey FOREIGN KEY (run_id, object_name) REFERENCES misko.analysis_output_uploads (run_id, object_name),
    CONSTRAINT analysis_artifacts_video_check CHECK ((kind = 'ANALYZED_VIDEO') = (video_asset_id IS NOT NULL))
);

-- Each run pairs its own analyzed video with the source generation it analyzed.
CREATE TABLE misko.analysis_video_pairs (
    run_id               uuid PRIMARY KEY REFERENCES misko.analysis_runs (id),
    attempt              integer NOT NULL,
    source_asset_id      uuid NOT NULL REFERENCES misko.video_assets (id),
    source_generation    bigint NOT NULL CHECK (source_generation > 0),
    analyzed_asset_id    uuid NOT NULL REFERENCES misko.video_assets (id),
    source_offset_us     bigint NOT NULL CHECK (source_offset_us >= 0),
    output_offset_us     bigint NOT NULL CHECK (output_offset_us >= 0),
    time_mapping_version text NOT NULL CHECK (char_length(time_mapping_version) BETWEEN 1 AND 60),
    CONSTRAINT analysis_video_pairs_analyzed_key UNIQUE (analyzed_asset_id)
);

CREATE TABLE misko.metric_results (
    run_id         uuid NOT NULL REFERENCES misko.analysis_runs (id),
    attempt        integer NOT NULL,
    metric_key     text NOT NULL CHECK (char_length(metric_key) BETWEEN 1 AND 80),
    unit           text NOT NULL,
    value          double precision,
    missing_reason text CHECK (missing_reason ~ '^[A-Z_]{1,60}$'),
    CONSTRAINT metric_results_pkey PRIMARY KEY (run_id, metric_key),
    CONSTRAINT metric_results_value_check CHECK ((value IS NULL) <> (missing_reason IS NULL)
        AND (value IS NULL OR value NOT IN ('NaN'::double precision, 'Infinity'::double precision, '-Infinity'::double precision)))
);

-- Times are microseconds relative to the recording (clip) start.
CREATE TABLE misko.analysis_events (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    run_id     uuid NOT NULL REFERENCES misko.analysis_runs (id),
    attempt    integer NOT NULL,
    test_id    uuid NOT NULL,
    event_type text NOT NULL CHECK (char_length(event_type) BETWEEN 1 AND 80),
    kind       text NOT NULL CHECK (kind IN ('INTERVAL', 'POINT')),
    start_us   bigint NOT NULL CHECK (start_us >= 0),
    end_us     bigint NOT NULL,
    confidence real NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    trial_id   uuid,
    CONSTRAINT analysis_events_time_check CHECK ((kind = 'POINT' AND end_us = start_us) OR (kind = 'INTERVAL' AND end_us > start_us)),
    CONSTRAINT analysis_events_trial_fkey FOREIGN KEY (test_id, trial_id) REFERENCES misko.trials (test_id, id)
);

CREATE INDEX analysis_events_run_idx ON misko.analysis_events (run_id, start_us);

CREATE TRIGGER analysis_output_uploads_fenced BEFORE INSERT ON misko.analysis_output_uploads
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_output_insert();
CREATE TRIGGER analysis_artifacts_fenced BEFORE INSERT ON misko.analysis_artifacts
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_output_insert();
CREATE TRIGGER analysis_video_pairs_fenced BEFORE INSERT ON misko.analysis_video_pairs
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_output_insert();
CREATE TRIGGER metric_results_fenced BEFORE INSERT ON misko.metric_results
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_output_insert();
CREATE TRIGGER analysis_events_fenced BEFORE INSERT ON misko.analysis_events
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_output_insert();

CREATE TRIGGER analysis_output_uploads_immutable BEFORE UPDATE OR DELETE ON misko.analysis_output_uploads
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER analysis_artifacts_immutable BEFORE UPDATE OR DELETE ON misko.analysis_artifacts
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER analysis_video_pairs_immutable BEFORE UPDATE OR DELETE ON misko.analysis_video_pairs
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER metric_results_immutable BEFORE UPDATE OR DELETE ON misko.metric_results
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER analysis_events_immutable BEFORE UPDATE OR DELETE ON misko.analysis_events
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- A run becomes SUCCEEDED only with its video pair and analyzed video in the same transaction.
CREATE FUNCTION misko.check_analysis_published() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status = 'SUCCEEDED' AND NOT EXISTS (
        SELECT 1 FROM misko.analysis_video_pairs p
        JOIN misko.analysis_artifacts a ON a.run_id = p.run_id AND a.video_asset_id = p.analyzed_asset_id
        WHERE p.run_id = NEW.id AND p.attempt = NEW.attempt) THEN
        RAISE EXCEPTION 'run % succeeded without its video pair', NEW.id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_runs_published';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER analysis_runs_published AFTER UPDATE ON misko.analysis_runs
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_published();
