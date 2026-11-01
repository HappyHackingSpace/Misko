-- The experiment side of analysis owns test_subject_analyses and
-- subject_results. It reads the study tables to decide which animals are
-- ready, and the analysis jobs only through the test_subject_jobs view.

-- ReadyTestSubjects lists the animals the orchestrator can analyse now: the
-- test's current video is verified, the test is not cancelled, the experiment
-- runs or completed, the animal is not excluded and, when the paradigm needs
-- a calibration (calibrated, "KEY@version"), the animal's arena has a valid
-- effective one. Only animals without an automatic job for the same video and
-- calibration are returned, so the oldest rows never starve newer ones. An
-- arena (environment) calibration never triggers a job for an animal that
-- already has an automatic job of this video: correcting the default must not
-- silently reanalyse every animal, that stays an explicit manual job.
-- name: ReadyTestSubjects :many
WITH candidates AS (
    SELECT ts.experiment_id, ts.test_id, ts.id AS test_subject_id, r.id AS recording_id, a.id AS source_asset_id,
           r.clip_start_us, r.clip_end_us, t.paradigm_key, t.paradigm_version, er.parameters, ps.session, r.created_at,
           coalesce(ec.calibration_id::text, '')::text AS calibration_id, coalesce(ec.source, '')::text AS calibration_source,
           ARRAY(SELECT DISTINCT z.zone_key FROM misko.calibration_zones z WHERE z.calibration_id = ec.calibration_id)::text[] AS zone_keys
    FROM misko.test_subjects ts
    JOIN misko.effective_calibrations ec ON ec.test_subject_id = ts.id
    JOIN misko.test_recordings r ON r.id = ec.recording_id
    JOIN misko.video_assets a ON a.id = r.video_asset_id
    JOIN misko.tests t ON t.id = ts.test_id
    JOIN misko.experiments e ON e.id = ts.experiment_id
    JOIN misko.allocations al ON al.id = ts.allocation_id
    JOIN misko.environment_revisions er ON er.id = ts.environment_revision_id
    JOIN misko.protocol_steps ps ON ps.protocol_version_id = t.protocol_version_id AND ps.position = t.step_position
    WHERE a.status = 'VERIFIED' AND t.status <> 'CANCELLED' AND e.status IN ('RUNNING', 'COMPLETED') AND al.excluded_at IS NULL
      AND (ec.calibration_id IS NOT NULL OR NOT (t.paradigm_key || '@' || t.paradigm_version::text) = ANY(@calibrated::text[]))
)
SELECT c.experiment_id, c.test_id, c.test_subject_id, c.recording_id, c.source_asset_id, c.clip_start_us, c.clip_end_us,
       c.paradigm_key, c.paradigm_version, c.parameters, c.session, c.calibration_id, c.zone_keys
FROM candidates c
WHERE NOT EXISTS (
    SELECT 1 FROM misko.test_subject_analyses x
    WHERE x.trigger = 'AUTOMATIC' AND x.test_subject_id = c.test_subject_id AND x.recording_id = c.recording_id
      AND (c.calibration_source = 'ENVIRONMENT' OR coalesce(x.calibration_id::text, '') IN (c.calibration_id, '')))
ORDER BY c.created_at, c.test_subject_id
LIMIT 500;

-- GetCandidate reads one animal of a test with its current video, if any, and
-- everything that decides whether it can be analysed.
-- name: GetCandidate :one
SELECT ts.experiment_id, ts.test_id, ts.id AS test_subject_id, coalesce(r.id::text, '')::text AS recording_id,
       coalesce(a.id::text, '')::text AS source_asset_id, coalesce(r.clip_start_us, 0)::bigint AS clip_start_us, r.clip_end_us,
       t.paradigm_key, t.paradigm_version, er.parameters, ps.session, coalesce(a.status, '')::text AS video_status,
       t.status AS test_status, e.status AS experiment_status, (al.excluded_at IS NOT NULL)::boolean AS excluded,
       coalesce(ec.calibration_id::text, '')::text AS calibration_id,
       ARRAY(SELECT DISTINCT z.zone_key FROM misko.calibration_zones z WHERE z.calibration_id = ec.calibration_id)::text[] AS zone_keys
FROM misko.test_subjects ts
JOIN misko.tests t ON t.id = ts.test_id
JOIN misko.experiments e ON e.id = ts.experiment_id
JOIN misko.allocations al ON al.id = ts.allocation_id
JOIN misko.environment_revisions er ON er.id = ts.environment_revision_id
JOIN misko.protocol_steps ps ON ps.protocol_version_id = t.protocol_version_id AND ps.position = t.step_position
LEFT JOIN misko.effective_calibrations ec ON ec.test_subject_id = ts.id
LEFT JOIN misko.test_recordings r ON r.id = ec.recording_id
LEFT JOIN misko.video_assets a ON a.id = r.video_asset_id
WHERE ts.id = @id;

-- LockTestSubjectAnalyses serializes manual job submission for one animal of
-- a test until the transaction ends, so two requests cannot both see no
-- pending job.
-- name: LockTestSubjectAnalyses :exec
SELECT pg_advisory_xact_lock(hashtextextended('test-subject-analyses:' || @test_subject_id::text, 0));

-- name: CreateLink :execrows
INSERT INTO misko.test_subject_analyses (job_id, experiment_id, test_id, test_subject_id, recording_id, calibration_id, trigger, created_by)
VALUES (@job_id, @experiment_id, @test_id, @test_subject_id, @recording_id, sqlc.narg(calibration_id), @trigger, sqlc.narg(created_by))
ON CONFLICT DO NOTHING;

