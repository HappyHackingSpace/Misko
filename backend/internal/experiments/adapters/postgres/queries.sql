-- name: CreateExperiment :one
INSERT INTO misko.experiments (code, title, description, requires_control)
VALUES (@code, @title, sqlc.narg(description), @requires_control)
RETURNING id, code, title, description, requires_control, created_at, updated_at;

-- name: GetExperiment :one
SELECT id, code, title, description, requires_control, created_at, updated_at
FROM misko.experiments
WHERE id = @id;

-- name: ListExperiments :many
SELECT id, code, title, description, requires_control, created_at, updated_at
FROM misko.experiments
WHERE (@search::text = '' OR code ILIKE '%' || @search::text || '%' ESCAPE '\' OR title ILIKE '%' || @search::text || '%' ESCAPE '\')
ORDER BY
    CASE WHEN @sort_key::text = 'code' AND NOT @descending::boolean THEN lower(code) END ASC,
    CASE WHEN @sort_key::text = 'code' AND @descending::boolean THEN lower(code) END DESC,
    CASE WHEN @sort_key::text = 'title' AND NOT @descending::boolean THEN lower(title) END ASC,
    CASE WHEN @sort_key::text = 'title' AND @descending::boolean THEN lower(title) END DESC,
    CASE WHEN @sort_key::text = 'createdAt' AND NOT @descending::boolean THEN created_at END ASC,
    CASE WHEN @sort_key::text = 'createdAt' AND @descending::boolean THEN created_at END DESC,
    id ASC
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountExperiments :one
SELECT count(*)
FROM misko.experiments
WHERE (@search::text = '' OR code ILIKE '%' || @search::text || '%' ESCAPE '\' OR title ILIKE '%' || @search::text || '%' ESCAPE '\');

-- A NULL argument leaves a column unchanged; an empty description clears it.
-- name: UpdateExperiment :one
UPDATE misko.experiments
SET code = coalesce(sqlc.narg(code), code),
    title = coalesce(sqlc.narg(title), title),
    description = CASE WHEN sqlc.narg(description)::text IS NULL THEN description ELSE NULLIF(sqlc.narg(description)::text, '') END,
    requires_control = coalesce(sqlc.narg(requires_control), requires_control),
    updated_at = now()
WHERE id = @id
RETURNING id, code, title, description, requires_control, created_at, updated_at;

-- name: ListPhases :many
SELECT id, experiment_id, name, position, description, created_at, updated_at
FROM misko.experiment_phases
WHERE experiment_id = @experiment_id
ORDER BY position, id;

-- name: CreatePhase :one
INSERT INTO misko.experiment_phases (experiment_id, name, position, description)
VALUES (@experiment_id, @name, @position, sqlc.narg(description))
RETURNING id, experiment_id, name, position, description, created_at, updated_at;

-- name: UpdatePhase :one
UPDATE misko.experiment_phases
SET name = coalesce(sqlc.narg(name), name),
    position = coalesce(sqlc.narg(position), position),
    description = CASE WHEN sqlc.narg(description)::text IS NULL THEN description ELSE NULLIF(sqlc.narg(description)::text, '') END,
    updated_at = now()
WHERE experiment_id = @experiment_id AND id = @id
RETURNING id, experiment_id, name, position, description, created_at, updated_at;

-- name: DeletePhase :execrows
DELETE FROM misko.experiment_phases
WHERE experiment_id = @experiment_id AND id = @id;

-- Active counts use the database clock so every caller sees the same instant.
-- name: ListGroupSizes :many
SELECT g.id, g.experiment_id, g.name, g.role, g.target_size, g.description, g.created_at, g.updated_at,
       (count(a.id) FILTER (WHERE a.valid_from <= now() AND (a.valid_to IS NULL OR a.valid_to > now())))::integer AS active_subjects
FROM misko.experiment_groups g
LEFT JOIN misko.group_assignments a ON a.group_id = g.id
WHERE g.experiment_id = @experiment_id
GROUP BY g.id
ORDER BY lower(g.name), g.id;

-- name: GetGroup :one
SELECT id, experiment_id, name, role, target_size, description, created_at, updated_at
FROM misko.experiment_groups
WHERE experiment_id = @experiment_id AND id = @id;

-- name: CreateGroup :one
INSERT INTO misko.experiment_groups (experiment_id, name, role, target_size, description)
VALUES (@experiment_id, @name, @role, sqlc.narg(target_size), sqlc.narg(description))
RETURNING id, experiment_id, name, role, target_size, description, created_at, updated_at;

-- A zero target size clears the target.
-- name: UpdateGroup :one
UPDATE misko.experiment_groups
SET name = coalesce(sqlc.narg(name), name),
    role = coalesce(sqlc.narg(role), role),
    target_size = CASE WHEN sqlc.narg(target_size)::integer IS NULL THEN target_size ELSE NULLIF(sqlc.narg(target_size)::integer, 0) END,
    description = CASE WHEN sqlc.narg(description)::text IS NULL THEN description ELSE NULLIF(sqlc.narg(description)::text, '') END,
    updated_at = now()
WHERE experiment_id = @experiment_id AND id = @id
RETURNING id, experiment_id, name, role, target_size, description, created_at, updated_at;

-- name: DeleteGroup :execrows
DELETE FROM misko.experiment_groups
WHERE experiment_id = @experiment_id AND id = @id;

-- name: CreateEnrollment :one
INSERT INTO misko.enrollments (experiment_id, subject_id, enrolled_at)
VALUES (@experiment_id, @subject_id, @enrolled_at::timestamptz)
RETURNING id, experiment_id, subject_id, enrolled_at, created_at;

-- name: GetEnrollment :one
SELECT id, experiment_id, subject_id, enrolled_at, created_at
FROM misko.enrollments
WHERE experiment_id = @experiment_id AND id = @id;

-- Serializes group changes of one enrollment.
-- name: LockEnrollment :one
SELECT id, experiment_id, subject_id, enrolled_at, created_at
FROM misko.enrollments
WHERE experiment_id = @experiment_id AND id = @id
FOR UPDATE;

-- current_group_id is the group active at the database clock, or empty when none.
-- name: ListEnrollments :many
SELECT e.id, e.experiment_id, e.subject_id, e.enrolled_at, e.created_at, coalesce(a.group_id::text, '')::text AS current_group_id
FROM misko.enrollments e
LEFT JOIN misko.group_assignments a
    ON a.enrollment_id = e.id AND a.valid_from <= now() AND (a.valid_to IS NULL OR a.valid_to > now())
WHERE e.experiment_id = @experiment_id
  AND (@subject_id::text = '' OR e.subject_id::text = @subject_id::text)
  AND (@group_id::text = '' OR a.group_id::text = @group_id::text)
ORDER BY
    CASE WHEN @descending::boolean THEN e.enrolled_at END DESC,
    CASE WHEN NOT @descending::boolean THEN e.enrolled_at END ASC,
    e.id ASC
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountEnrollments :one
SELECT count(*)
FROM misko.enrollments e
LEFT JOIN misko.group_assignments a
    ON a.enrollment_id = e.id AND a.valid_from <= now() AND (a.valid_to IS NULL OR a.valid_to > now())
WHERE e.experiment_id = @experiment_id
  AND (@subject_id::text = '' OR e.subject_id::text = @subject_id::text)
  AND (@group_id::text = '' OR a.group_id::text = @group_id::text);

-- name: ListSubjectEnrollments :many
SELECT e.id, e.experiment_id, e.subject_id, e.enrolled_at, e.created_at, coalesce(a.group_id::text, '')::text AS current_group_id
FROM misko.enrollments e
LEFT JOIN misko.group_assignments a
    ON a.enrollment_id = e.id AND a.valid_from <= now() AND (a.valid_to IS NULL OR a.valid_to > now())
WHERE e.subject_id = @subject_id
ORDER BY e.enrolled_at DESC, e.id ASC;

-- name: ListAssignments :many
SELECT id, experiment_id, enrollment_id, group_id, valid_from, valid_to, created_at
FROM misko.group_assignments
WHERE enrollment_id = @enrollment_id
ORDER BY valid_from, id;

-- name: CloseAssignment :execrows
UPDATE misko.group_assignments
SET valid_to = @valid_to::timestamptz
WHERE id = @id AND valid_to IS NULL;

-- name: CreateAssignment :one
INSERT INTO misko.group_assignments (experiment_id, enrollment_id, group_id, valid_from)
VALUES (@experiment_id, @enrollment_id, @group_id, @valid_from::timestamptz)
RETURNING id, experiment_id, enrollment_id, group_id, valid_from, valid_to, created_at;
