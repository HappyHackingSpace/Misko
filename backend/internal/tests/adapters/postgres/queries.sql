-- name: GetExperimentStatus :one
SELECT status FROM misko.experiments WHERE id = @id;

-- GetTimepoint computes the default schedule: day 0 plus the study day, at
-- midnight in the laboratory's time zone. It finds nothing before day 0 is set.
-- name: GetTimepoint :one
SELECT tp.id, tp.study_day,
       ((e.day_zero + tp.study_day)::timestamp AT TIME ZONE coalesce((SELECT l.timezone FROM misko.laboratory l), 'UTC'))::timestamptz AS default_scheduled_at
FROM misko.timepoints tp
JOIN misko.experiments e ON e.id = tp.experiment_id
WHERE tp.experiment_id = @experiment_id AND tp.id = @id AND e.day_zero IS NOT NULL;

-- name: ProtocolVersionExists :one
SELECT EXISTS (SELECT 1 FROM misko.protocol_versions WHERE experiment_id = @experiment_id AND id = @id);

-- name: GetProtocolStep :one
SELECT s.protocol_version_id, s.position, s.paradigm_key, s.paradigm_version, s.trials
FROM misko.protocol_steps s
JOIN misko.protocol_versions v ON v.id = s.protocol_version_id
WHERE v.experiment_id = @experiment_id AND v.id = @protocol_version_id AND s.position = @position;

-- LockAllocations takes a lock that waits for a concurrent plan of the same
-- animal and for exclusions, but not for foreign key checks of other rows.
-- name: LockAllocations :many
SELECT a.id, a.subject_id, (a.excluded_at IS NOT NULL)::boolean AS excluded
FROM misko.allocations a
WHERE a.experiment_id = @experiment_id AND a.id = ANY(@ids::uuid[])
ORDER BY a.id
FOR NO KEY UPDATE;

-- name: ListArenas :many
SELECT id, environment_id, paradigm_key, paradigm_version
FROM misko.environment_revisions
WHERE id = ANY(@ids::uuid[]);

-- name: ScheduledAllocations :many
SELECT DISTINCT ts.allocation_id
FROM misko.test_subjects ts
JOIN misko.tests t ON t.id = ts.test_id
WHERE t.timepoint_id = @timepoint_id
  AND t.protocol_version_id = @protocol_version_id
  AND t.step_position = @step_position
  AND t.status <> 'CANCELLED'
  AND ts.allocation_id = ANY(@ids::uuid[])
  AND (sqlc.narg(except_test_id)::uuid IS NULL OR t.id <> sqlc.narg(except_test_id)::uuid)
ORDER BY ts.allocation_id;

-- name: CreateTest :one
INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, notes, created_by)
VALUES (@experiment_id, @timepoint_id, @protocol_version_id, @step_position, @paradigm_key, @paradigm_version, @planned_trials, @scheduled_at, sqlc.narg(notes), @created_by)
RETURNING id;

-- name: CreateTestSubject :exec
INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
VALUES (@experiment_id, @test_id, @allocation_id, @environment_revision_id, @paradigm_key, @paradigm_version, @created_by);

-- name: DeleteTestSubject :execrows
DELETE FROM misko.test_subjects WHERE test_id = @test_id AND id = @id;

-- name: GetTest :one
SELECT t.id, t.experiment_id, e.code AS experiment_code, t.timepoint_id, tp.name AS timepoint_name, tp.study_day,
       p.id AS protocol_id, p.name AS protocol_name, t.protocol_version_id, pv.number AS protocol_version, t.step_position,
       t.paradigm_key, t.paradigm_version, t.planned_trials, t.status, t.scheduled_at, t.started_at, t.completed_at, t.cancelled_at,
       t.cancel_reason, t.notes, t.created_by, t.created_at, t.updated_at
FROM misko.tests t
JOIN misko.experiments e ON e.id = t.experiment_id
JOIN misko.timepoints tp ON tp.id = t.timepoint_id
JOIN misko.protocol_versions pv ON pv.id = t.protocol_version_id
JOIN misko.protocols p ON p.id = pv.protocol_id
WHERE t.id = @id;

