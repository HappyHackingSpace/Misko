-- name: ListProtocols :many
SELECT p.id, p.experiment_id, p.name, p.description, p.created_at, p.updated_at,
       coalesce((SELECT max(v.number) FROM misko.protocol_versions v WHERE v.protocol_id = p.id), 0)::integer AS latest_version
FROM misko.protocols p
WHERE p.experiment_id = @experiment_id
ORDER BY lower(p.name), p.id;

-- name: GetProtocol :one
SELECT p.id, p.experiment_id, p.name, p.description, p.created_at, p.updated_at,
       coalesce((SELECT max(v.number) FROM misko.protocol_versions v WHERE v.protocol_id = p.id), 0)::integer AS latest_version
FROM misko.protocols p
WHERE p.experiment_id = @experiment_id AND p.id = @id;

-- name: LockProtocol :one
SELECT id, experiment_id, name, description, created_at, updated_at
FROM misko.protocols
WHERE experiment_id = @experiment_id AND id = @id
FOR UPDATE;

-- name: CreateProtocol :one
INSERT INTO misko.protocols (experiment_id, name, description)
VALUES (@experiment_id, @name, sqlc.narg(description))
RETURNING id, experiment_id, name, description, created_at, updated_at;

-- name: UpdateProtocol :one
UPDATE misko.protocols AS p
SET name = coalesce(sqlc.narg(name), p.name),
    description = CASE WHEN sqlc.narg(description)::text IS NULL THEN p.description ELSE NULLIF(sqlc.narg(description)::text, '') END,
    updated_at = now()
WHERE p.experiment_id = @experiment_id AND p.id = @id
RETURNING p.id, p.experiment_id, p.name, p.description, p.created_at, p.updated_at,
          coalesce((SELECT max(v.number) FROM misko.protocol_versions v WHERE v.protocol_id = p.id), 0)::integer AS latest_version;

-- name: GetEnvironmentRevision :one
SELECT id, environment_id, number, paradigm_key, paradigm_version, apparatus
FROM misko.environment_revisions
WHERE id = @id;

-- CreateVersion numbers versions per protocol; callers lock the protocol or
-- create it in the same transaction.
-- name: CreateVersion :one
INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, notes, created_by)
SELECT @experiment_id::uuid, @protocol_id::uuid, coalesce(max(v.number), 0) + 1, @step_count::integer, sqlc.narg(notes)::text, @created_by::uuid
FROM misko.protocol_versions v
WHERE v.protocol_id = @protocol_id::uuid
RETURNING id, experiment_id, protocol_id, number, step_count, notes, created_by, created_at;

-- name: CreateStep :exec
INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, environment_revision_id, trial_type, trials, inter_trial_interval_s, session, notes)
VALUES (@protocol_version_id, @position, @paradigm_key, @paradigm_version, @environment_revision_id, @trial_type, @trials, @inter_trial_interval_s, @session, sqlc.narg(notes));

-- name: ListVersions :many
SELECT id, experiment_id, protocol_id, number, step_count, notes, created_by, created_at
FROM misko.protocol_versions
WHERE experiment_id = @experiment_id AND protocol_id = @protocol_id
ORDER BY number;

-- name: GetVersion :one
SELECT id, experiment_id, protocol_id, number, step_count, notes, created_by, created_at
FROM misko.protocol_versions
WHERE experiment_id = @experiment_id AND protocol_id = @protocol_id AND number = @number;

-- name: ListSteps :many
SELECT s.protocol_version_id, s.position, s.paradigm_key, s.paradigm_version, s.environment_revision_id,
       r.environment_id, r.number AS environment_revision, s.trial_type, s.trials, s.inter_trial_interval_s, s.session, s.notes
FROM misko.protocol_steps s
JOIN misko.environment_revisions r ON r.id = s.environment_revision_id
WHERE s.protocol_version_id = ANY(@version_ids::uuid[])
ORDER BY s.protocol_version_id, s.position;
