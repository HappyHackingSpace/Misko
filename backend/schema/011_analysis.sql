-- Automated analysis. Workers are service identities, separate from users;
-- only a SHA-256 hash of their token is stored.
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

-- A run analyses one animal of a test in one video: the job and its record.
-- Inputs are pinned when the run is created; attempt is the fencing token of
-- the current lease.
CREATE TABLE misko.analysis_runs (
    id                    uuid PRIMARY KEY DEFAULT uuidv7(),
    experiment_id         uuid NOT NULL,
    test_id               uuid NOT NULL,
    test_subject_id       uuid NOT NULL,
    recording_id          uuid NOT NULL,
    source_asset_id       uuid NOT NULL,
    source_generation     bigint NOT NULL CHECK (source_generation > 0),
    source_crc32c         bigint NOT NULL CHECK (source_crc32c BETWEEN 0 AND 4294967295),
    clip_start_us         bigint NOT NULL CHECK (clip_start_us >= 0),
    clip_end_us           bigint,
    calibration_id        uuid REFERENCES misko.calibrations (id),
    paradigm_key          text NOT NULL,
    paradigm_version      integer NOT NULL,
    metric_engine_version integer NOT NULL CHECK (metric_engine_version >= 1),
    result_schema_version integer NOT NULL CHECK (result_schema_version >= 1),
    parameters            jsonb NOT NULL CHECK (jsonb_typeof(parameters) = 'object'),
    trigger               text NOT NULL CHECK (trigger IN ('AUTOMATIC', 'MANUAL')),
    status                text NOT NULL CHECK (status IN ('QUEUED', 'RUNNING', 'SUCCEEDED', 'FAILED')),
    attempt               integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts          integer NOT NULL CHECK (max_attempts BETWEEN 1 AND 20),
    worker_id             uuid REFERENCES misko.analysis_workers (id),
    lease_expires_at      timestamptz,
    available_at          timestamptz NOT NULL,
    model_version         text CHECK (char_length(model_version) BETWEEN 1 AND 120),
    failure_reason        text CHECK (char_length(failure_reason) BETWEEN 1 AND 2000),
    created_by            uuid,
    created_at            timestamptz NOT NULL DEFAULT now(),
    finished_at           timestamptz,
    CONSTRAINT analysis_runs_test_fkey FOREIGN KEY (experiment_id, test_id) REFERENCES misko.tests (experiment_id, id),
    CONSTRAINT analysis_runs_test_subject_fkey FOREIGN KEY (test_id, test_subject_id) REFERENCES misko.test_subjects (test_id, id),
    CONSTRAINT analysis_runs_paradigm_fkey FOREIGN KEY (test_id, paradigm_key, paradigm_version) REFERENCES misko.tests (id, paradigm_key, paradigm_version),
    CONSTRAINT analysis_runs_recording_fkey FOREIGN KEY (test_id, recording_id) REFERENCES misko.test_recordings (test_id, id),
    CONSTRAINT analysis_runs_source_fkey FOREIGN KEY (recording_id, source_asset_id) REFERENCES misko.test_recordings (id, video_asset_id),
    CONSTRAINT analysis_runs_attempts_check CHECK (attempt <= max_attempts),
    CONSTRAINT analysis_runs_clip_check CHECK (clip_end_us IS NULL OR clip_end_us > clip_start_us),
    CONSTRAINT analysis_runs_state_check CHECK (
        (status = 'QUEUED' AND worker_id IS NULL AND lease_expires_at IS NULL AND finished_at IS NULL)
        OR (status = 'RUNNING' AND worker_id IS NOT NULL AND lease_expires_at IS NOT NULL AND attempt >= 1 AND finished_at IS NULL)
        OR (status = 'SUCCEEDED' AND worker_id IS NOT NULL AND model_version IS NOT NULL AND attempt >= 1 AND lease_expires_at IS NULL AND finished_at IS NOT NULL)
        OR (status = 'FAILED' AND lease_expires_at IS NULL AND failure_reason IS NOT NULL AND finished_at IS NOT NULL)),
    CONSTRAINT analysis_runs_test_subject_scope_key UNIQUE (test_subject_id, id)
);

