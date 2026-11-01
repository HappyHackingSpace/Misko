-- Analysis becomes a service of its own. A job analyses "this video, with this
-- paradigm version, these parameters and this calibration" and knows nothing
-- about experiments: its input is an immutable snapshot (the calibration is
-- copied, not referenced) and its requester is an opaque reference, a test
-- subject or a paradigm trial. Jobs replace analysis runs; every existing run
-- keeps its id as a job, so leases held by workers stay valid.
--
-- The experiment side keeps what only it knows: which job analyses which
-- animal of a test (test_subject_analyses) and the review decision
-- (subject_results), which now points to a job.

-- The jobs. calibration_* columns are the calibration snapshot: all set when
-- the paradigm needs a calibration, all NULL otherwise. calibration_zones is
-- an array of {"key", "kind", "pixels", "centimeters"} shapes, a circle as
-- {"x", "y", "radius"} and a polygon as an array of {"x", "y"} vertices.
-- idempotency_key lets a requester submit the same automatic job twice and get
-- the first one back.
CREATE TABLE misko.analysis_jobs (
    id                       uuid PRIMARY KEY DEFAULT uuidv7(),
    requester_kind           text NOT NULL CHECK (requester_kind IN ('TRIAL', 'TEST_SUBJECT')),
    requester_id             uuid NOT NULL,
    idempotency_key          text CHECK (char_length(idempotency_key) BETWEEN 1 AND 200),
    paradigm_key             text NOT NULL CHECK (paradigm_key ~ '^[A-Z][A-Z_]{0,39}$'),
    paradigm_version         integer NOT NULL CHECK (paradigm_version >= 1),
    metric_engine_version    integer NOT NULL CHECK (metric_engine_version >= 1),
    result_schema_version    integer NOT NULL CHECK (result_schema_version >= 1),
    parameters               jsonb NOT NULL CHECK (jsonb_typeof(parameters) = 'object'),
    source_asset_id          uuid NOT NULL REFERENCES misko.video_assets (id),
    source_generation        bigint NOT NULL CHECK (source_generation > 0),
    source_crc32c            bigint NOT NULL CHECK (source_crc32c BETWEEN 0 AND 4294967295),
    clip_start_us            bigint NOT NULL CHECK (clip_start_us >= 0),
    clip_end_us              bigint,
    calibration_id           uuid,
    calibration_frame_width  integer CHECK (calibration_frame_width BETWEEN 1 AND 16384),
    calibration_frame_height integer CHECK (calibration_frame_height BETWEEN 1 AND 16384),
    calibration_crop_x       integer,
    calibration_crop_y       integer,
    calibration_crop_width   integer,
    calibration_crop_height  integer,
    calibration_plane        text CHECK (calibration_plane IN ('ARENA_FLOOR', 'WATER_SURFACE', 'APPARATUS_TOP', 'APPARATUS_SIDE')),
    calibration_transform    double precision[] CHECK (cardinality(calibration_transform) = 9),
    calibration_zones        jsonb CHECK (jsonb_typeof(calibration_zones) = 'array'),
    trigger                  text NOT NULL CHECK (trigger IN ('AUTOMATIC', 'MANUAL')),
    status                   text NOT NULL CHECK (status IN ('QUEUED', 'RUNNING', 'SUCCEEDED', 'FAILED')),
    attempt                  integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts             integer NOT NULL CHECK (max_attempts BETWEEN 1 AND 20),
    worker_id                uuid REFERENCES misko.analysis_workers (id),
    lease_expires_at         timestamptz,
    available_at             timestamptz NOT NULL,
    model_version            text CHECK (char_length(model_version) BETWEEN 1 AND 120),
    failure_reason           text CHECK (char_length(failure_reason) BETWEEN 1 AND 2000),
    created_by               uuid,
    created_at               timestamptz NOT NULL DEFAULT now(),
    finished_at              timestamptz,
    CONSTRAINT analysis_jobs_attempts_check CHECK (attempt <= max_attempts),
    CONSTRAINT analysis_jobs_clip_check CHECK (clip_end_us IS NULL OR clip_end_us > clip_start_us),
    CONSTRAINT analysis_jobs_calibration_check CHECK (
        (calibration_id IS NULL AND calibration_frame_width IS NULL AND calibration_frame_height IS NULL AND calibration_crop_x IS NULL
            AND calibration_crop_y IS NULL AND calibration_crop_width IS NULL AND calibration_crop_height IS NULL AND calibration_plane IS NULL
            AND calibration_transform IS NULL AND calibration_zones IS NULL)
        OR (calibration_id IS NOT NULL AND calibration_frame_width IS NOT NULL AND calibration_frame_height IS NOT NULL
            AND calibration_plane IS NOT NULL AND calibration_transform IS NOT NULL AND calibration_zones IS NOT NULL
            AND calibration_crop_x >= 0 AND calibration_crop_y >= 0 AND calibration_crop_width > 0 AND calibration_crop_height > 0
            AND calibration_crop_x + calibration_crop_width <= calibration_frame_width
            AND calibration_crop_y + calibration_crop_height <= calibration_frame_height)),
    CONSTRAINT analysis_jobs_state_check CHECK (
        (status = 'QUEUED' AND worker_id IS NULL AND lease_expires_at IS NULL AND finished_at IS NULL)
        OR (status = 'RUNNING' AND worker_id IS NOT NULL AND lease_expires_at IS NOT NULL AND attempt >= 1 AND finished_at IS NULL)
        OR (status = 'SUCCEEDED' AND worker_id IS NOT NULL AND model_version IS NOT NULL AND attempt >= 1 AND lease_expires_at IS NULL AND finished_at IS NOT NULL)
        OR (status = 'FAILED' AND lease_expires_at IS NULL AND failure_reason IS NOT NULL AND finished_at IS NOT NULL))
);

