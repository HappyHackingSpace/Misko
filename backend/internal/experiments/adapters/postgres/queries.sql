-- Experiments ----------------------------------------------------------------

-- name: CreateExperiment :one
INSERT INTO misko.experiments (code, title, description, hypothesis, randomisation, blinded)
VALUES (@code, @title, sqlc.narg(description), sqlc.narg(hypothesis), sqlc.narg(randomisation), @blinded)
RETURNING *;

-- name: GetExperiment :one
SELECT * FROM misko.experiments WHERE id = @id;

-- name: LockExperiment :one
SELECT * FROM misko.experiments WHERE id = @id FOR UPDATE;

-- name: ListExperiments :many
SELECT *
FROM misko.experiments
WHERE (@search::text = '' OR code ILIKE '%' || @search::text || '%' ESCAPE '\' OR title ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@status::text = '' OR status = @status::text)
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
WHERE (@search::text = '' OR code ILIKE '%' || @search::text || '%' ESCAPE '\' OR title ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@status::text = '' OR status = @status::text);

-- name: UpdateExperiment :one
UPDATE misko.experiments
SET code = @code, title = @title, description = sqlc.narg(description), hypothesis = sqlc.narg(hypothesis),
    randomisation = sqlc.narg(randomisation), blinded = @blinded
WHERE id = @id
RETURNING *;

-- name: StartExperiment :one
UPDATE misko.experiments
SET status = 'RUNNING', day_zero = @day_zero::date, started_at = now()
WHERE id = @id AND status = 'DRAFT'
RETURNING *;

-- name: CompleteExperiment :one
UPDATE misko.experiments
SET status = 'COMPLETED', completed_at = greatest(now(), started_at)
WHERE id = @id AND status = 'RUNNING'
RETURNING *;

-- Tests belong to the tests context; completion only counts the open ones.
-- name: CountOpenTests :one
SELECT count(*) FROM misko.tests WHERE experiment_id = @experiment_id AND status IN ('PLANNED', 'IN_PROGRESS');

-- Protocol steps belong to the protocols context; endpoints only read which
-- paradigm versions they run.
-- name: ListProtocolParadigms :many
SELECT DISTINCT s.paradigm_key, s.paradigm_version
FROM misko.protocol_steps s
JOIN misko.protocol_versions v ON v.id = s.protocol_version_id
WHERE v.experiment_id = @experiment_id
ORDER BY s.paradigm_key, s.paradigm_version;

-- Groups ---------------------------------------------------------------------

-- name: ListGroupFills :many
SELECT g.*,
       count(a.id) FILTER (WHERE a.excluded_at IS NULL)::integer AS allocated,
       count(a.id) FILTER (WHERE a.excluded_at IS NOT NULL)::integer AS excluded
FROM misko.experiment_groups g
LEFT JOIN misko.allocations a ON a.group_id = g.id
WHERE g.experiment_id = @experiment_id
GROUP BY g.id
ORDER BY lower(g.code), g.id;

-- name: LockGroup :one
SELECT * FROM misko.experiment_groups WHERE experiment_id = @experiment_id AND id = @id FOR UPDATE;

-- name: CreateGroup :one
INSERT INTO misko.experiment_groups (experiment_id, code, name, role, disease_model_id, target_size, description)
VALUES (@experiment_id, @code, @name, @role, sqlc.narg(disease_model_id), sqlc.narg(target_size), sqlc.narg(description))
RETURNING *;

-- name: UpdateGroup :one
UPDATE misko.experiment_groups
SET code = @code, name = @name, role = @role, disease_model_id = sqlc.narg(disease_model_id),
    target_size = sqlc.narg(target_size), description = sqlc.narg(description)
WHERE experiment_id = @experiment_id AND id = @id
RETURNING *;

-- name: DeleteGroupRegimen :exec
DELETE FROM misko.regimen_items WHERE experiment_id = @experiment_id AND group_id = @group_id;

-- name: DeleteGroup :execrows
DELETE FROM misko.experiment_groups WHERE experiment_id = @experiment_id AND id = @id;

-- Regimen items --------------------------------------------------------------

-- name: ListRegimenItems :many
SELECT r.*
FROM misko.regimen_items r
JOIN misko.experiment_groups g ON g.id = r.group_id
WHERE r.experiment_id = @experiment_id
ORDER BY lower(g.code), r.start_day, r.purpose, r.id;

-- name: LockRegimenItem :one
SELECT * FROM misko.regimen_items WHERE experiment_id = @experiment_id AND id = @id FOR UPDATE;

-- name: CreateRegimenItem :one
INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, interval_days, start_day, end_day)
VALUES (@experiment_id, @group_id, @purpose, @substance_id, @dose_micro, @unit, @route, @frequency, sqlc.narg(interval_days), @start_day, @end_day)
RETURNING *;

-- name: UpdateRegimenItem :one
UPDATE misko.regimen_items
SET group_id = @group_id, purpose = @purpose, substance_id = @substance_id, dose_micro = @dose_micro, unit = @unit,
    route = @route, frequency = @frequency, interval_days = sqlc.narg(interval_days), start_day = @start_day, end_day = @end_day
WHERE experiment_id = @experiment_id AND id = @id
RETURNING *;

-- name: DeleteRegimenItem :execrows
DELETE FROM misko.regimen_items WHERE experiment_id = @experiment_id AND id = @id;

-- Timepoints -----------------------------------------------------------------

-- name: ListTimepoints :many
SELECT * FROM misko.timepoints WHERE experiment_id = @experiment_id ORDER BY position, id;

-- name: LockTimepoint :one
SELECT * FROM misko.timepoints WHERE experiment_id = @experiment_id AND id = @id FOR UPDATE;

-- name: CreateTimepoint :one
INSERT INTO misko.timepoints (experiment_id, name, study_day, position)
VALUES (@experiment_id, @name, @study_day, @position)
RETURNING *;

-- name: UpdateTimepoint :one
UPDATE misko.timepoints
SET name = @name, study_day = @study_day, position = @position
WHERE experiment_id = @experiment_id AND id = @id
RETURNING *;

-- name: DeleteTimepoint :execrows
DELETE FROM misko.timepoints WHERE experiment_id = @experiment_id AND id = @id;

-- Endpoints ------------------------------------------------------------------

-- name: ListEndpoints :many
SELECT e.*
FROM misko.endpoints e
JOIN misko.timepoints t ON t.id = e.timepoint_id
WHERE e.experiment_id = @experiment_id
ORDER BY e.kind = 'SECONDARY', t.position, e.paradigm_key, e.metric_key, e.id;

-- name: LockEndpoint :one
SELECT * FROM misko.endpoints WHERE experiment_id = @experiment_id AND id = @id FOR UPDATE;

-- name: CreateEndpoint :one
INSERT INTO misko.endpoints (experiment_id, timepoint_id, baseline_timepoint_id, paradigm_key, metric_key, kind, direction)
VALUES (@experiment_id, @timepoint_id, sqlc.narg(baseline_timepoint_id), @paradigm_key, @metric_key, @kind, @direction)
RETURNING *;

-- name: UpdateEndpoint :one
UPDATE misko.endpoints
SET timepoint_id = @timepoint_id, baseline_timepoint_id = sqlc.narg(baseline_timepoint_id), paradigm_key = @paradigm_key,
    metric_key = @metric_key, kind = @kind, direction = @direction
WHERE experiment_id = @experiment_id AND id = @id
RETURNING *;

-- name: DeleteEndpoint :execrows
DELETE FROM misko.endpoints WHERE experiment_id = @experiment_id AND id = @id;

-- Allocations ----------------------------------------------------------------

-- name: ListAllocations :many
SELECT a.*, s.code AS subject_code
FROM misko.allocations a
JOIN misko.subjects s ON s.id = a.subject_id
JOIN misko.experiment_groups g ON g.id = a.group_id
WHERE a.experiment_id = @experiment_id
  AND (sqlc.narg(group_id)::uuid IS NULL OR a.group_id = sqlc.narg(group_id)::uuid)
  AND (sqlc.narg(excluded)::boolean IS NULL OR (a.excluded_at IS NOT NULL) = sqlc.narg(excluded)::boolean)
ORDER BY lower(g.code), lower(s.code), a.id;

-- name: LockAllocation :one
SELECT * FROM misko.allocations WHERE experiment_id = @experiment_id AND id = @id FOR UPDATE;

-- name: CreateAllocation :one
INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, cage, created_by)
VALUES (@experiment_id, @group_id, @subject_id, @allocated_at, sqlc.narg(cage), @created_by)
RETURNING *;

-- name: UpdateAllocation :one
UPDATE misko.allocations
SET group_id = @group_id, cage = sqlc.narg(cage), excluded_at = sqlc.narg(excluded_at),
    exclusion_reason = sqlc.narg(exclusion_reason), exclusion_note = sqlc.narg(exclusion_note)
WHERE experiment_id = @experiment_id AND id = @id
RETURNING *;

-- name: DeleteAllocation :execrows
DELETE FROM misko.allocations WHERE experiment_id = @experiment_id AND id = @id;
