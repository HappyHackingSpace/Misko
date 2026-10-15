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

-- ReadyTestSubjects lists the animals the scheduler can analyse now: the
-- test's current video is verified, the test is not cancelled, the experiment
-- runs or completed, the animal is not excluded, an enabled worker supports the
-- paradigm version (capabilities, "KEY@version") and, when the paradigm needs
-- a calibration (calibrated), the animal's arena has a valid effective one.
-- Only candidates without an automatic run for the same video generation and
-- calibration are returned, so the oldest rows never starve newer ones. An
-- arena (environment) calibration never triggers a run for an animal that
-- already has an automatic run of this video: correcting the default must not
-- silently reanalyse every animal, that stays an explicit manual run.
-- name: ReadyTestSubjects :many
WITH candidates AS (
    SELECT ts.experiment_id, ts.test_id, ts.id AS test_subject_id, r.id AS recording_id, a.id AS source_asset_id,
           a.generation AS source_generation, a.crc32c AS source_crc32c, r.clip_start_us, r.clip_end_us,
           t.paradigm_key, t.paradigm_version, er.parameters, ps.session, r.created_at,
           coalesce(ec.calibration_id::text, '')::text AS calibration_id, coalesce(ec.source, '')::text AS calibration_source
    FROM misko.test_subjects ts
    JOIN misko.effective_calibrations ec ON ec.test_subject_id = ts.id
    JOIN misko.test_recordings r ON r.id = ec.recording_id
    JOIN misko.video_assets a ON a.id = r.video_asset_id
    JOIN misko.tests t ON t.id = ts.test_id
    JOIN misko.experiments e ON e.id = ts.experiment_id
    JOIN misko.allocations al ON al.id = ts.allocation_id
    JOIN misko.environment_revisions er ON er.id = ts.environment_revision_id
    JOIN misko.protocol_steps ps ON ps.protocol_version_id = t.protocol_version_id AND ps.position = t.step_position
    WHERE a.status = 'VERIFIED' AND t.status <> 'CANCELLED' AND e.status IN ('RUNNING', 'COMPLETED') AND al.excluded_at IS NULL
      AND (t.paradigm_key || '@' || t.paradigm_version::text) = ANY(@capabilities::text[])
      AND (ec.calibration_id IS NOT NULL OR NOT (t.paradigm_key || '@' || t.paradigm_version::text) = ANY(@calibrated::text[]))
)
SELECT c.experiment_id, c.test_id, c.test_subject_id, c.recording_id, c.source_asset_id, c.source_generation::bigint AS source_generation,
       c.source_crc32c, c.clip_start_us, c.clip_end_us, c.paradigm_key, c.paradigm_version, c.parameters, c.session, c.calibration_id
FROM candidates c
WHERE NOT EXISTS (
    SELECT 1 FROM misko.analysis_runs x
    WHERE x.trigger = 'AUTOMATIC' AND x.test_subject_id = c.test_subject_id AND x.recording_id = c.recording_id
      AND x.source_generation = c.source_generation AND x.paradigm_key = c.paradigm_key AND x.paradigm_version = c.paradigm_version
      AND (c.calibration_source = 'ENVIRONMENT' OR coalesce(x.calibration_id::text, '') IN (c.calibration_id, '')))
ORDER BY c.created_at, c.test_subject_id
LIMIT 500;

-- GetCandidate reads one animal of a test with its current video, if any, and
-- everything that decides whether it can be analysed.
-- name: GetCandidate :one
SELECT ts.experiment_id, ts.test_id, ts.id AS test_subject_id, coalesce(r.id::text, '')::text AS recording_id,
       coalesce(a.id::text, '')::text AS source_asset_id, coalesce(a.generation, 0)::bigint AS source_generation,
       coalesce(a.crc32c, 0)::bigint AS source_crc32c, coalesce(r.clip_start_us, 0)::bigint AS clip_start_us, r.clip_end_us,
       t.paradigm_key, t.paradigm_version, er.parameters, ps.session, coalesce(a.status, '')::text AS video_status,
       t.status AS test_status, e.status AS experiment_status, (al.excluded_at IS NOT NULL)::boolean AS excluded,
       coalesce(ec.calibration_id::text, '')::text AS calibration_id
