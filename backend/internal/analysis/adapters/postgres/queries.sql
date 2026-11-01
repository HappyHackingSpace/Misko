-- The analysis service reads and writes only its own tables: workers, their
-- capabilities and the jobs with their outputs. It never joins the tables of
-- the modules that request jobs.

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

-- CreateJob stores a queued job; a job with the same idempotency key wins and
-- nothing is inserted.
-- name: CreateJob :many
INSERT INTO misko.analysis_jobs (requester_kind, requester_id, idempotency_key, paradigm_key, paradigm_version, metric_engine_version,
    result_schema_version, parameters, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
    calibration_id, calibration_frame_width, calibration_frame_height, calibration_crop_x, calibration_crop_y, calibration_crop_width,
    calibration_crop_height, calibration_plane, calibration_transform, calibration_zones,
    trigger, status, max_attempts, available_at, created_by)
VALUES (@requester_kind, @requester_id, sqlc.narg(idempotency_key), @paradigm_key, @paradigm_version, @metric_engine_version,
    @result_schema_version, @parameters, @source_asset_id, @source_generation, @source_crc32c, @clip_start_us, sqlc.narg(clip_end_us),
    sqlc.narg(calibration_id), sqlc.narg(calibration_frame_width), sqlc.narg(calibration_frame_height), sqlc.narg(calibration_crop_x),
    sqlc.narg(calibration_crop_y), sqlc.narg(calibration_crop_width), sqlc.narg(calibration_crop_height), sqlc.narg(calibration_plane),
    sqlc.narg(calibration_transform)::double precision[], sqlc.narg(calibration_zones),
    @trigger, 'QUEUED', @max_attempts, @available_at, sqlc.narg(created_by))
ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
RETURNING *;

-- name: JobByIdempotencyKey :one
SELECT * FROM misko.analysis_jobs WHERE idempotency_key = @idempotency_key;

-- name: LockExpiredJobs :many
SELECT * FROM misko.analysis_jobs
WHERE status = 'RUNNING' AND lease_expires_at < @now::timestamptz
ORDER BY lease_expires_at, id
LIMIT 100
FOR UPDATE SKIP LOCKED;

-- ClaimNext takes the oldest available job for a capability ("KEY@version"),
-- skipping jobs another worker is claiming.
-- name: ClaimNext :one
SELECT * FROM misko.analysis_jobs j
WHERE j.status = 'QUEUED' AND j.available_at <= @now::timestamptz
  AND (j.paradigm_key || '@' || j.paradigm_version::text) = ANY(@capabilities::text[])
ORDER BY j.available_at, j.id
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: UpdateJob :one
UPDATE misko.analysis_jobs
SET status = @status, attempt = @attempt, worker_id = sqlc.narg(worker_id), lease_expires_at = sqlc.narg(lease_expires_at),
    available_at = @available_at, model_version = sqlc.narg(model_version), failure_reason = sqlc.narg(failure_reason),
    finished_at = sqlc.narg(finished_at)
WHERE id = @id
RETURNING *;

-- name: GetJob :one
SELECT * FROM misko.analysis_jobs WHERE id = @id;

-- name: LockJob :one
SELECT * FROM misko.analysis_jobs WHERE id = @id FOR UPDATE;

-- name: ListJobs :many
SELECT * FROM misko.analysis_jobs WHERE id = ANY(@ids::uuid[]) ORDER BY created_at DESC, id DESC;

-- name: ListJobsByRequester :many
SELECT * FROM misko.analysis_jobs
WHERE requester_kind = @requester_kind AND requester_id = ANY(@requester_ids::uuid[])
ORDER BY created_at DESC, id DESC;

-- name: CreateOutputUpload :exec
INSERT INTO misko.analysis_job_uploads (job_id, attempt, object_name, kind, content_type, size_bytes, crc32c)
VALUES (@job_id, @attempt, @object_name, @kind, @content_type, @size_bytes, @crc32c);

-- name: OutputUploads :many
SELECT job_id, attempt, object_name, kind, content_type, size_bytes, crc32c
FROM misko.analysis_job_uploads
WHERE job_id = @job_id AND attempt = @attempt
ORDER BY object_name;

-- name: CreateOutput :exec
INSERT INTO misko.analysis_job_outputs (job_id, attempt, object_name, kind, bucket, generation, size_bytes, crc32c, content_type)
VALUES (@job_id, @attempt, @object_name, @kind, @bucket, @generation, @size_bytes, @crc32c, @content_type);

-- name: CreatePair :exec
INSERT INTO misko.analysis_job_pairs (job_id, attempt, analyzed_object_name, source_offset_us, output_offset_us, time_mapping_version)
VALUES (@job_id, @attempt, @analyzed_object_name, @source_offset_us, @output_offset_us, @time_mapping_version);

-- name: CreateMetric :exec
INSERT INTO misko.analysis_job_metrics (job_id, attempt, metric_key, unit, value, missing_reason)
VALUES (@job_id, @attempt, @metric_key, @unit, sqlc.narg(value), sqlc.narg(missing_reason));

-- name: CreateEvent :exec
INSERT INTO misko.analysis_job_events (job_id, attempt, event_type, label, kind, start_us, end_us, confidence)
VALUES (@job_id, @attempt, @event_type, sqlc.narg(label), @kind, @start_us, @end_us, @confidence);

-- name: CreateQC :exec
INSERT INTO misko.analysis_job_qc (job_id, attempt, samples, tracked, inferred, lost_ratio, inferred_ratio, mean_confidence, passed)
VALUES (@job_id, @attempt, @samples, @tracked, @inferred, @lost_ratio, @inferred_ratio, @mean_confidence, @passed);

-- name: ListMetrics :many
SELECT job_id, metric_key, unit, value, missing_reason FROM misko.analysis_job_metrics
WHERE job_id = ANY(@job_ids::uuid[])
ORDER BY job_id, metric_key;

-- name: ListEvents :many
SELECT job_id, event_type, label, kind, start_us, end_us, confidence FROM misko.analysis_job_events
WHERE job_id = ANY(@job_ids::uuid[])
ORDER BY job_id, start_us, event_type, label, end_us;

-- name: ListOutputs :many
SELECT job_id, attempt, object_name, kind, bucket, generation, size_bytes, crc32c, content_type FROM misko.analysis_job_outputs
WHERE job_id = ANY(@job_ids::uuid[])
ORDER BY job_id, object_name;

-- name: ListPairs :many
SELECT p.job_id, p.analyzed_object_name, o.generation AS analyzed_generation, p.source_offset_us, p.output_offset_us, p.time_mapping_version
FROM misko.analysis_job_pairs p
JOIN misko.analysis_job_outputs o ON o.job_id = p.job_id AND o.object_name = p.analyzed_object_name
WHERE p.job_id = ANY(@job_ids::uuid[]);

-- name: ListQC :many
SELECT job_id, samples, tracked, inferred, lost_ratio, inferred_ratio, mean_confidence, passed FROM misko.analysis_job_qc
WHERE job_id = ANY(@job_ids::uuid[]);
