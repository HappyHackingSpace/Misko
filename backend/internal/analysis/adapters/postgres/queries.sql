-- name: CreateWorker :one
INSERT INTO misko.analysis_workers (name, model_version, token_sha256, created_by)
VALUES (@name, @model_version, @token_sha256, @created_by)
RETURNING id, name, model_version, disabled_at, created_by, created_at;

-- name: AddCapability :exec
INSERT INTO misko.worker_capabilities (worker_id, paradigm_key, paradigm_version)
VALUES (@worker_id, @paradigm_key, @paradigm_version);

-- name: ListWorkers :many
SELECT id, name, model_version, disabled_at, created_by, created_at
FROM misko.analysis_workers
ORDER BY lower(name), id;

-- name: ListCapabilities :many
SELECT worker_id, paradigm_key, paradigm_version
FROM misko.worker_capabilities
ORDER BY worker_id, paradigm_key, paradigm_version;

-- name: DisableWorker :one
UPDATE misko.analysis_workers
SET disabled_at = coalesce(disabled_at, @at::timestamptz)
WHERE id = @id
RETURNING id, name, model_version, disabled_at, created_by, created_at;

-- name: WorkerByToken :one
SELECT id, name, model_version, disabled_at, created_by, created_at
FROM misko.analysis_workers
WHERE token_sha256 = @token_sha256 AND disabled_at IS NULL;

-- name: WorkerCapabilities :many
SELECT worker_id, paradigm_key, paradigm_version
FROM misko.worker_capabilities
WHERE worker_id = @worker_id
ORDER BY paradigm_key, paradigm_version;

-- name: ActiveCapabilities :many
SELECT DISTINCT c.paradigm_key, c.paradigm_version
FROM misko.worker_capabilities c
JOIN misko.analysis_workers w ON w.id = c.worker_id
WHERE w.disabled_at IS NULL
ORDER BY c.paradigm_key, c.paradigm_version;

-- ReadyRecordings lists verified recordings of tests that are not cancelled,
-- with an active capability and no automatic run for the current source
-- generation and calibration. An environment calibration never triggers a run for
-- a recording that already has an automatic run: correcting the default must not
-- silently reanalyze every recording, that stays an explicit manual run.
-- name: ReadyRecordings :many
WITH candidates AS (
    SELECT r.experiment_id, r.test_id, r.id AS recording_id, a.id AS source_asset_id, a.generation AS source_generation,
           a.crc32c AS source_crc32c, r.clip_start_us, r.clip_end_us, t.paradigm_key, t.paradigm_version,
           t.environment_revision_id, t.protocol_version_id, e.apparatus, s.session, r.created_at,
           coalesce(ec.calibration_id::text, '')::text AS calibration_id, coalesce(ec.source, '')::text AS calibration_source
    FROM misko.test_recordings r
    JOIN misko.effective_calibrations ec ON ec.recording_id = r.id
    JOIN misko.video_assets a ON a.id = r.video_asset_id
    JOIN misko.tests t ON t.id = r.test_id
    JOIN misko.environment_revisions e ON e.id = t.environment_revision_id
    JOIN misko.protocol_steps s ON s.protocol_version_id = t.protocol_version_id AND s.position = t.step_position
    WHERE a.status = 'VERIFIED' AND t.status <> 'CANCELLED'
      AND EXISTS (SELECT 1 FROM misko.worker_capabilities wc JOIN misko.analysis_workers w ON w.id = wc.worker_id
                  WHERE w.disabled_at IS NULL AND wc.paradigm_key = t.paradigm_key AND wc.paradigm_version = t.paradigm_version)
)
SELECT c.experiment_id, c.test_id, c.recording_id, c.source_asset_id, c.source_generation::bigint AS source_generation,
       c.source_crc32c, c.clip_start_us, c.clip_end_us, c.paradigm_key, c.paradigm_version, c.environment_revision_id,
       c.protocol_version_id, c.apparatus, c.session, c.calibration_id