CREATE UNIQUE INDEX analysis_jobs_idempotency_key ON misko.analysis_jobs (idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX analysis_jobs_queue_idx ON misko.analysis_jobs (available_at, id) WHERE status = 'QUEUED';
CREATE INDEX analysis_jobs_lease_idx ON misko.analysis_jobs (lease_expires_at) WHERE status = 'RUNNING';
CREATE INDEX analysis_jobs_requester_idx ON misko.analysis_jobs (requester_kind, requester_id, created_at);
CREATE INDEX analysis_jobs_source_idx ON misko.analysis_jobs (source_asset_id);

-- An output the worker declared before uploading it.
CREATE TABLE misko.analysis_job_uploads (
    job_id       uuid NOT NULL REFERENCES misko.analysis_jobs (id),
    attempt      integer NOT NULL CHECK (attempt >= 1),
    object_name  text NOT NULL CHECK (char_length(object_name) BETWEEN 1 AND 1024),
    kind         text NOT NULL CHECK (kind IN ('ANALYZED_VIDEO', 'TRAJECTORY', 'THUMBNAIL')),
    content_type text NOT NULL CHECK (char_length(content_type) BETWEEN 1 AND 100),
    size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
    crc32c       bigint NOT NULL CHECK (crc32c BETWEEN 0 AND 4294967295),
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT analysis_job_uploads_pkey PRIMARY KEY (job_id, object_name)
);

-- An output verified in storage and published with its job.
CREATE TABLE misko.analysis_job_outputs (
    job_id       uuid NOT NULL,
    attempt      integer NOT NULL,
    object_name  text NOT NULL,
    kind         text NOT NULL CHECK (kind IN ('ANALYZED_VIDEO', 'TRAJECTORY', 'THUMBNAIL')),
    bucket       text NOT NULL,
    generation   bigint NOT NULL CHECK (generation > 0),
    size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
    crc32c       bigint NOT NULL CHECK (crc32c BETWEEN 0 AND 4294967295),
    content_type text NOT NULL,
    CONSTRAINT analysis_job_outputs_pkey PRIMARY KEY (job_id, object_name),
    CONSTRAINT analysis_job_outputs_upload_fkey FOREIGN KEY (job_id, object_name) REFERENCES misko.analysis_job_uploads (job_id, object_name)
);

-- Each job pairs its analysed video with the source generation it analysed:
-- source time = source_offset_us + recording time, output time =
-- output_offset_us + recording time.
CREATE TABLE misko.analysis_job_pairs (
    job_id               uuid PRIMARY KEY,
    attempt              integer NOT NULL,
    analyzed_object_name text NOT NULL,
    source_offset_us     bigint NOT NULL CHECK (source_offset_us >= 0),
    output_offset_us     bigint NOT NULL CHECK (output_offset_us >= 0),
    time_mapping_version text NOT NULL CHECK (char_length(time_mapping_version) BETWEEN 1 AND 60),
    CONSTRAINT analysis_job_pairs_output_fkey FOREIGN KEY (job_id, analyzed_object_name) REFERENCES misko.analysis_job_outputs (job_id, object_name)
);

CREATE TABLE misko.analysis_job_metrics (
    job_id         uuid NOT NULL REFERENCES misko.analysis_jobs (id),
    attempt        integer NOT NULL,
    metric_key     text NOT NULL CHECK (metric_key ~ '^[a-z][a-z0-9_]{0,79}$'),
    unit           text NOT NULL,
    value          double precision,
    missing_reason text CHECK (missing_reason ~ '^[A-Z_]{1,60}$'),
    CONSTRAINT analysis_job_metrics_pkey PRIMARY KEY (job_id, metric_key),
    CONSTRAINT analysis_job_metrics_value_check CHECK ((value IS NULL) <> (missing_reason IS NULL)
        AND (value IS NULL OR value NOT IN ('NaN'::double precision, 'Infinity'::double precision, '-Infinity'::double precision)))
);

-- Derived events in microseconds from the clip start.
CREATE TABLE misko.analysis_job_events (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    job_id     uuid NOT NULL REFERENCES misko.analysis_jobs (id),
    attempt    integer NOT NULL,
    event_type text NOT NULL CHECK (char_length(event_type) BETWEEN 1 AND 80),
    label      text CHECK (label ~ '^[a-z][a-z0-9_]{0,39}$'),
    kind       text NOT NULL CHECK (kind IN ('INTERVAL', 'POINT')),
    start_us   bigint NOT NULL CHECK (start_us >= 0),
    end_us     bigint NOT NULL,
    confidence real NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    CONSTRAINT analysis_job_events_time_check CHECK ((kind = 'POINT' AND end_us = start_us) OR (kind = 'INTERVAL' AND end_us > start_us))
);

CREATE INDEX analysis_job_events_job_idx ON misko.analysis_job_events (job_id, start_us);

-- The tracking quality of the trajectory the job's metrics were computed
-- from, stored whether QC passed or failed the job.
CREATE TABLE misko.analysis_job_qc (
    job_id          uuid PRIMARY KEY REFERENCES misko.analysis_jobs (id),
    attempt         integer NOT NULL,
    samples         integer NOT NULL CHECK (samples >= 0),
    tracked         integer NOT NULL CHECK (tracked BETWEEN 0 AND samples),
    inferred        integer NOT NULL CHECK (inferred BETWEEN 0 AND tracked),
    lost_ratio      double precision NOT NULL CHECK (lost_ratio BETWEEN 0 AND 1),
    inferred_ratio  double precision NOT NULL CHECK (inferred_ratio BETWEEN 0 AND 1),
    mean_confidence double precision NOT NULL CHECK (mean_confidence BETWEEN 0 AND 1),
    passed          boolean NOT NULL
);

-- The experiment side: which job analyses which animal of a test, from which
-- of the test's videos and with which effective calibration, and why.
CREATE TABLE misko.test_subject_analyses (
    job_id          uuid PRIMARY KEY REFERENCES misko.analysis_jobs (id),
    experiment_id   uuid NOT NULL,
    test_id         uuid NOT NULL,
    test_subject_id uuid NOT NULL,
    recording_id    uuid NOT NULL,
    calibration_id  uuid,
    trigger         text NOT NULL CHECK (trigger IN ('AUTOMATIC', 'MANUAL')),
    created_by      uuid,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT test_subject_analyses_test_fkey FOREIGN KEY (experiment_id, test_id) REFERENCES misko.tests (experiment_id, id),
    CONSTRAINT test_subject_analyses_test_subject_fkey FOREIGN KEY (test_id, test_subject_id) REFERENCES misko.test_subjects (test_id, id),
    CONSTRAINT test_subject_analyses_recording_fkey FOREIGN KEY (test_id, recording_id) REFERENCES misko.test_recordings (test_id, id),
    CONSTRAINT test_subject_analyses_scope_key UNIQUE (test_subject_id, job_id)
);

-- Concurrent orchestrators submit one automatic job per animal, video and
-- calibration.
CREATE UNIQUE INDEX test_subject_analyses_automatic_key ON misko.test_subject_analyses
    (test_subject_id, recording_id, coalesce(calibration_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE trigger = 'AUTOMATIC';
CREATE INDEX test_subject_analyses_test_idx ON misko.test_subject_analyses (test_id, created_at);

-- Moving the runs. Each run becomes a job with the same id, requested by its
-- test subject, with the calibration it pinned copied into the snapshot.
INSERT INTO misko.analysis_jobs (id, requester_kind, requester_id, idempotency_key, paradigm_key, paradigm_version, metric_engine_version,
    result_schema_version, parameters, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
    calibration_id, calibration_frame_width, calibration_frame_height, calibration_crop_x, calibration_crop_y, calibration_crop_width,
    calibration_crop_height, calibration_plane, calibration_transform, calibration_zones,
    trigger, status, attempt, max_attempts, worker_id, lease_expires_at, available_at, model_version, failure_reason, created_by, created_at, finished_at)
SELECT r.id, 'TEST_SUBJECT', r.test_subject_id,
       CASE WHEN r.trigger = 'AUTOMATIC' THEN 'test-subject:' || r.test_subject_id || ':recording:' || r.recording_id || ':calibration:'
            || coalesce(r.calibration_id::text, 'none') END,
       r.paradigm_key, r.paradigm_version, r.metric_engine_version, r.result_schema_version, r.parameters,
       r.source_asset_id, r.source_generation, r.source_crc32c, r.clip_start_us, r.clip_end_us,
       c.id, c.frame_width, c.frame_height, c.crop_x, c.crop_y, c.crop_width, c.crop_height, c.measurement_plane, c.transform,
       CASE WHEN c.id IS NOT NULL THEN coalesce((
           SELECT jsonb_agg(jsonb_build_object('key', z.zone_key, 'kind', z.kind, 'pixels', z.pixels, 'centimeters', z.centimeters)
                            ORDER BY z.zone_key, z.position)
           FROM misko.calibration_zones z WHERE z.calibration_id = c.id), '[]'::jsonb) END,
       r.trigger, r.status, r.attempt, r.max_attempts, r.worker_id, r.lease_expires_at, r.available_at, r.model_version, r.failure_reason,
       r.created_by, r.created_at, r.finished_at
FROM misko.analysis_runs r
LEFT JOIN misko.calibrations c ON c.id = r.calibration_id;

INSERT INTO misko.analysis_job_uploads (job_id, attempt, object_name, kind, content_type, size_bytes, crc32c, created_at)
SELECT run_id, attempt, object_name, kind, content_type, size_bytes, crc32c, created_at FROM misko.analysis_output_uploads;

INSERT INTO misko.analysis_job_outputs (job_id, attempt, object_name, kind, bucket, generation, size_bytes, crc32c, content_type)
SELECT run_id, attempt, object_name, kind, bucket, generation, size_bytes, crc32c, content_type FROM misko.analysis_artifacts;

INSERT INTO misko.analysis_job_pairs (job_id, attempt, analyzed_object_name, source_offset_us, output_offset_us, time_mapping_version)
SELECT p.run_id, p.attempt, a.object_name, p.source_offset_us, p.output_offset_us, p.time_mapping_version
FROM misko.analysis_video_pairs p
JOIN misko.video_assets a ON a.id = p.analyzed_asset_id;

INSERT INTO misko.analysis_job_metrics (job_id, attempt, metric_key, unit, value, missing_reason)
SELECT run_id, attempt, metric_key, unit, value, missing_reason FROM misko.metric_results;

INSERT INTO misko.analysis_job_events (id, job_id, attempt, event_type, label, kind, start_us, end_us, confidence)
SELECT id, run_id, attempt, event_type, label, kind, start_us, end_us, confidence FROM misko.analysis_events;

INSERT INTO misko.test_subject_analyses (job_id, experiment_id, test_id, test_subject_id, recording_id, calibration_id, trigger, created_by, created_at)
SELECT id, experiment_id, test_id, test_subject_id, recording_id, calibration_id, trigger, created_by, created_at FROM misko.analysis_runs;

-- The review decision points to a job of the same animal.
DROP VIEW misko.report_accepted_metrics;
DROP VIEW misko.report_test_subjects;
ALTER TABLE misko.subject_results DROP CONSTRAINT subject_results_run_fkey;
ALTER TABLE misko.subject_results RENAME COLUMN run_id TO job_id;
ALTER TABLE misko.subject_results ADD CONSTRAINT subject_results_job_fkey
    FOREIGN KEY (test_subject_id, job_id) REFERENCES misko.test_subject_analyses (test_subject_id, job_id);

-- The chosen job must have succeeded.
CREATE OR REPLACE FUNCTION misko.check_subject_result() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (SELECT j.status FROM misko.analysis_jobs j WHERE j.id = NEW.job_id) IS DISTINCT FROM 'SUCCEEDED' THEN
        RAISE EXCEPTION 'result of test subject % must point to a succeeded job', NEW.test_subject_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'subject_results_succeeded';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

-- The analysis runs and what hung off them are replaced.
DROP TABLE misko.analysis_events;
DROP TABLE misko.metric_results;
DROP TABLE misko.analysis_video_pairs;
DROP TABLE misko.analysis_artifacts;
DROP TABLE misko.analysis_output_uploads;
DROP TABLE misko.analysis_runs;
DROP FUNCTION misko.check_analysis_run_calibration();
DROP FUNCTION misko.check_analysis_run_change();
DROP FUNCTION misko.check_analysis_output_insert();
DROP FUNCTION misko.check_analysis_published();

-- Rules of the jobs, installed after the existing rows moved in.

-- The input snapshot never changes, finished jobs never change, and the
-- status and attempt move only forward.
CREATE FUNCTION misko.check_analysis_job_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE'
       OR OLD.status IN ('SUCCEEDED', 'FAILED')
       OR ROW(NEW.requester_kind, NEW.requester_id, NEW.idempotency_key, NEW.paradigm_key, NEW.paradigm_version, NEW.metric_engine_version,
              NEW.result_schema_version, NEW.parameters, NEW.source_asset_id, NEW.source_generation, NEW.source_crc32c, NEW.clip_start_us,
              NEW.clip_end_us, NEW.calibration_id, NEW.calibration_frame_width, NEW.calibration_frame_height, NEW.calibration_crop_x,
              NEW.calibration_crop_y, NEW.calibration_crop_width, NEW.calibration_crop_height, NEW.calibration_plane, NEW.calibration_transform,
              NEW.calibration_zones, NEW.trigger, NEW.max_attempts, NEW.created_by, NEW.created_at)
          IS DISTINCT FROM
          ROW(OLD.requester_kind, OLD.requester_id, OLD.idempotency_key, OLD.paradigm_key, OLD.paradigm_version, OLD.metric_engine_version,
              OLD.result_schema_version, OLD.parameters, OLD.source_asset_id, OLD.source_generation, OLD.source_crc32c, OLD.clip_start_us,
              OLD.clip_end_us, OLD.calibration_id, OLD.calibration_frame_width, OLD.calibration_frame_height, OLD.calibration_crop_x,
              OLD.calibration_crop_y, OLD.calibration_crop_width, OLD.calibration_crop_height, OLD.calibration_plane, OLD.calibration_transform,
              OLD.calibration_zones, OLD.trigger, OLD.max_attempts, OLD.created_by, OLD.created_at)
       OR NOT ((OLD.status = 'QUEUED' AND NEW.status = 'RUNNING' AND NEW.attempt = OLD.attempt + 1)
               OR (OLD.status = 'RUNNING' AND NEW.status IN ('RUNNING', 'QUEUED', 'SUCCEEDED', 'FAILED') AND NEW.attempt = OLD.attempt)) THEN
        RAISE EXCEPTION 'analysis job % cannot change from % to %', OLD.id, OLD.status, NEW.status
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_jobs_transition';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER analysis_jobs_change BEFORE UPDATE OR DELETE ON misko.analysis_jobs
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_job_change();

-- Output rows may be written only by the current attempt of a running job.
CREATE FUNCTION misko.check_analysis_job_output() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    job_status  text;
    job_attempt integer;
BEGIN
    SELECT j.status, j.attempt INTO job_status, job_attempt FROM misko.analysis_jobs j WHERE j.id = NEW.job_id FOR SHARE;
    IF job_status IS DISTINCT FROM 'RUNNING' OR job_attempt IS DISTINCT FROM NEW.attempt THEN
        RAISE EXCEPTION 'outputs of job % need its current running attempt', NEW.job_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_job_outputs_fenced';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER analysis_job_uploads_fenced BEFORE INSERT ON misko.analysis_job_uploads
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_job_output();
CREATE TRIGGER analysis_job_outputs_fenced BEFORE INSERT ON misko.analysis_job_outputs
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_job_output();
CREATE TRIGGER analysis_job_pairs_fenced BEFORE INSERT ON misko.analysis_job_pairs
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_job_output();
CREATE TRIGGER analysis_job_metrics_fenced BEFORE INSERT ON misko.analysis_job_metrics
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_job_output();
CREATE TRIGGER analysis_job_events_fenced BEFORE INSERT ON misko.analysis_job_events
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_job_output();
CREATE TRIGGER analysis_job_qc_fenced BEFORE INSERT ON misko.analysis_job_qc
    FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_job_output();

CREATE TRIGGER analysis_job_uploads_immutable BEFORE UPDATE OR DELETE ON misko.analysis_job_uploads
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER analysis_job_outputs_immutable BEFORE UPDATE OR DELETE ON misko.analysis_job_outputs
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER analysis_job_pairs_immutable BEFORE UPDATE OR DELETE ON misko.analysis_job_pairs
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER analysis_job_metrics_immutable BEFORE UPDATE OR DELETE ON misko.analysis_job_metrics
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER analysis_job_events_immutable BEFORE UPDATE OR DELETE ON misko.analysis_job_events
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER analysis_job_qc_immutable BEFORE UPDATE OR DELETE ON misko.analysis_job_qc
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- A job becomes SUCCEEDED only with its video pair and analysed video in the
-- same transaction.
CREATE FUNCTION misko.check_analysis_job_published() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status = 'SUCCEEDED' AND NOT EXISTS (
        SELECT 1 FROM misko.analysis_job_pairs p
        JOIN misko.analysis_job_outputs o ON o.job_id = p.job_id AND o.object_name = p.analyzed_object_name AND o.kind = 'ANALYZED_VIDEO'
        WHERE p.job_id = NEW.id AND p.attempt = NEW.attempt) THEN
        RAISE EXCEPTION 'job % succeeded without its video pair', NEW.id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_jobs_published';
    END IF;
    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER analysis_jobs_published AFTER UPDATE ON misko.analysis_jobs
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION misko.check_analysis_job_published();

-- A link names a job its own test subject requested; links never change.
CREATE FUNCTION misko.check_test_subject_analysis() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM misko.analysis_jobs j
                   WHERE j.id = NEW.job_id AND j.requester_kind = 'TEST_SUBJECT' AND j.requester_id = NEW.test_subject_id) THEN
        RAISE EXCEPTION 'job % was not requested by test subject %', NEW.job_id, NEW.test_subject_id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'test_subject_analyses_requester';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER test_subject_analyses_requester BEFORE INSERT ON misko.test_subject_analyses
    FOR EACH ROW EXECUTE FUNCTION misko.check_test_subject_analysis();
CREATE TRIGGER test_subject_analyses_immutable BEFORE UPDATE OR DELETE ON misko.test_subject_analyses
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();

-- Every job of an animal of a test with the experiment facts of its link: the
-- read model reports and the dashboard use instead of the analysis tables.
CREATE VIEW misko.test_subject_jobs AS
SELECT l.job_id, l.experiment_id, l.test_id, l.test_subject_id, l.recording_id, l.calibration_id, l.trigger, l.created_by, l.created_at,
       j.paradigm_key, j.paradigm_version, j.status, j.metric_engine_version, j.result_schema_version, j.model_version, j.finished_at
FROM misko.test_subject_analyses l
JOIN misko.analysis_jobs j ON j.id = l.job_id;

-- Every animal of every test with the design it belongs to, as in
-- 012_reports.sql; its result now names a job.
CREATE VIEW misko.report_test_subjects AS
SELECT ts.id AS test_subject_id, ts.test_id, ts.experiment_id,
       e.code AS experiment_code, e.blinded,
       a.id AS allocation_id, s.id AS subject_id, s.code AS subject_code, s.species, s.sex,
       g.id AS group_id, g.code AS group_code, g.name AS group_name, g.role AS group_role,
       g.disease_model_id, dm.name AS disease_model_name,
       tr.substance_id AS treatment_substance_id, sub.name AS treatment_substance_name,
       tr.dose_micro AS treatment_dose_micro, tr.unit AS treatment_unit,
       a.excluded_at IS NOT NULL AS excluded,
       tp.id AS timepoint_id, tp.name AS timepoint_name, tp.study_day,
       t.paradigm_key, t.paradigm_version, t.status AS test_status, t.scheduled_at,
       ts.environment_revision_id, er.environment_id, env.name AS environment_name, er.number AS environment_revision,
       res.status AS result_status, res.job_id AS result_job_id
FROM misko.test_subjects ts
JOIN misko.tests t ON t.id = ts.test_id
JOIN misko.experiments e ON e.id = ts.experiment_id
JOIN misko.allocations a ON a.id = ts.allocation_id
JOIN misko.subjects s ON s.id = a.subject_id
JOIN misko.experiment_groups g ON g.id = a.group_id
LEFT JOIN misko.disease_models dm ON dm.id = g.disease_model_id
LEFT JOIN misko.regimen_items tr ON tr.group_id = g.id AND tr.purpose = 'TREATMENT'
LEFT JOIN misko.substances sub ON sub.id = tr.substance_id
JOIN misko.timepoints tp ON tp.id = t.timepoint_id
JOIN misko.environment_revisions er ON er.id = ts.environment_revision_id
JOIN misko.environments env ON env.id = er.environment_id
LEFT JOIN misko.subject_results res ON res.test_subject_id = ts.id;

-- Accepted measurements only: the numbers statistics are computed from. Only
-- jobs linked to an animal of a test can be accepted, so a paradigm trial
-- never reaches a report.
CREATE VIEW misko.report_accepted_metrics AS
SELECT rts.*, m.metric_key, m.unit, m.value, m.missing_reason,
       j.metric_engine_version, j.result_schema_version, j.model_version, j.finished_at
FROM misko.report_test_subjects rts
JOIN misko.test_subject_jobs j ON j.job_id = rts.result_job_id AND j.test_subject_id = rts.test_subject_id
JOIN misko.analysis_job_metrics m ON m.job_id = j.job_id
WHERE rts.result_status = 'ACCEPTED' AND NOT rts.excluded;

CREATE TRIGGER test_subject_jobs_read_only INSTEAD OF INSERT OR UPDATE OR DELETE ON misko.test_subject_jobs
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER report_test_subjects_read_only INSTEAD OF INSERT OR UPDATE OR DELETE ON misko.report_test_subjects
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER report_accepted_metrics_read_only INSTEAD OF INSERT OR UPDATE OR DELETE ON misko.report_accepted_metrics
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