FROM misko.test_subjects ts
JOIN misko.tests t ON t.id = ts.test_id
JOIN misko.experiments e ON e.id = ts.experiment_id
JOIN misko.allocations al ON al.id = ts.allocation_id
JOIN misko.environment_revisions er ON er.id = ts.environment_revision_id
JOIN misko.protocol_steps ps ON ps.protocol_version_id = t.protocol_version_id AND ps.position = t.step_position
LEFT JOIN misko.effective_calibrations ec ON ec.test_subject_id = ts.id
LEFT JOIN misko.test_recordings r ON r.id = ec.recording_id
LEFT JOIN misko.video_assets a ON a.id = r.video_asset_id
WHERE ts.id = @id;

-- name: CreateAutomaticRun :execrows
INSERT INTO misko.analysis_runs (experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c,
    clip_start_us, clip_end_us, calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version,
    parameters, trigger, status, max_attempts, available_at, created_by)
VALUES (@experiment_id, @test_id, @test_subject_id, @recording_id, @source_asset_id, @source_generation, @source_crc32c,
    @clip_start_us, sqlc.narg(clip_end_us), sqlc.narg(calibration_id), @paradigm_key, @paradigm_version, @metric_engine_version,
    @result_schema_version, @parameters, 'AUTOMATIC', 'QUEUED', @max_attempts, @available_at, NULL)
ON CONFLICT DO NOTHING;

-- name: CreateManualRun :one
INSERT INTO misko.analysis_runs (experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c,
    clip_start_us, clip_end_us, calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version,
    parameters, trigger, status, max_attempts, available_at, created_by)
VALUES (@experiment_id, @test_id, @test_subject_id, @recording_id, @source_asset_id, @source_generation, @source_crc32c,
    @clip_start_us, sqlc.narg(clip_end_us), sqlc.narg(calibration_id), @paradigm_key, @paradigm_version, @metric_engine_version,
    @result_schema_version, @parameters, 'MANUAL', 'QUEUED', @max_attempts, @available_at, @created_by)
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
SELECT * FROM misko.analysis_runs
WHERE test_id = @test_id AND (sqlc.narg(test_subject_id)::uuid IS NULL OR test_subject_id = sqlc.narg(test_subject_id)::uuid)
ORDER BY created_at DESC, id DESC;

-- name: ListSubjectRuns :many
SELECT * FROM misko.analysis_runs WHERE test_subject_id = @test_subject_id ORDER BY created_at DESC, id DESC;

-- name: TestSubjectArena :one
SELECT environment_revision_id FROM misko.test_subjects WHERE id = @id;

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
INSERT INTO misko.analysis_events (run_id, attempt, test_subject_id, event_type, kind, start_us, end_us, confidence, trial_id)
VALUES (@run_id, @attempt, @test_subject_id, @event_type, @kind, @start_us, @end_us, @confidence, sqlc.narg(trial_id));

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

-- LockTestSubjectRuns serializes manual run creation for one animal of a
-- test until the transaction ends, so two requests cannot both see no
-- pending run.
-- name: LockTestSubjectRuns :exec
SELECT pg_advisory_xact_lock(hashtextextended(@test_subject_id::text, 0));

-- name: PendingRunExists :one
SELECT EXISTS (SELECT 1 FROM misko.analysis_runs WHERE test_subject_id = @test_subject_id AND status IN ('QUEUED', 'RUNNING'))::boolean;

-- OpenResult creates the PENDING result of an animal on its first successful
-- run; later runs leave the reviewed choice alone.
-- name: OpenResult :exec
INSERT INTO misko.subject_results (test_subject_id, run_id)
VALUES (@test_subject_id, @run_id)
ON CONFLICT (test_subject_id) DO NOTHING;

-- LockResult locks a result for review and its experiment's status against a
-- concurrent completion.
-- name: LockResult :one
SELECT sr.test_subject_id, sr.run_id, sr.status, sr.reason, sr.note, sr.reviewed_by, sr.reviewed_at, sr.created_at, sr.updated_at,
       misko.experiment_status(ts.experiment_id)::text AS experiment_status
