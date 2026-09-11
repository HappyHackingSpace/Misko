-- name: GetEnrollmentRef :one
SELECT id, experiment_id, subject_id, enrolled_at
FROM misko.enrollments
WHERE experiment_id = @experiment_id AND id = @id;

-- LockEnrollmentRef conflicts with the FOR UPDATE lock taken when assigning groups.
-- name: LockEnrollmentRef :one
SELECT id, experiment_id, subject_id, enrolled_at
FROM misko.enrollments
WHERE experiment_id = @experiment_id AND id = @id
FOR SHARE;

-- name: PhaseExists :one
SELECT EXISTS (SELECT 1 FROM misko.experiment_phases WHERE experiment_id = @experiment_id AND id = @id);

-- name: ProtocolVersionExists :one
SELECT EXISTS (SELECT 1 FROM misko.protocol_versions WHERE experiment_id = @experiment_id AND id = @id);

-- name: GroupAt :one
SELECT group_id
FROM misko.group_assignments
WHERE enrollment_id = @enrollment_id
  AND valid_from <= @at::timestamptz
  AND (valid_to IS NULL OR valid_to > @at::timestamptz);

-- name: GetProtocolStep :one
SELECT s.protocol_version_id, s.position, s.paradigm_key, s.paradigm_version, s.environment_revision_id, s.trials
FROM misko.protocol_steps s
JOIN misko.protocol_versions v ON v.id = s.protocol_version_id
WHERE v.experiment_id = @experiment_id AND v.id = @protocol_version_id AND s.position = @position;

-- name: CreateTest :one
INSERT INTO misko.tests (experiment_id, enrollment_id, subject_id, phase_id, group_id, protocol_version_id, step_position, paradigm_key, paradigm_version, environment_revision_id, planned_trials, scheduled_at, notes, created_by)
VALUES (@experiment_id, @enrollment_id, @subject_id, sqlc.narg(phase_id), sqlc.narg(group_id), @protocol_version_id, @step_position, @paradigm_key, @paradigm_version, @environment_revision_id, @planned_trials, @scheduled_at, sqlc.narg(notes), @created_by)
RETURNING *;

-- name: GetTest :one
SELECT * FROM misko.tests WHERE id = @id;

-- name: LockTest :one
SELECT * FROM misko.tests WHERE id = @id FOR UPDATE;

-- name: UpdateTestStatus :one
UPDATE misko.tests
SET status = @status,
    started_at = sqlc.narg(started_at),
    completed_at = sqlc.narg(completed_at),
    cancelled_at = sqlc.narg(cancelled_at),
    cancel_reason = sqlc.narg(cancel_reason),
    updated_at = now()
WHERE id = @id AND status = @from_status
RETURNING *;

-- name: ListTests :many
SELECT * FROM misko.tests
WHERE experiment_id = @experiment_id
  AND (sqlc.narg(subject_id)::uuid IS NULL OR subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(phase_id)::uuid IS NULL OR phase_id = sqlc.narg(phase_id)::uuid)
  AND (@status::text = '' OR status = @status::text)
  AND (@paradigm_key::text = '' OR paradigm_key = @paradigm_key::text)
ORDER BY scheduled_at, id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountTests :one
SELECT count(*) FROM misko.tests
WHERE experiment_id = @experiment_id
  AND (sqlc.narg(subject_id)::uuid IS NULL OR subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(phase_id)::uuid IS NULL OR phase_id = sqlc.narg(phase_id)::uuid)
  AND (@status::text = '' OR status = @status::text)
  AND (@paradigm_key::text = '' OR paradigm_key = @paradigm_key::text);

-- name: ListTrials :many
SELECT * FROM misko.trials WHERE test_id = @test_id ORDER BY number;

-- name: CreateTrial :one
INSERT INTO misko.trials (test_id, number, repetition, attempt, started_at, ended_at, notes, recorded_by)
VALUES (@test_id, @number, @repetition, @attempt, @started_at, sqlc.narg(ended_at), sqlc.narg(notes), @recorded_by)
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