FROM candidates c
WHERE NOT EXISTS (
    SELECT 1 FROM misko.analysis_runs x
    WHERE x.trigger = 'AUTOMATIC' AND x.recording_id = c.recording_id AND x.source_generation = c.source_generation
      AND x.paradigm_key = c.paradigm_key AND x.paradigm_version = c.paradigm_version
      AND (c.calibration_source = 'ENVIRONMENT' OR coalesce(x.calibration_id::text, '') IN (c.calibration_id, '')))
ORDER BY c.created_at, c.recording_id
LIMIT 500;

-- name: GetCandidate :one
SELECT r.experiment_id, r.test_id, r.id AS recording_id, a.id AS source_asset_id, coalesce(a.generation, 0)::bigint AS source_generation,
       a.crc32c AS source_crc32c, r.clip_start_us, r.clip_end_us, t.paradigm_key, t.paradigm_version,
       t.environment_revision_id, t.protocol_version_id, e.apparatus, s.session, a.status AS video_status, t.status AS test_status,
       coalesce(ec.calibration_id::text, '')::text AS calibration_id
FROM misko.test_recordings r
JOIN misko.effective_calibrations ec ON ec.recording_id = r.id
JOIN misko.video_assets a ON a.id = r.video_asset_id
JOIN misko.tests t ON t.id = r.test_id
JOIN misko.environment_revisions e ON e.id = t.environment_revision_id
JOIN misko.protocol_steps s ON s.protocol_version_id = t.protocol_version_id AND s.position = t.step_position
WHERE r.test_id = @test_id AND r.id = @id;

-- name: CreateAutomaticRun :execrows
INSERT INTO misko.analysis_runs (experiment_id, test_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
    calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, environment_revision_id, protocol_version_id,
    parameters, trigger, status, max_attempts, available_at, created_by)
VALUES (@experiment_id, @test_id, @recording_id, @source_asset_id, @source_generation, @source_crc32c, @clip_start_us, sqlc.narg(clip_end_us),
    sqlc.narg(calibration_id), @paradigm_key, @paradigm_version, @metric_engine_version, @result_schema_version, @environment_revision_id,
    @protocol_version_id, @parameters, 'AUTOMATIC', 'QUEUED', @max_attempts, @available_at, NULL)
ON CONFLICT DO NOTHING;

-- name: CreateManualRun :one
INSERT INTO misko.analysis_runs (experiment_id, test_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
    calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, environment_revision_id, protocol_version_id,
    parameters, trigger, status, max_attempts, available_at, created_by)
VALUES (@experiment_id, @test_id, @recording_id, @source_asset_id, @source_generation, @source_crc32c, @clip_start_us, sqlc.narg(clip_end_us),
    sqlc.narg(calibration_id), @paradigm_key, @paradigm_version, @metric_engine_version, @result_schema_version, @environment_revision_id,
    @protocol_version_id, @parameters, 'MANUAL', 'QUEUED', @max_attempts, @available_at, @created_by)
RETURNING *;

-- name: LockExpiredRuns :many
SELECT * FROM misko.analysis_runs
WHERE status = 'RUNNING' AND lease_expires_at < @now::timestamptz
ORDER BY lease_expires_at, id
LIMIT 100
FOR UPDATE SKIP LOCKED;

-- ClaimNext takes the oldest available run for a capability ("KEY@version"),
-- skipping runs another worker is claiming.
-- name: ClaimNext :one
SELECT * FROM misko.analysis_runs r
WHERE r.status = 'QUEUED' AND r.available_at <= @now::timestamptz
  AND (r.paradigm_key || '@' || r.paradigm_version::text) = ANY(@capabilities::text[])
ORDER BY r.available_at, r.id
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: UpdateRun :one
UPDATE misko.analysis_runs
SET status = @status, attempt = @attempt, worker_id = sqlc.narg(worker_id), lease_expires_at = sqlc.narg(lease_expires_at),
    available_at = @available_at, model_version = sqlc.narg(model_version), failure_reason = sqlc.narg(failure_reason),
    finished_at = sqlc.narg(finished_at)
