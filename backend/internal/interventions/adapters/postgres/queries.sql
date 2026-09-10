-- name: ListDiseaseModels :many
SELECT id, name, description, created_at, updated_at
FROM misko.disease_models
ORDER BY lower(name), id;

-- name: GetDiseaseModel :one
SELECT id, name, description, created_at, updated_at
FROM misko.disease_models
WHERE id = @id;

-- name: CreateDiseaseModel :one
INSERT INTO misko.disease_models (name, description)
VALUES (@name, sqlc.narg(description))
RETURNING id, name, description, created_at, updated_at;

-- name: UpdateDiseaseModel :one
UPDATE misko.disease_models
SET name = coalesce(sqlc.narg(name), name),
    description = CASE WHEN sqlc.narg(description)::text IS NULL THEN description ELSE NULLIF(sqlc.narg(description)::text, '') END,
    updated_at = now()
WHERE id = @id
RETURNING id, name, description, created_at, updated_at;

-- name: ListSubstances :many
SELECT id, name, description, created_at, updated_at
FROM misko.substances
ORDER BY lower(name), id;

-- name: GetSubstance :one
SELECT id, name, description, created_at, updated_at
FROM misko.substances
WHERE id = @id;

-- name: CreateSubstance :one
INSERT INTO misko.substances (name, description)
VALUES (@name, sqlc.narg(description))
RETURNING id, name, description, created_at, updated_at;

-- name: UpdateSubstance :one
UPDATE misko.substances
SET name = coalesce(sqlc.narg(name), name),
    description = CASE WHEN sqlc.narg(description)::text IS NULL THEN description ELSE NULLIF(sqlc.narg(description)::text, '') END,
    updated_at = now()
WHERE id = @id
RETURNING id, name, description, created_at, updated_at;

-- name: ListPlans :many
SELECT id, experiment_id, group_id, phase_id, substance_id, amount_micro, unit, route, schedule, notes, created_at, updated_at
FROM misko.intervention_plans
WHERE experiment_id = @experiment_id
ORDER BY created_at, id;

-- name: GetPlan :one
SELECT id, experiment_id, group_id, phase_id, substance_id, amount_micro, unit, route, schedule, notes, created_at, updated_at
FROM misko.intervention_plans
WHERE experiment_id = @experiment_id AND id = @id;

-- name: CreatePlan :one
INSERT INTO misko.intervention_plans (experiment_id, group_id, phase_id, substance_id, amount_micro, unit, route, schedule, notes)
VALUES (@experiment_id, @group_id, sqlc.narg(phase_id), @substance_id, @amount_micro, @unit, @route, @schedule, sqlc.narg(notes))
RETURNING id, experiment_id, group_id, phase_id, substance_id, amount_micro, unit, route, schedule, notes, created_at, updated_at;

-- name: UpdatePlan :one
UPDATE misko.intervention_plans
SET amount_micro = coalesce(sqlc.narg(amount_micro), amount_micro),
    unit = coalesce(sqlc.narg(unit), unit),
    route = coalesce(sqlc.narg(route), route),
    schedule = coalesce(sqlc.narg(schedule), schedule),
    notes = CASE WHEN sqlc.narg(notes)::text IS NULL THEN notes ELSE NULLIF(sqlc.narg(notes)::text, '') END,
    updated_at = now()
WHERE experiment_id = @experiment_id AND id = @id
RETURNING id, experiment_id, group_id, phase_id, substance_id, amount_micro, unit, route, schedule, notes, created_at, updated_at;

-- name: CreateWeight :one
INSERT INTO misko.weight_measurements (subject_id, body_milligrams, measured_at, recorded_by)
VALUES (@subject_id, @body_milligrams, @measured_at, @recorded_by)
RETURNING id, subject_id, body_milligrams, measured_at, recorded_by, created_at;

-- name: GetWeight :one
SELECT id, subject_id, body_milligrams, measured_at, recorded_by, created_at
FROM misko.weight_measurements
WHERE id = @id;

-- name: ListWeights :many
SELECT id, subject_id, body_milligrams, measured_at, recorded_by, created_at
FROM misko.weight_measurements
WHERE subject_id = @subject_id
ORDER BY measured_at DESC, id DESC;

-- name: CreateCondition :one
INSERT INTO misko.subject_conditions (subject_id, disease_model_id, disease_model_name, enrollment_id, status, observed_at, notes, recorded_by)
VALUES (@subject_id, @disease_model_id, @disease_model_name, sqlc.narg(enrollment_id), @status, @observed_at, sqlc.narg(notes), @recorded_by)
RETURNING id, subject_id, disease_model_id, disease_model_name, enrollment_id, status, observed_at, notes, recorded_by, created_at;

