-- Reads allocation, group and experiment rows to validate a record; the
-- foreign keys and triggers on administrations and induction_outcomes enforce
-- the same rules.
-- name: GetAllocationRef :one
SELECT a.id, a.experiment_id, a.group_id, a.subject_id, a.allocated_at, a.excluded_at,
       e.status AS experiment_status, (g.disease_model_id IS NOT NULL)::boolean AS has_disease
FROM misko.allocations a
JOIN misko.experiments e ON e.id = a.experiment_id
JOIN misko.experiment_groups g ON g.experiment_id = a.experiment_id AND g.id = a.group_id
WHERE a.experiment_id = @experiment_id AND a.id = @id;

-- name: GetRegimenItem :one
SELECT r.id, r.experiment_id, r.group_id, r.purpose, r.substance_id, s.name AS substance_name, r.dose_micro, r.unit, r.route,
       r.frequency, r.interval_days, r.start_day, r.end_day
FROM misko.regimen_items r
JOIN misko.substances s ON s.id = r.substance_id
WHERE r.experiment_id = @experiment_id AND r.id = @id;

-- name: GetSubstanceName :one
SELECT name FROM misko.substances WHERE id = @id;

-- name: GetWeight :one
SELECT id, subject_id, body_milligrams, measured_at
FROM misko.weight_measurements
WHERE id = @id;

-- name: LatestWeight :one
SELECT id, subject_id, body_milligrams, measured_at
FROM misko.weight_measurements
WHERE subject_id = @subject_id AND measured_at <= @at::timestamptz
ORDER BY measured_at DESC, created_at DESC, id DESC
LIMIT 1;

-- name: CreateAdministration :one
INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, regimen_item_id, purpose, substance_id,
    dose_micro, unit, route, weight_measurement_id, administered_at, notes, recorded_by)
VALUES (@experiment_id, @allocation_id, @group_id, @subject_id, sqlc.narg(regimen_item_id), @purpose, @substance_id,
    @dose_micro, @unit, @route, sqlc.narg(weight_measurement_id), @administered_at, sqlc.narg(notes), @recorded_by)
RETURNING id, created_at;

-- to_time is exclusive.
-- name: ListAdministrations :many
SELECT d.id, d.experiment_id, d.allocation_id, d.group_id, d.subject_id, d.regimen_item_id, d.purpose, d.substance_id,
       s.name AS substance_name, d.dose_micro, d.unit, d.route, d.weight_measurement_id, w.body_milligrams,
       d.administered_at, d.notes, d.recorded_by, d.created_at
FROM misko.administrations d
JOIN misko.substances s ON s.id = d.substance_id
LEFT JOIN misko.weight_measurements w ON w.id = d.weight_measurement_id
WHERE (@experiment_id::text = '' OR d.experiment_id::text = @experiment_id::text)
  AND (@allocation_id::text = '' OR d.allocation_id::text = @allocation_id::text)
  AND (@subject_id::text = '' OR d.subject_id::text = @subject_id::text)
  AND (@substance_id::text = '' OR d.substance_id::text = @substance_id::text)
  AND (@purpose::text = '' OR d.purpose = @purpose::text)
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR d.administered_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR d.administered_at < sqlc.narg(to_time)::timestamptz)
ORDER BY d.administered_at DESC, d.id DESC
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountAdministrations :one
SELECT count(*)
FROM misko.administrations d
WHERE (@experiment_id::text = '' OR d.experiment_id::text = @experiment_id::text)
  AND (@allocation_id::text = '' OR d.allocation_id::text = @allocation_id::text)
  AND (@subject_id::text = '' OR d.subject_id::text = @subject_id::text)
  AND (@substance_id::text = '' OR d.substance_id::text = @substance_id::text)
  AND (@purpose::text = '' OR d.purpose = @purpose::text)
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR d.administered_at >= sqlc.narg(from_time)::timestamptz)
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR d.administered_at < sqlc.narg(to_time)::timestamptz);

