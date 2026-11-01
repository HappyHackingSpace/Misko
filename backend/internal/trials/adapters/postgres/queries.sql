-- Paradigm trials own paradigm_trials and paradigm_trial_jobs. The list reads
-- the arena's name from the environments, and the candidates read their
-- video's status, both as read models; analysis jobs are never joined.

-- name: CreateTrial :one
INSERT INTO misko.paradigm_trials (name, paradigm_key, paradigm_version, environment_revision_id, notes, created_by)
VALUES (@name, @paradigm_key, @paradigm_version, @environment_revision_id, sqlc.narg(notes), @created_by)
RETURNING *;

-- name: GetTrial :one
SELECT * FROM misko.paradigm_trials WHERE id = @id AND deleted_at IS NULL;

-- name: LockTrial :one
SELECT * FROM misko.paradigm_trials WHERE id = @id AND deleted_at IS NULL FOR UPDATE;

-- name: SetVideo :one
UPDATE misko.paradigm_trials
SET video_asset_id = @video_asset_id, clip_start_us = @clip_start_us, clip_end_us = sqlc.narg(clip_end_us)
WHERE id = @id
RETURNING *;

-- name: DeleteTrial :exec
UPDATE misko.paradigm_trials SET deleted_at = @deleted_at WHERE id = @id;

-- name: ListTrials :many
SELECT t.*, er.environment_id, env.name AS environment_name, er.number AS revision_number
FROM misko.paradigm_trials t
JOIN misko.environment_revisions er ON er.id = t.environment_revision_id
JOIN misko.environments env ON env.id = er.environment_id
WHERE t.deleted_at IS NULL
  AND (@search::text = '' OR strpos(lower(t.name), lower(@search::text)) > 0)
  AND (@paradigm_key::text = '' OR t.paradigm_key = @paradigm_key::text)
ORDER BY t.created_at DESC, t.id DESC
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountTrials :one
SELECT count(*) FROM misko.paradigm_trials t
WHERE t.deleted_at IS NULL
  AND (@search::text = '' OR strpos(lower(t.name), lower(@search::text)) > 0)
  AND (@paradigm_key::text = '' OR t.paradigm_key = @paradigm_key::text);

-- Trials whose verified current video has no automatic analysis yet.
-- name: Candidates :many
SELECT t.* FROM misko.paradigm_trials t
JOIN misko.video_assets a ON a.id = t.video_asset_id
WHERE t.deleted_at IS NULL AND a.status = 'VERIFIED'
  AND NOT EXISTS (SELECT 1 FROM misko.paradigm_trial_jobs j
                  WHERE j.trial_id = t.id AND j.video_asset_id = t.video_asset_id AND j.trigger = 'AUTOMATIC')
ORDER BY t.updated_at DESC, t.id
LIMIT @row_limit::integer;

-- name: CreateLink :execrows
INSERT INTO misko.paradigm_trial_jobs (job_id, trial_id, video_asset_id, calibration_id, trigger, overrides, created_by)
VALUES (@job_id, @trial_id, @video_asset_id, sqlc.narg(calibration_id), @trigger, @overrides, sqlc.narg(created_by))
ON CONFLICT DO NOTHING;

-- name: ListLinks :many
SELECT * FROM misko.paradigm_trial_jobs WHERE trial_id = @trial_id ORDER BY created_at DESC, job_id DESC;
