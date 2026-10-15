-- name: GetTestExists :one
SELECT EXISTS (SELECT 1 FROM misko.tests WHERE id = @id);

-- name: GetTestSubjectRef :one
SELECT s.id, s.test_id, s.environment_revision_id, s.paradigm_key, s.paradigm_version, e.parameters
FROM misko.test_subjects s
JOIN misko.environment_revisions e ON e.id = s.environment_revision_id
WHERE s.test_id = @test_id AND s.id = @id;

-- The row lock serializes the overrides of one test subject; test subjects are
-- never updated. NO KEY UPDATE leaves trials and runs that reference it unblocked.
-- name: LockTestSubjectRef :one
SELECT s.id, s.test_id, s.environment_revision_id, s.paradigm_key, s.paradigm_version, e.parameters
FROM misko.test_subjects s
JOIN misko.environment_revisions e ON e.id = s.environment_revision_id
WHERE s.test_id = @test_id AND s.id = @id
FOR NO KEY UPDATE OF s;

-- name: GetCurrentVideo :one
SELECT r.id, a.status
FROM misko.current_recordings r
JOIN misko.video_assets a ON a.id = r.video_asset_id
WHERE r.test_id = @test_id;

-- name: GetRevisionRef :one
SELECT id, environment_id, paradigm_key, paradigm_version, parameters
FROM misko.environment_revisions
WHERE environment_id = @environment_id AND number = @number;

-- The row lock serializes calibrations of one revision. NO KEY UPDATE does not
-- block test subjects that only reference the revision.
-- name: LockRevisionRef :one
SELECT id, environment_id, paradigm_key, paradigm_version, parameters
FROM misko.environment_revisions
WHERE environment_id = @environment_id AND number = @number
FOR NO KEY UPDATE;

-- name: ListOverrides :many
SELECT * FROM misko.calibrations WHERE test_subject_id = @test_subject_id ORDER BY created_at, id;

-- name: ListEnvironmentCalibrations :many
SELECT * FROM misko.calibrations
WHERE environment_revision_id = @environment_revision_id AND test_subject_id IS NULL
ORDER BY created_at, id;

-- name: GetCalibration :one
SELECT * FROM misko.calibrations WHERE id = @id;

-- The effective calibration of every animal of a test, with what deciding its
-- status needs: the arena's measurements and the current video.
-- name: ListEffective :many
SELECT s.id AS test_subject_id, s.environment_revision_id, s.paradigm_key, s.paradigm_version, e.parameters,
       r.id AS recording_id, a.status AS video_status,
       coalesce(ec.source, '')::text AS source, coalesce(ec.calibration_id::text, '')::text AS calibration_id
FROM misko.test_subjects s
JOIN misko.environment_revisions e ON e.id = s.environment_revision_id
LEFT JOIN misko.current_recordings r ON r.test_id = s.test_id
LEFT JOIN misko.video_assets a ON a.id = r.video_asset_id
LEFT JOIN misko.effective_calibrations ec ON ec.test_subject_id = s.id
WHERE s.test_id = @test_id
ORDER BY s.created_at, s.id;

-- name: CreateCalibration :one
INSERT INTO misko.calibrations (environment_revision_id, test_subject_id, recording_id, supersedes_id, camera_id, frame_width, frame_height,
    crop_x, crop_y, crop_width, crop_height, reference_frame_us, measurement_plane, fit_points, check_points, transform,
    fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, rejection_reason, created_by)
VALUES (@environment_revision_id, sqlc.narg(test_subject_id), sqlc.narg(recording_id), sqlc.narg(supersedes_id), @camera_id, @frame_width, @frame_height,
    @crop_x, @crop_y, @crop_width, @crop_height, sqlc.narg(reference_frame_us), @measurement_plane, @fit_points, @check_points, @transform,
    @fit_rms_error_cm, @check_rms_error_cm, @check_max_error_cm, @tolerance_cm, @algorithm_version, @status, sqlc.narg(rejection_reason), @created_by)
RETURNING *;