-- name: LockTest :one
SELECT t.id, t.experiment_id, e.code AS experiment_code, t.timepoint_id, tp.name AS timepoint_name, tp.study_day,
       p.id AS protocol_id, p.name AS protocol_name, t.protocol_version_id, pv.number AS protocol_version, t.step_position,
       t.paradigm_key, t.paradigm_version, t.planned_trials, t.status, t.scheduled_at, t.started_at, t.completed_at, t.cancelled_at,
       t.cancel_reason, t.notes, t.created_by, t.created_at, t.updated_at
FROM misko.tests t
JOIN misko.experiments e ON e.id = t.experiment_id
JOIN misko.timepoints tp ON tp.id = t.timepoint_id
JOIN misko.protocol_versions pv ON pv.id = t.protocol_version_id
JOIN misko.protocols p ON p.id = pv.protocol_id
WHERE t.id = @id
FOR UPDATE OF t;

-- name: UpdateTestStatus :execrows
UPDATE misko.tests
SET status = @status,
    started_at = sqlc.narg(started_at),
    completed_at = sqlc.narg(completed_at),
    cancelled_at = sqlc.narg(cancelled_at),
    cancel_reason = sqlc.narg(cancel_reason)
WHERE id = @id AND status = @from_status;

-- ListTests filters on the scheduled date in the laboratory's time zone; from
-- and to are inclusive dates.
-- name: ListTests :many
WITH lab AS (SELECT coalesce((SELECT l.timezone FROM misko.laboratory l), 'UTC') AS timezone)
SELECT t.id, t.experiment_id, e.code AS experiment_code, t.timepoint_id, tp.name AS timepoint_name, tp.study_day,
       p.id AS protocol_id, p.name AS protocol_name, t.protocol_version_id, pv.number AS protocol_version, t.step_position,
       t.paradigm_key, t.paradigm_version, t.planned_trials, t.status, t.scheduled_at, t.started_at, t.completed_at, t.cancelled_at,
       t.cancel_reason, t.notes, t.created_by, t.created_at, t.updated_at