-- name: ListSubjectConditions :many
SELECT id, subject_id, disease_model_id, disease_model_name, enrollment_id, status, observed_at, notes, recorded_by, created_at
FROM misko.subject_conditions
WHERE subject_id = @subject_id
  AND (@disease_model_id::text = '' OR disease_model_id::text = @disease_model_id::text)
ORDER BY observed_at DESC, created_at DESC, id DESC;

-- current_only keeps the latest observation per subject and disease model,
-- ordered by observation time, then recording time, then id.
-- name: ListConditions :many
SELECT c.id, c.subject_id, c.disease_model_id, c.disease_model_name, c.enrollment_id, c.status, c.observed_at, c.notes, c.recorded_by, c.created_at
FROM misko.subject_conditions c
WHERE (@disease_model_id::text = '' OR c.disease_model_id::text = @disease_model_id::text)
  AND (@status::text = '' OR c.status = @status::text)
  AND (NOT @current_only::boolean OR NOT EXISTS (
      SELECT 1 FROM misko.subject_conditions l
      WHERE l.subject_id = c.subject_id AND l.disease_model_id = c.disease_model_id
        AND (l.observed_at, l.created_at, l.id) > (c.observed_at, c.created_at, c.id)))
ORDER BY c.observed_at DESC, c.created_at DESC, c.id DESC
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountConditions :one
SELECT count(*)
FROM misko.subject_conditions c
WHERE (@disease_model_id::text = '' OR c.disease_model_id::text = @disease_model_id::text)
  AND (@status::text = '' OR c.status = @status::text)
  AND (NOT @current_only::boolean OR NOT EXISTS (
      SELECT 1 FROM misko.subject_conditions l
      WHERE l.subject_id = c.subject_id AND l.disease_model_id = c.disease_model_id
        AND (l.observed_at, l.created_at, l.id) > (c.observed_at, c.created_at, c.id)));

-- Reads enrollment and assignment rows to validate records; the foreign keys
-- on administrations and conditions enforce the same references.
-- name: GetEnrollmentRef :one
SELECT id, experiment_id, subject_id, enrolled_at
FROM misko.enrollments
WHERE experiment_id = @experiment_id AND id = @id;

-- name: GroupAt :one
SELECT group_id
FROM misko.group_assignments
WHERE enrollment_id = @enrollment_id
  AND valid_from <= @at::timestamptz
  AND (valid_to IS NULL OR valid_to > @at::timestamptz);

-- name: CreateAdministration :one
INSERT INTO misko.administrations (experiment_id, enrollment_id, subject_id, substance_id, substance_name, plan_id, weight_measurement_id, body_milligrams, amount_micro, unit, route, administered_at, notes, recorded_by)
VALUES (@experiment_id, @enrollment_id, @subject_id, @substance_id, @substance_name, sqlc.narg(plan_id), sqlc.narg(weight_measurement_id), sqlc.narg(body_milligrams), @amount_micro, @unit, @route, @administered_at, sqlc.narg(notes), @recorded_by)
RETURNING id, experiment_id, enrollment_id, subject_id, substance_id, substance_name, plan_id, weight_measurement_id, body_milligrams, amount_micro, unit, route, administered_at, notes, recorded_by, created_at;

-- to_time is exclusive.
-- name: ListAdministrations :many
SELECT id, experiment_id, enrollment_id, subject_id, substance_id, substance_name, plan_id, weight_measurement_id, body_milligrams, amount_micro, unit, route, administered_at, notes, recorded_by, created_at
FROM misko.administrations
WHERE (@subject_id::text = '' OR subject_id::text = @subject_id::text)
  AND (@substance_id::text = '' OR substance_id::text = @substance_id::text)
  AND (@experiment_id::text = '' OR experiment_id::text = @experiment_id::text)
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR administered_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR administered_at < sqlc.narg(to_time)::timestamptz)
ORDER BY administered_at DESC, id DESC
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountAdministrations :one
SELECT count(*)
FROM misko.administrations
WHERE (@subject_id::text = '' OR subject_id::text = @subject_id::text)
  AND (@substance_id::text = '' OR substance_id::text = @substance_id::text)
  AND (@experiment_id::text = '' OR experiment_id::text = @experiment_id::text)
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR administered_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR administered_at < sqlc.narg(to_time)::timestamptz);
