-- Every read of a subject carries its allocation, if any: the experiment and
-- group it belongs to and whether it was excluded. An animal has at most one
-- allocation (allocations_subject_key), so the joins never repeat a subject.

-- name: CreateSubject :one
INSERT INTO misko.subjects (code, species, sex)
VALUES (@code, @species, @sex)
RETURNING id, code, species, sex, created_at, updated_at;

-- name: GetSubject :one
SELECT s.id, s.code, s.species, s.sex, s.created_at, s.updated_at,
       a.experiment_id AS allocation_experiment_id, e.code AS allocation_experiment_code,
       a.group_id AS allocation_group_id, g.code AS allocation_group_code, g.role AS allocation_group_role,
       (a.excluded_at IS NOT NULL)::boolean AS allocation_excluded
FROM misko.subjects s
LEFT JOIN misko.allocations a ON a.subject_id = s.id
LEFT JOIN misko.experiments e ON e.id = a.experiment_id
LEFT JOIN misko.experiment_groups g ON g.id = a.group_id
WHERE s.id = @id;

-- The sort key is allowlisted by the application; id breaks ties for stable paging.
-- name: ListSubjects :many
SELECT s.id, s.code, s.species, s.sex, s.created_at, s.updated_at,
       a.experiment_id AS allocation_experiment_id, e.code AS allocation_experiment_code,
       a.group_id AS allocation_group_id, g.code AS allocation_group_code, g.role AS allocation_group_role,
       (a.excluded_at IS NOT NULL)::boolean AS allocation_excluded
FROM misko.subjects s
LEFT JOIN misko.allocations a ON a.subject_id = s.id
LEFT JOIN misko.experiments e ON e.id = a.experiment_id
LEFT JOIN misko.experiment_groups g ON g.id = a.group_id
WHERE (@search::text = '' OR s.code ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@species::text = '' OR s.species = @species::text)
  AND (@sex::text = '' OR s.sex = @sex::text)
  AND (NOT @unallocated::boolean OR a.id IS NULL)
ORDER BY
    CASE WHEN @sort_key::text = 'code' AND NOT @descending::boolean THEN lower(s.code) END ASC,
    CASE WHEN @sort_key::text = 'code' AND @descending::boolean THEN lower(s.code) END DESC,
    CASE WHEN @sort_key::text = 'species' AND NOT @descending::boolean THEN s.species END ASC,
    CASE WHEN @sort_key::text = 'species' AND @descending::boolean THEN s.species END DESC,
    CASE WHEN @sort_key::text = 'sex' AND NOT @descending::boolean THEN s.sex END ASC,
    CASE WHEN @sort_key::text = 'sex' AND @descending::boolean THEN s.sex END DESC,
    CASE WHEN @sort_key::text = 'createdAt' AND NOT @descending::boolean THEN s.created_at END ASC,
    CASE WHEN @sort_key::text = 'createdAt' AND @descending::boolean THEN s.created_at END DESC,
    s.id ASC
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountSubjects :one
SELECT count(*)
FROM misko.subjects s
WHERE (@search::text = '' OR s.code ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@species::text = '' OR s.species = @species::text)
  AND (@sex::text = '' OR s.sex = @sex::text)
  AND (NOT @unallocated::boolean OR NOT EXISTS (SELECT 1 FROM misko.allocations a WHERE a.subject_id = s.id));

-- A NULL argument leaves a column unchanged. The store reads the subject
-- back with GetSubject to include its allocation.
-- name: UpdateSubject :one
UPDATE misko.subjects
SET code = coalesce(sqlc.narg(code), code),
    species = coalesce(sqlc.narg(species), species),
    sex = coalesce(sqlc.narg(sex), sex)
WHERE id = @id
RETURNING id;

-- name: DeleteSubject :execrows
DELETE FROM misko.subjects
WHERE id = @id;

-- name: CreateWeight :one
INSERT INTO misko.weight_measurements (subject_id, body_milligrams, measured_at, recorded_by)
VALUES (@subject_id, @body_milligrams, @measured_at, @recorded_by)
RETURNING id, subject_id, body_milligrams, measured_at, recorded_by, created_at;

-- name: ListWeights :many
SELECT id, subject_id, body_milligrams, measured_at, recorded_by, created_at
FROM misko.weight_measurements
WHERE subject_id = @subject_id
ORDER BY measured_at DESC, id DESC;
