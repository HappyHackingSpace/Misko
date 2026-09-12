-- name: GetLaboratory :one
SELECT name, code, timezone, created_at, updated_at
FROM misko.laboratory
WHERE singleton;

-- name: CreateLaboratory :execrows
INSERT INTO misko.laboratory (name, code, timezone)
VALUES (@name, sqlc.narg(code), @timezone)
ON CONFLICT (singleton) DO NOTHING;

-- set_code distinguishes "leave code unchanged" from "clear code".
-- name: UpdateLaboratory :one
UPDATE misko.laboratory
SET name = coalesce(sqlc.narg(name), name),
    code = CASE WHEN @set_code::boolean THEN sqlc.narg(code) ELSE code END,
    timezone = coalesce(sqlc.narg(timezone), timezone),
    updated_at = now()
WHERE singleton
RETURNING name, code, timezone, created_at, updated_at;