FROM misko.subject_results sr
JOIN misko.test_subjects ts ON ts.id = sr.test_subject_id
WHERE sr.test_subject_id = @test_subject_id
FOR UPDATE OF sr;

-- name: UpdateResult :one
UPDATE misko.subject_results
SET run_id = @run_id, status = @status, reason = sqlc.narg(reason), note = sqlc.narg(note),
    reviewed_by = sqlc.narg(reviewed_by), reviewed_at = sqlc.narg(reviewed_at)
WHERE test_subject_id = @test_subject_id
RETURNING *;

-- ReviewItems lists results with what a reviewer sees: the animal, its arena,
-- the test and the run under review. Group, disease and treatment columns are
-- read here and blanked by the application for blinded experiments.
-- name: ReviewItems :many
SELECT sr.test_subject_id, sr.run_id, sr.status, sr.reason, sr.note, sr.reviewed_by, sr.reviewed_at, sr.created_at, sr.updated_at,
       ts.test_id, ts.experiment_id, e.code AS experiment_code, e.blinded, e.status AS experiment_status,
       s.code AS subject_code, (al.excluded_at IS NOT NULL)::boolean AS excluded,
       ts.environment_revision_id, env.id AS environment_id, env.name AS environment_name, er.number AS environment_revision,
       tp.id AS timepoint_id, tp.name AS timepoint_name, t.paradigm_key, t.paradigm_version, t.scheduled_at,
       g.id AS group_id, g.code AS group_code, g.name AS group_name, g.role AS group_role, dm.name AS disease_model_name,
       sub.name AS treatment_substance_name, tr.dose_micro AS treatment_dose_micro, tr.unit AS treatment_unit,
       r.trigger, r.model_version, r.metric_engine_version, r.finished_at, r.calibration_id, r.recording_id,
       latest.id AS latest_run_id
FROM misko.subject_results sr
JOIN misko.test_subjects ts ON ts.id = sr.test_subject_id
JOIN misko.tests t ON t.id = ts.test_id
JOIN misko.experiments e ON e.id = ts.experiment_id
JOIN misko.allocations al ON al.id = ts.allocation_id
JOIN misko.subjects s ON s.id = al.subject_id
JOIN misko.experiment_groups g ON g.id = al.group_id
LEFT JOIN misko.disease_models dm ON dm.id = g.disease_model_id
LEFT JOIN misko.regimen_items tr ON tr.group_id = g.id AND tr.purpose = 'TREATMENT'
LEFT JOIN misko.substances sub ON sub.id = tr.substance_id
JOIN misko.timepoints tp ON tp.id = t.timepoint_id
JOIN misko.environment_revisions er ON er.id = ts.environment_revision_id
JOIN misko.environments env ON env.id = er.environment_id
JOIN misko.analysis_runs r ON r.id = sr.run_id
JOIN LATERAL (
    SELECT x.id FROM misko.analysis_runs x
    WHERE x.test_subject_id = sr.test_subject_id AND x.status = 'SUCCEEDED'
    ORDER BY x.finished_at DESC, x.id DESC
    LIMIT 1
) latest ON true
WHERE (sqlc.narg(experiment_id)::uuid IS NULL OR ts.experiment_id = sqlc.narg(experiment_id)::uuid)
  AND (sqlc.narg(test_subject_id)::uuid IS NULL OR sr.test_subject_id = sqlc.narg(test_subject_id)::uuid)
  AND (@status::text = '' OR sr.status = @status::text)
ORDER BY sr.created_at, sr.test_subject_id
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountReviewItems :one
SELECT count(*)
FROM misko.subject_results sr
JOIN misko.test_subjects ts ON ts.id = sr.test_subject_id
WHERE (sqlc.narg(experiment_id)::uuid IS NULL OR ts.experiment_id = sqlc.narg(experiment_id)::uuid)
  AND (@status::text = '' OR sr.status = @status::text);

-- name: RunMetrics :many
SELECT run_id, metric_key, unit, value, missing_reason
FROM misko.metric_results
WHERE run_id = ANY(@run_ids::uuid[])
ORDER BY run_id, metric_key;
