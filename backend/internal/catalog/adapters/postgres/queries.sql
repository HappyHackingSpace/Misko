-- The search is escaped by the store; id breaks ties between names that
-- differ only in case.
-- name: ListSubstances :many
SELECT id, name, kind, description, created_at, updated_at
FROM misko.substances
WHERE (@search::text = '' OR name ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@kind::text = '' OR kind = @kind::text)
ORDER BY lower(name), id;

-- name: GetSubstance :one
SELECT id, name, kind, description, created_at, updated_at
FROM misko.substances
WHERE id = @id;

-- name: CreateSubstance :one
INSERT INTO misko.substances (name, kind, description)
VALUES (@name, @kind, sqlc.narg(description))
RETURNING id, name, kind, description, created_at, updated_at;

-- A NULL argument leaves a column unchanged; an empty description clears it.
-- name: UpdateSubstance :one
UPDATE misko.substances
SET name = coalesce(sqlc.narg(name), name),
    kind = coalesce(sqlc.narg(kind), kind),
    description = CASE WHEN sqlc.narg(description)::text IS NULL THEN description ELSE NULLIF(sqlc.narg(description)::text, '') END
WHERE id = @id
RETURNING id, name, kind, description, created_at, updated_at;

-- name: DeleteSubstance :execrows
DELETE FROM misko.substances
WHERE id = @id;

-- name: ListDiseaseModels :many
SELECT id, name, induction_type, confirmation_criterion, description, created_at, updated_at
FROM misko.disease_models
WHERE (@search::text = '' OR name ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@induction_type::text = '' OR induction_type = @induction_type::text)
ORDER BY lower(name), id;

-- name: GetDiseaseModel :one
SELECT id, name, induction_type, confirmation_criterion, description, created_at, updated_at
FROM misko.disease_models
WHERE id = @id;

-- name: CreateDiseaseModel :one
INSERT INTO misko.disease_models (name, induction_type, confirmation_criterion, description)
VALUES (@name, @induction_type, sqlc.narg(confirmation_criterion), sqlc.narg(description))
RETURNING id, name, induction_type, confirmation_criterion, description, created_at, updated_at;

-- A NULL argument leaves a column unchanged; an empty criterion or description clears it.
-- name: UpdateDiseaseModel :one
UPDATE misko.disease_models
SET name = coalesce(sqlc.narg(name), name),
    induction_type = coalesce(sqlc.narg(induction_type), induction_type),
    confirmation_criterion = CASE WHEN sqlc.narg(confirmation_criterion)::text IS NULL THEN confirmation_criterion
                                  ELSE NULLIF(sqlc.narg(confirmation_criterion)::text, '') END,
    description = CASE WHEN sqlc.narg(description)::text IS NULL THEN description ELSE NULLIF(sqlc.narg(description)::text, '') END
WHERE id = @id
RETURNING id, name, induction_type, confirmation_criterion, description, created_at, updated_at;

-- name: DeleteDiseaseModel :execrows
DELETE FROM misko.disease_models
WHERE id = @id;