FROM misko.tests t
JOIN misko.experiments e ON e.id = t.experiment_id
JOIN misko.timepoints tp ON tp.id = t.timepoint_id
JOIN misko.protocol_versions pv ON pv.id = t.protocol_version_id
JOIN misko.protocols p ON p.id = pv.protocol_id
CROSS JOIN lab
WHERE (sqlc.narg(experiment_id)::uuid IS NULL OR t.experiment_id = sqlc.narg(experiment_id)::uuid)
  AND (sqlc.narg(timepoint_id)::uuid IS NULL OR t.timepoint_id = sqlc.narg(timepoint_id)::uuid)
  AND (sqlc.narg(subject_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM misko.test_subjects ts JOIN misko.allocations a ON a.id = ts.allocation_id
        WHERE ts.test_id = t.id AND a.subject_id = sqlc.narg(subject_id)::uuid))
  AND (cardinality(@statuses::text[]) = 0 OR t.status = ANY(@statuses::text[]))
  AND (@paradigm_key::text = '' OR t.paradigm_key = @paradigm_key::text)
  AND (@search::text = '' OR e.code ILIKE '%' || @search::text || '%' ESCAPE '\' OR tp.name ILIKE '%' || @search::text || '%' ESCAPE '\' OR EXISTS (
        SELECT 1 FROM misko.test_subjects ts JOIN misko.allocations a ON a.id = ts.allocation_id JOIN misko.subjects s ON s.id = a.subject_id
        WHERE ts.test_id = t.id AND s.code ILIKE '%' || @search::text || '%' ESCAPE '\'))
  AND (sqlc.narg(from_date)::date IS NULL OR t.scheduled_at >= (sqlc.narg(from_date)::date)::timestamp AT TIME ZONE lab.timezone)
  AND (sqlc.narg(to_date)::date IS NULL OR t.scheduled_at < (sqlc.narg(to_date)::date + 1)::timestamp AT TIME ZONE lab.timezone)
ORDER BY CASE WHEN @newest_first::boolean THEN t.scheduled_at END DESC,
         CASE WHEN NOT @newest_first::boolean THEN t.scheduled_at END,
         t.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountTests :one
WITH lab AS (SELECT coalesce((SELECT l.timezone FROM misko.laboratory l), 'UTC') AS timezone)
SELECT count(*)
FROM misko.tests t
JOIN misko.experiments e ON e.id = t.experiment_id
JOIN misko.timepoints tp ON tp.id = t.timepoint_id
CROSS JOIN lab
WHERE (sqlc.narg(experiment_id)::uuid IS NULL OR t.experiment_id = sqlc.narg(experiment_id)::uuid)
  AND (sqlc.narg(timepoint_id)::uuid IS NULL OR t.timepoint_id = sqlc.narg(timepoint_id)::uuid)
  AND (sqlc.narg(subject_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM misko.test_subjects ts JOIN misko.allocations a ON a.id = ts.allocation_id
        WHERE ts.test_id = t.id AND a.subject_id = sqlc.narg(subject_id)::uuid))
  AND (cardinality(@statuses::text[]) = 0 OR t.status = ANY(@statuses::text[]))
  AND (@paradigm_key::text = '' OR t.paradigm_key = @paradigm_key::text)
  AND (@search::text = '' OR e.code ILIKE '%' || @search::text || '%' ESCAPE '\' OR tp.name ILIKE '%' || @search::text || '%' ESCAPE '\' OR EXISTS (
        SELECT 1 FROM misko.test_subjects ts JOIN misko.allocations a ON a.id = ts.allocation_id JOIN misko.subjects s ON s.id = a.subject_id
        WHERE ts.test_id = t.id AND s.code ILIKE '%' || @search::text || '%' ESCAPE '\'))
  AND (sqlc.narg(from_date)::date IS NULL OR t.scheduled_at >= (sqlc.narg(from_date)::date)::timestamp AT TIME ZONE lab.timezone)
  AND (sqlc.narg(to_date)::date IS NULL OR t.scheduled_at < (sqlc.narg(to_date)::date + 1)::timestamp AT TIME ZONE lab.timezone);

-- name: ListTestSubjects :many
SELECT ts.id, ts.test_id, ts.allocation_id, a.subject_id, s.code AS subject_code, (a.excluded_at IS NOT NULL)::boolean AS excluded,
       ts.environment_revision_id, r.environment_id, env.name AS environment_name, r.number AS environment_revision,
       ts.created_by, ts.created_at
FROM misko.test_subjects ts
JOIN misko.allocations a ON a.id = ts.allocation_id
JOIN misko.subjects s ON s.id = a.subject_id
JOIN misko.environment_revisions r ON r.id = ts.environment_revision_id
JOIN misko.environments env ON env.id = r.environment_id
WHERE ts.test_id = ANY(@test_ids::uuid[])
ORDER BY ts.test_id, env.name, ts.id;

-- name: ListTrials :many
SELECT * FROM misko.trials WHERE test_id = @test_id ORDER BY test_subject_id, number;

-- name: CreateTrial :one
INSERT INTO misko.trials (test_id, test_subject_id, number, repetition, attempt, started_at, ended_at, notes, recorded_by)
VALUES (@test_id, @test_subject_id, @number, @repetition, @attempt, @started_at, sqlc.narg(ended_at), sqlc.narg(notes), @recorded_by)
RETURNING *;

-- name: ListComments :many
SELECT c.id, c.test_id, c.author_id, coalesce(u.name, '')::text AS author_name, c.body, c.created_at, c.updated_at
FROM misko.test_comments c
LEFT JOIN misko.users u ON u.id = c.author_id
WHERE c.test_id = @test_id
ORDER BY c.created_at, c.id;

-- name: GetComment :one
SELECT c.id, c.test_id, c.author_id, coalesce(u.name, '')::text AS author_name, c.body, c.created_at, c.updated_at
FROM misko.test_comments c
LEFT JOIN misko.users u ON u.id = c.author_id
WHERE c.test_id = @test_id AND c.id = @id;

-- name: CreateComment :one
INSERT INTO misko.test_comments (test_id, author_id, body)
VALUES (@test_id, @author_id, @body)
RETURNING id;

-- name: UpdateComment :execrows
UPDATE misko.test_comments SET body = @body, updated_at = now()
WHERE test_id = @test_id AND id = @id;

-- name: DeleteComment :execrows
DELETE FROM misko.test_comments WHERE test_id = @test_id AND id = @id;