WHERE id = @id
RETURNING *;

-- name: GetRun :one
SELECT * FROM misko.analysis_runs WHERE id = @id;

-- name: LockRun :one
SELECT * FROM misko.analysis_runs WHERE id = @id FOR UPDATE;

-- name: ListRuns :many
SELECT * FROM misko.analysis_runs WHERE test_id = @test_id ORDER BY created_at DESC, id DESC;

-- name: GetCalibration :one
SELECT id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height, measurement_plane, transform
FROM misko.calibrations
WHERE id = @id;

-- name: SourceObject :one
SELECT object_name FROM misko.video_assets WHERE id = @id;

-- name: CreateOutputUpload :exec
INSERT INTO misko.analysis_output_uploads (run_id, attempt, object_name, kind, content_type, size_bytes, crc32c)
VALUES (@run_id, @attempt, @object_name, @kind, @content_type, @size_bytes, @crc32c);

-- name: OutputUploads :many
SELECT run_id, attempt, object_name, kind, content_type, size_bytes, crc32c
FROM misko.analysis_output_uploads
WHERE run_id = @run_id AND attempt = @attempt
ORDER BY object_name;

-- name: CreateAnalyzedAsset :one
INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
VALUES ('ANALYZED', @bucket, @object_name, @content_type, @size_bytes, @crc32c, 'VERIFIED', @generation, @verified_at, @created_by)
RETURNING id;

-- name: CreateArtifact :exec
INSERT INTO misko.analysis_artifacts (run_id, attempt, object_name, kind, bucket, generation, size_bytes, crc32c, content_type, video_asset_id)
VALUES (@run_id, @attempt, @object_name, @kind, @bucket, @generation, @size_bytes, @crc32c, @content_type, sqlc.narg(video_asset_id));

-- name: CreatePair :exec
INSERT INTO misko.analysis_video_pairs (run_id, attempt, source_asset_id, source_generation, analyzed_asset_id, source_offset_us, output_offset_us, time_mapping_version)
VALUES (@run_id, @attempt, @source_asset_id, @source_generation, @analyzed_asset_id, @source_offset_us, @output_offset_us, @time_mapping_version);

-- name: CreateMetric :exec
INSERT INTO misko.metric_results (run_id, attempt, metric_key, unit, value, missing_reason)
VALUES (@run_id, @attempt, @metric_key, @unit, sqlc.narg(value), sqlc.narg(missing_reason));

-- name: CreateEvent :exec
INSERT INTO misko.analysis_events (run_id, attempt, test_id, event_type, kind, start_us, end_us, confidence, trial_id)
VALUES (@run_id, @attempt, @test_id, @event_type, @kind, @start_us, @end_us, @confidence, sqlc.narg(trial_id));

-- name: ListMetrics :many
SELECT metric_key, unit, value, missing_reason FROM misko.metric_results WHERE run_id = @run_id ORDER BY metric_key;

-- name: ListEvents :many
SELECT event_type, kind, start_us, end_us, confidence, trial_id FROM misko.analysis_events WHERE run_id = @run_id ORDER BY start_us, event_type, end_us;

-- name: ListArtifacts :many
SELECT run_id, attempt, object_name, kind, bucket, generation, size_bytes, crc32c, content_type FROM misko.analysis_artifacts WHERE run_id = @run_id ORDER BY object_name;

-- name: GetPair :one
SELECT p.source_asset_id, p.source_generation, s.object_name AS source_object_name, p.analyzed_asset_id, a.object_name AS analyzed_object_name,
       a.generation AS analyzed_generation, p.source_offset_us, p.output_offset_us, p.time_mapping_version
FROM misko.analysis_video_pairs p
JOIN misko.video_assets s ON s.id = p.source_asset_id
JOIN misko.video_assets a ON a.id = p.analyzed_asset_id
WHERE p.run_id = @run_id;