-- name: GetLink :one
SELECT * FROM misko.test_subject_analyses WHERE job_id = @job_id;

-- name: ListLinks :many
SELECT * FROM misko.test_subject_analyses
WHERE test_subject_id = ANY(@test_subject_ids::uuid[])
ORDER BY created_at DESC, job_id DESC;

-- name: ListTestLinks :many
SELECT * FROM misko.test_subject_analyses
WHERE test_id = @test_id AND (sqlc.narg(test_subject_id)::uuid IS NULL OR test_subject_id = sqlc.narg(test_subject_id)::uuid)
ORDER BY created_at DESC, job_id DESC;

-- Unopened lists animals whose first succeeded job has no result yet, with
-- that job.
-- name: Unopened :many
SELECT DISTINCT ON (j.test_subject_id) j.test_subject_id, j.job_id
FROM misko.test_subject_jobs j
WHERE j.status = 'SUCCEEDED' AND NOT EXISTS (SELECT 1 FROM misko.subject_results sr WHERE sr.test_subject_id = j.test_subject_id)
ORDER BY j.test_subject_id, j.finished_at, j.job_id
LIMIT @row_limit::integer;

-- OpenResult creates the PENDING result of an animal on its first successful
-- job; later jobs leave the reviewed choice alone.
-- name: OpenResult :exec
INSERT INTO misko.subject_results (test_subject_id, job_id)
VALUES (@test_subject_id, @job_id)
ON CONFLICT (test_subject_id) DO NOTHING;

-- LockResult locks a result for review and its experiment's status against a
-- concurrent completion.
-- name: LockResult :one
SELECT sr.test_subject_id, sr.job_id, sr.status, sr.reason, sr.note, sr.reviewed_by, sr.reviewed_at, sr.created_at, sr.updated_at,
       misko.experiment_status(ts.experiment_id)::text AS experiment_status
FROM misko.subject_results sr
JOIN misko.test_subjects ts ON ts.id = sr.test_subject_id
WHERE sr.test_subject_id = @test_subject_id
FOR UPDATE OF sr;

-- name: UpdateResult :one
UPDATE misko.subject_results
SET job_id = @job_id, status = @status, reason = sqlc.narg(reason), note = sqlc.narg(note),
    reviewed_by = sqlc.narg(reviewed_by), reviewed_at = sqlc.narg(reviewed_at)
WHERE test_subject_id = @test_subject_id
RETURNING *;

-- ReviewItems lists results with what a reviewer sees: the animal, its arena,
-- the test and the link of the job under review. Group, disease and treatment
-- columns are read here and blanked by the application for blinded
-- experiments; the job's own facts come from the analysis service.
-- name: ReviewItems :many
SELECT sr.test_subject_id, sr.job_id, sr.status, sr.reason, sr.note, sr.reviewed_by, sr.reviewed_at, sr.created_at, sr.updated_at,
       ts.test_id, ts.experiment_id, e.code AS experiment_code, e.blinded, e.status AS experiment_status,
       s.code AS subject_code, (al.excluded_at IS NOT NULL)::boolean AS excluded,
       ts.environment_revision_id, env.id AS environment_id, env.name AS environment_name, er.number AS environment_revision,
       tp.id AS timepoint_id, tp.name AS timepoint_name, t.paradigm_key, t.paradigm_version, t.scheduled_at,
       g.id AS group_id, g.code AS group_code, g.name AS group_name, g.role AS group_role, dm.name AS disease_model_name,
       sub.name AS treatment_substance_name, tr.dose_micro AS treatment_dose_micro, tr.unit AS treatment_unit,
       l.trigger, l.calibration_id, l.recording_id
FROM misko.subject_results sr
JOIN misko.test_subject_analyses l ON l.job_id = sr.job_id
JOIN misko.test_subjects ts ON ts.id = sr.test_subject_id
JOIN misko.tests t ON t.id = ts.test_id
JOIN misko.experiments e ON e.id = ts.experiment_id
JOIN misko.allocations al ON al.id = ts.allocation_id
JOIN misko.subjects s ON s.id = al.subject_id
JOIN misko.experiment_groups g ON g.id = al.group_id
LEFT JOIN misko.disease_models dm ON dm.id = g.disease_model_id
LEFT JOIN misko.regimen_items tr ON tr.group_id = g.id AND tr.purpose = 'TREATMENT'
LEFT JOIN misko.substances sub ON sub.id = tr.substance_id
JOIN misko.timepoints tp ON tp.id = t.timepoint_id
JOIN misko.environment_revisions er ON er.id = ts.environment_revision_id
JOIN misko.environments env ON env.id = er.environment_id
WHERE (sqlc.narg(experiment_id)::uuid IS NULL OR ts.experiment_id = sqlc.narg(experiment_id)::uuid)
  AND (sqlc.narg(test_subject_id)::uuid IS NULL OR sr.test_subject_id = sqlc.narg(test_subject_id)::uuid)
  AND (@status::text = '' OR sr.status = @status::text)
ORDER BY sr.created_at, sr.test_subject_id
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountReviewItems :one
SELECT count(*)
FROM misko.subject_results sr
JOIN misko.test_subjects ts ON ts.id = sr.test_subject_id
WHERE (sqlc.narg(experiment_id)::uuid IS NULL OR ts.experiment_id = sqlc.narg(experiment_id)::uuid)
  AND (@status::text = '' OR sr.status = @status::text);