-- Concurrent schedulers create one automatic run per animal, video
-- generation, calibration and paradigm version.
CREATE UNIQUE INDEX analysis_runs_automatic_key ON misko.analysis_runs
    (test_subject_id, recording_id, source_generation, paradigm_key, paradigm_version,
     coalesce(calibration_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE trigger = 'AUTOMATIC';
CREATE INDEX analysis_runs_queue_idx ON misko.analysis_runs (available_at, id) WHERE status = 'QUEUED';
CREATE INDEX analysis_runs_lease_idx ON misko.analysis_runs (lease_expires_at) WHERE status = 'RUNNING';
CREATE INDEX analysis_runs_test_subject_idx ON misko.analysis_runs (test_subject_id, created_at);
CREATE INDEX analysis_runs_experiment_idx ON misko.analysis_runs (experiment_id, finished_at) WHERE status = 'SUCCEEDED';
CREATE INDEX analysis_runs_source_idx ON misko.analysis_runs (source_asset_id);

-- A run pins a VALID calibration: the default of its animal's arena, or an
-- override of that animal for the run's own video.
CREATE FUNCTION misko.check_analysis_run_calibration() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.calibration_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM misko.calibrations c JOIN misko.test_subjects s ON s.id = NEW.test_subject_id
        WHERE c.id = NEW.calibration_id AND c.status = 'VALID' AND c.environment_revision_id = s.environment_revision_id
          AND (c.test_subject_id IS NULL OR (c.test_subject_id = s.id AND c.recording_id = NEW.recording_id))) THEN
        RAISE EXCEPTION 'analysis run of test subject % cannot pin calibration %', NEW.test_subject_id, NEW.calibration_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_runs_calibration_scope';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER analysis_runs_calibration_scope BEFORE INSERT ON misko.analysis_runs
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_run_calibration();

-- Pinned inputs never change, finished runs never change, and the status and
-- attempt move only forward.
CREATE FUNCTION misko.check_analysis_run_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE'
       OR OLD.status IN ('SUCCEEDED', 'FAILED')
       OR ROW(NEW.experiment_id, NEW.test_id, NEW.test_subject_id, NEW.recording_id, NEW.source_asset_id, NEW.source_generation,
              NEW.source_crc32c, NEW.clip_start_us, NEW.clip_end_us, NEW.calibration_id, NEW.paradigm_key, NEW.paradigm_version,
              NEW.metric_engine_version, NEW.result_schema_version, NEW.parameters, NEW.trigger, NEW.max_attempts, NEW.created_by, NEW.created_at)
          IS DISTINCT FROM
          ROW(OLD.experiment_id, OLD.test_id, OLD.test_subject_id, OLD.recording_id, OLD.source_asset_id, OLD.source_generation,
              OLD.source_crc32c, OLD.clip_start_us, OLD.clip_end_us, OLD.calibration_id, OLD.paradigm_key, OLD.paradigm_version,
              OLD.metric_engine_version, OLD.result_schema_version, OLD.parameters, OLD.trigger, OLD.max_attempts, OLD.created_by, OLD.created_at)
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

-- An output the worker declared before uploading it.
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

-- An output verified in storage and published with its run.
CREATE TABLE misko.analysis_artifacts (
    run_id         uuid NOT NULL,
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

-- Each run pairs its analysed video with the source generation it analysed.
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
    metric_key     text NOT NULL CHECK (metric_key ~ '^[a-z][a-z0-9_]{0,79}$'),
    unit           text NOT NULL,
    value          double precision,
    missing_reason text CHECK (missing_reason ~ '^[A-Z_]{1,60}$'),
    CONSTRAINT metric_results_pkey PRIMARY KEY (run_id, metric_key),
    CONSTRAINT metric_results_value_check CHECK ((value IS NULL) <> (missing_reason IS NULL)
        AND (value IS NULL OR value NOT IN ('NaN'::double precision, 'Infinity'::double precision, '-Infinity'::double precision)))
);

-- Derived events in microseconds from the clip start, optionally within one of
-- the animal's trials.
CREATE TABLE misko.analysis_events (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    run_id          uuid NOT NULL,
    attempt         integer NOT NULL,
    test_subject_id uuid NOT NULL,
    event_type      text NOT NULL CHECK (char_length(event_type) BETWEEN 1 AND 80),
    kind            text NOT NULL CHECK (kind IN ('INTERVAL', 'POINT')),
    start_us        bigint NOT NULL CHECK (start_us >= 0),
    end_us          bigint NOT NULL,
    confidence      real NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    trial_id        uuid,
    CONSTRAINT analysis_events_run_fkey FOREIGN KEY (test_subject_id, run_id) REFERENCES misko.analysis_runs (test_subject_id, id),
    CONSTRAINT analysis_events_trial_fkey FOREIGN KEY (test_subject_id, trial_id) REFERENCES misko.trials (test_subject_id, id),
    CONSTRAINT analysis_events_time_check CHECK ((kind = 'POINT' AND end_us = start_us) OR (kind = 'INTERVAL' AND end_us > start_us))
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

-- A run becomes SUCCEEDED only with its video pair and analysed video in the
-- same transaction.
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

-- The human decision on which run counts for one animal of a test. The first
-- successful run opens it as PENDING; only ACCEPTED results enter statistics.
-- Choosing another run sends it back to PENDING.
CREATE TABLE misko.subject_results (
    test_subject_id uuid PRIMARY KEY REFERENCES misko.test_subjects (id),
    run_id          uuid NOT NULL,
    status          text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'ACCEPTED', 'REJECTED')),
    reason          text CHECK (reason IN ('TRACKING_LOST', 'WRONG_ANIMAL', 'VIDEO_DEFECT', 'PROTOCOL_DEVIATION', 'OTHER')),
    note            text CHECK (char_length(note) BETWEEN 1 AND 2000),
    reviewed_by     uuid,
    reviewed_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT subject_results_run_fkey FOREIGN KEY (test_subject_id, run_id) REFERENCES misko.analysis_runs (test_subject_id, id),
    CONSTRAINT subject_results_rejection_check CHECK ((status = 'REJECTED') = (reason IS NOT NULL)),
    CONSTRAINT subject_results_review_check CHECK ((status = 'PENDING') = (reviewed_by IS NULL) AND (reviewed_by IS NULL) = (reviewed_at IS NULL))
);

CREATE INDEX subject_results_pending_idx ON misko.subject_results (created_at) WHERE status = 'PENDING';

-- The chosen run must have succeeded.
CREATE FUNCTION misko.check_subject_result() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (SELECT r.status FROM misko.analysis_runs r WHERE r.id = NEW.run_id) IS DISTINCT FROM 'SUCCEEDED' THEN
        RAISE EXCEPTION 'result of test subject % must point to a succeeded run', NEW.test_subject_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'subject_results_succeeded';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER subject_results_succeeded BEFORE INSERT OR UPDATE ON misko.subject_results
    FOR EACH ROW EXECUTE FUNCTION misko.check_subject_result();
CREATE TRIGGER subject_results_no_delete BEFORE DELETE ON misko.subject_results
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
