-- name: ListEnvironments :many
SELECT e.id, e.name, e.paradigm_key, e.notes, e.created_at, e.updated_at,
       coalesce((SELECT max(r.number) FROM misko.environment_revisions r WHERE r.environment_id = e.id), 0)::integer AS latest_revision
FROM misko.environments e
WHERE @paradigm_key::text = '' OR e.paradigm_key = @paradigm_key::text
ORDER BY lower(e.name), e.id;

-- name: GetEnvironment :one
SELECT e.id, e.name, e.paradigm_key, e.notes, e.created_at, e.updated_at,
       coalesce((SELECT max(r.number) FROM misko.environment_revisions r WHERE r.environment_id = e.id), 0)::integer AS latest_revision
FROM misko.environments e
WHERE e.id = @id;

-- name: LockEnvironment :one
SELECT id, name, paradigm_key, notes, created_at, updated_at
FROM misko.environments
WHERE id = @id
FOR UPDATE;

-- name: CreateEnvironment :one
INSERT INTO misko.environments (name, paradigm_key, notes)
VALUES (@name, @paradigm_key, sqlc.narg(notes))
RETURNING id, name, paradigm_key, notes, created_at, updated_at;

-- name: UpdateEnvironment :one
UPDATE misko.environments AS e
SET name = coalesce(sqlc.narg(name), e.name),
    notes = CASE WHEN sqlc.narg(notes)::text IS NULL THEN e.notes ELSE NULLIF(sqlc.narg(notes)::text, '') END,
    updated_at = now()
WHERE e.id = @id
RETURNING e.id, e.name, e.paradigm_key, e.notes, e.created_at, e.updated_at,
          coalesce((SELECT max(r.number) FROM misko.environment_revisions r WHERE r.environment_id = e.id), 0)::integer AS latest_revision;

-- CreateRevision numbers revisions per environment; callers lock the
-- environment first so concurrent revisions do not collide.
-- name: CreateRevision :one
INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, notes, created_by)
SELECT @environment_id::uuid, @paradigm_key::text, coalesce(max(r.number), 0) + 1, @paradigm_version::integer, @apparatus::jsonb, sqlc.narg(notes)::text, @created_by::uuid
FROM misko.environment_revisions r
WHERE r.environment_id = @environment_id::uuid
RETURNING id, environment_id, paradigm_key, number, paradigm_version, apparatus, notes, created_by, created_at;

-- name: ListRevisions :many
SELECT id, environment_id, paradigm_key, number, paradigm_version, apparatus, notes, created_by, created_at
FROM misko.environment_revisions
WHERE environment_id = @environment_id
ORDER BY number;

-- name: GetRevision :one
SELECT id, environment_id, paradigm_key, number, paradigm_version, apparatus, notes, created_by, created_at
FROM misko.environment_revisions
WHERE environment_id = @environment_id AND number = @number;
