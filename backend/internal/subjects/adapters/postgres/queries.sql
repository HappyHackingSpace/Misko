-- name: CreateSubject :one
INSERT INTO misko.subjects (code, species, sex, strain, birth_date, notes)
VALUES (@code, @species, @sex, sqlc.narg(strain), sqlc.narg(birth_date)::date, sqlc.narg(notes))
RETURNING id, code, species, sex, strain, birth_date, notes, created_at, updated_at;

-- name: GetSubject :one
SELECT id, code, species, sex, strain, birth_date, notes, created_at, updated_at
FROM misko.subjects
WHERE id = @id;

-- The sort key is allowlisted by the application; id breaks ties for stable paging.
-- name: ListSubjects :many
SELECT id, code, species, sex, strain, birth_date, notes, created_at, updated_at
FROM misko.subjects
WHERE (@search::text = '' OR code ILIKE '%' || @search::text || '%' ESCAPE '\' OR strain ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@species::text = '' OR species = @species::text)
  AND (@sex::text = '' OR sex = @sex::text)
ORDER BY
    CASE WHEN @sort_key::text = 'code' AND NOT @descending::boolean THEN lower(code) END ASC,
    CASE WHEN @sort_key::text = 'code' AND @descending::boolean THEN lower(code) END DESC,
    CASE WHEN @sort_key::text = 'species' AND NOT @descending::boolean THEN species END ASC,
    CASE WHEN @sort_key::text = 'species' AND @descending::boolean THEN species END DESC,
    CASE WHEN @sort_key::text = 'sex' AND NOT @descending::boolean THEN sex END ASC,
    CASE WHEN @sort_key::text = 'sex' AND @descending::boolean THEN sex END DESC,
    CASE WHEN @sort_key::text = 'birthDate' AND NOT @descending::boolean THEN birth_date END ASC NULLS LAST,
    CASE WHEN @sort_key::text = 'birthDate' AND @descending::boolean THEN birth_date END DESC NULLS LAST,
    CASE WHEN @sort_key::text = 'createdAt' AND NOT @descending::boolean THEN created_at END ASC,
    CASE WHEN @sort_key::text = 'createdAt' AND @descending::boolean THEN created_at END DESC,
    id ASC
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountSubjects :one
SELECT count(*)
FROM misko.subjects
WHERE (@search::text = '' OR code ILIKE '%' || @search::text || '%' ESCAPE '\' OR strain ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@species::text = '' OR species = @species::text)
  AND (@sex::text = '' OR sex = @sex::text);

-- A NULL argument leaves a column unchanged; an empty strain or notes clears it.
-- name: UpdateSubject :one
UPDATE misko.subjects
SET code = coalesce(sqlc.narg(code), code),
    species = coalesce(sqlc.narg(species), species),
    sex = coalesce(sqlc.narg(sex), sex),
    strain = CASE WHEN sqlc.narg(strain)::text IS NULL THEN strain ELSE NULLIF(sqlc.narg(strain)::text, '') END,
    notes = CASE WHEN sqlc.narg(notes)::text IS NULL THEN notes ELSE NULLIF(sqlc.narg(notes)::text, '') END,
    birth_date = CASE WHEN @clear_birth_date::boolean THEN NULL ELSE coalesce(sqlc.narg(birth_date)::date, birth_date) END,
    updated_at = now()
WHERE id = @id
RETURNING id, code, species, sex, strain, birth_date, notes, created_at, updated_at;

-- name: DeleteSubject :execrows
DELETE FROM misko.subjects
WHERE id = @id;
