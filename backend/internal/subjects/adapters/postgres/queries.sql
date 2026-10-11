-- name: CreateSubject :one
INSERT INTO misko.subjects (code, species, sex)
VALUES (@code, @species, @sex)
RETURNING id, code, species, sex, created_at, updated_at;

-- name: GetSubject :one
SELECT id, code, species, sex, created_at, updated_at
FROM misko.subjects
WHERE id = @id;

-- The sort key is allowlisted by the application; id breaks ties for stable paging.
-- name: ListSubjects :many
SELECT id, code, species, sex, created_at, updated_at
FROM misko.subjects
WHERE (@search::text = '' OR code ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@species::text = '' OR species = @species::text)
  AND (@sex::text = '' OR sex = @sex::text)
ORDER BY
    CASE WHEN @sort_key::text = 'code' AND NOT @descending::boolean THEN lower(code) END ASC,
    CASE WHEN @sort_key::text = 'code' AND @descending::boolean THEN lower(code) END DESC,
    CASE WHEN @sort_key::text = 'species' AND NOT @descending::boolean THEN species END ASC,
    CASE WHEN @sort_key::text = 'species' AND @descending::boolean THEN species END DESC,
    CASE WHEN @sort_key::text = 'sex' AND NOT @descending::boolean THEN sex END ASC,
    CASE WHEN @sort_key::text = 'sex' AND @descending::boolean THEN sex END DESC,
    CASE WHEN @sort_key::text = 'createdAt' AND NOT @descending::boolean THEN created_at END ASC,
    CASE WHEN @sort_key::text = 'createdAt' AND @descending::boolean THEN created_at END DESC,
    id ASC
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountSubjects :one
SELECT count(*)
FROM misko.subjects
WHERE (@search::text = '' OR code ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@species::text = '' OR species = @species::text)
  AND (@sex::text = '' OR sex = @sex::text);

-- A NULL argument leaves a column unchanged.
-- name: UpdateSubject :one
UPDATE misko.subjects
SET code = coalesce(sqlc.narg(code), code),
    species = coalesce(sqlc.narg(species), species),
    sex = coalesce(sqlc.narg(sex), sex),
    updated_at = now()
WHERE id = @id
RETURNING id, code, species, sex, created_at, updated_at;

-- name: DeleteSubject :execrows
DELETE FROM misko.subjects
WHERE id = @id;
