-- LockSubject serializes scoring of one animal of a test until the
-- transaction ends, so two requests cannot both start a sheet or both score a
-- single event.
-- name: LockSubject :exec
SELECT pg_advisory_xact_lock(hashtextextended('scoring:' || @test_subject_id::text, 0));

-- GetSubject reads one animal of a test with the test's current verified
-- video, if any, and everything that decides whether it can be scored.
-- name: GetSubject :one
SELECT ts.experiment_id, ts.test_id, ts.id AS test_subject_id, e.status AS experiment_status, t.status AS test_status,
       (al.excluded_at IS NOT NULL)::boolean AS excluded, t.paradigm_key, t.paradigm_version, er.parameters, ps.session,
       coalesce(r.id::text, '')::text AS recording_id, coalesce(a.id::text, '')::text AS asset_id,
       coalesce(a.generation, 0)::bigint AS generation, coalesce(a.crc32c, 0)::bigint AS crc32c,
       coalesce(r.clip_start_us, 0)::bigint AS clip_start_us, r.clip_end_us
FROM misko.test_subjects ts
JOIN misko.tests t ON t.id = ts.test_id
JOIN misko.experiments e ON e.id = ts.experiment_id
JOIN misko.allocations al ON al.id = ts.allocation_id
JOIN misko.environment_revisions er ON er.id = ts.environment_revision_id
JOIN misko.protocol_steps ps ON ps.protocol_version_id = t.protocol_version_id AND ps.position = t.step_position
LEFT JOIN misko.current_recordings r ON r.test_id = t.id
LEFT JOIN misko.video_assets a ON a.id = r.video_asset_id
WHERE ts.id = @test_subject_id AND ts.test_id = @test_id;

-- name: LatestSheet :one
SELECT * FROM misko.scoring_sheets
WHERE test_subject_id = @test_subject_id
ORDER BY version DESC
LIMIT 1;

-- name: CreateSheet :one
INSERT INTO misko.scoring_sheets (experiment_id, test_id, test_subject_id, version, created_by, created_at)
VALUES (@experiment_id, @test_id, @test_subject_id, @version, @created_by, now())
RETURNING *;

-- name: CompleteSheet :one
UPDATE misko.scoring_sheets
SET status = 'COMPLETED', scored_types = @scored_types::text[], duration_us = sqlc.narg(duration_us), recording_id = @recording_id,
    run_id = @run_id, completed_by = @completed_by, completed_at = @completed_at
WHERE id = @id AND status = 'DRAFT'
RETURNING *;

-- name: ListObservations :many
SELECT * FROM misko.scored_observations
WHERE sheet_id = @sheet_id
ORDER BY start_us, end_us, id;

-- name: CreateObservation :one
INSERT INTO misko.scored_observations (sheet_id, test_subject_id, event_type, kind, label, start_us, end_us, scored_by)
VALUES (@sheet_id, @test_subject_id, @event_type, @kind, sqlc.narg(label), @start_us, @end_us, @scored_by)
RETURNING *;

-- name: UpdateObservation :one
UPDATE misko.scored_observations
SET event_type = @event_type, kind = @kind, label = sqlc.narg(label), start_us = @start_us, end_us = @end_us, scored_by = @scored_by
WHERE id = @id AND sheet_id = @sheet_id
RETURNING *;

-- name: DeleteObservation :execrows
DELETE FROM misko.scored_observations WHERE id = @id AND sheet_id = @sheet_id;

-- CopyObservations carries the events of a completed sheet to the next
-- version, keeping who scored each one.
-- name: CopyObservations :exec
INSERT INTO misko.scored_observations (sheet_id, test_subject_id, event_type, kind, label, start_us, end_us, scored_by)
SELECT @to_sheet_id, o.test_subject_id, o.event_type, o.kind, o.label, o.start_us, o.end_us, o.scored_by
FROM misko.scored_observations o
WHERE o.sheet_id = @from_sheet_id
ORDER BY o.start_us, o.end_us, o.id;

-- CreateScoredRun starts a SCORED run on the animal's current video. It is
-- published with FinishScoredRun in the same transaction.
-- name: CreateScoredRun :one
INSERT INTO misko.analysis_runs (experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c,
    clip_start_us, clip_end_us, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, parameters, trigger, status,
    attempt, max_attempts, available_at, created_by, method, scored_by)
VALUES (@experiment_id, @test_id, @test_subject_id, @recording_id, @source_asset_id, @source_generation, @source_crc32c,
    @clip_start_us, sqlc.narg(clip_end_us), @paradigm_key, @paradigm_version, @metric_engine_version, @result_schema_version, @parameters,
    'MANUAL', 'RUNNING', 1, 1, @available_at, @scored_by, 'SCORED', @scored_by)
RETURNING id;

-- name: CreateMetric :exec
INSERT INTO misko.metric_results (run_id, attempt, metric_key, unit, value, missing_reason)
VALUES (@run_id, 1, @metric_key, @unit, sqlc.narg(value), sqlc.narg(missing_reason));

-- name: CreateEvent :exec
INSERT INTO misko.analysis_events (run_id, attempt, test_subject_id, event_type, kind, start_us, end_us, confidence)
VALUES (@run_id, 1, @test_subject_id, @event_type, @kind, @start_us, @end_us, 1);

-- name: FinishScoredRun :execrows
UPDATE misko.analysis_runs
SET status = 'SUCCEEDED', finished_at = @finished_at
WHERE id = @id AND method = 'SCORED' AND status = 'RUNNING';

-- OpenResult creates the PENDING result of an animal on its first published
-- run; later runs leave the reviewed choice alone.
-- name: OpenResult :exec
INSERT INTO misko.subject_results (test_subject_id, run_id)
VALUES (@test_subject_id, @run_id)
ON CONFLICT (test_subject_id) DO NOTHING;