-- Day 0 and the laboratory's current date as text in the form YYYY-MM-DD.
-- name: GetExperimentDay :one
SELECT e.status, coalesce(to_char(e.day_zero, 'YYYY-MM-DD'), '')::text AS day_zero,
       to_char((now() AT TIME ZONE l.timezone)::date, 'YYYY-MM-DD')::text AS today
FROM misko.experiments e
CROSS JOIN misko.laboratory l
WHERE e.id = @id;

-- Every regimen item of every animal allocated by the end of the day and not
-- excluded before it began. The day runs from midnight to midnight in the
-- laboratory's time zone; given counts that item's doses in that window and
-- the weight is the latest measured before the window ends.
-- name: ListDueCandidates :many
WITH bounds AS (
    SELECT (CAST(@day::text AS date)::timestamp AT TIME ZONE l.timezone) AS day_start,
           ((CAST(@day::text AS date) + 1)::timestamp AT TIME ZONE l.timezone) AS day_end
    FROM misko.laboratory l
)
SELECT a.id AS allocation_id, a.experiment_id, a.group_id, a.subject_id, a.allocated_at, a.excluded_at,
       sub.code AS subject_code, g.code AS group_code, a.cage,
       (a.excluded_at IS NOT NULL AND a.excluded_at < b.day_end)::boolean AS excluded,
       r.id AS regimen_item_id, r.purpose, r.substance_id, s.name AS substance_name, r.dose_micro, r.unit, r.route,
       r.frequency, r.interval_days, r.start_day, r.end_day,
       (SELECT count(*) FROM misko.administrations d
        WHERE d.allocation_id = a.id AND d.regimen_item_id = r.id
          AND d.administered_at >= b.day_start AND d.administered_at < b.day_end)::integer AS given,
       -- No weight yet: an empty id, with placeholders sqlc can scan.
       coalesce(w.id::text, '')::text AS weight_id, coalesce(w.body_milligrams, 0)::bigint AS weight_milligrams,
       coalesce(w.measured_at, a.allocated_at)::timestamptz AS weight_measured_at
FROM bounds b
JOIN misko.allocations a ON a.experiment_id = @experiment_id
JOIN misko.subjects sub ON sub.id = a.subject_id
JOIN misko.experiment_groups g ON g.experiment_id = a.experiment_id AND g.id = a.group_id
JOIN misko.regimen_items r ON r.experiment_id = a.experiment_id AND r.group_id = a.group_id
JOIN misko.substances s ON s.id = r.substance_id
LEFT JOIN LATERAL (
    SELECT wm.id, wm.body_milligrams, wm.measured_at
    FROM misko.weight_measurements wm
    WHERE wm.subject_id = a.subject_id AND wm.measured_at < b.day_end
    ORDER BY wm.measured_at DESC, wm.created_at DESC, wm.id DESC
    LIMIT 1
) w ON true
WHERE a.allocated_at < b.day_end
  AND (a.excluded_at IS NULL OR a.excluded_at >= b.day_start)
ORDER BY lower(g.code), lower(sub.code), a.id,
         CASE r.purpose WHEN 'INDUCTION' THEN 1 WHEN 'VEHICLE' THEN 2 ELSE 3 END, r.start_day, r.id;

-- name: CreateInductionOutcome :one
INSERT INTO misko.induction_outcomes (allocation_id, experiment_id, status, assessed_at, note, recorded_by)
VALUES (@allocation_id, @experiment_id, @status, @assessed_at, sqlc.narg(note), @recorded_by)
RETURNING allocation_id, experiment_id, status, assessed_at, note, recorded_by, created_at;

-- name: ListInductionOutcomes :many
SELECT o.allocation_id, o.experiment_id, o.status, o.assessed_at, o.note, o.recorded_by, o.created_at,
       a.subject_id, sub.code AS subject_code, a.group_id, g.code AS group_code
FROM misko.induction_outcomes o
JOIN misko.allocations a ON a.id = o.allocation_id
JOIN misko.subjects sub ON sub.id = a.subject_id
JOIN misko.experiment_groups g ON g.id = a.group_id
WHERE o.experiment_id = @experiment_id
  AND (@status::text = '' OR o.status = @status::text)
ORDER BY lower(g.code), lower(sub.code), o.allocation_id;
