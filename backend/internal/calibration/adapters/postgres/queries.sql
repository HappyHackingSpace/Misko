-- name: GetRecordingRef :one
SELECT r.id, r.test_id, a.status AS video_status, t.paradigm_key, t.paradigm_version, t.environment_revision_id, e.apparatus
FROM misko.test_recordings r
JOIN misko.video_assets a ON a.id = r.video_asset_id
JOIN misko.tests t ON t.id = r.test_id
JOIN misko.environment_revisions e ON e.id = t.environment_revision_id
WHERE r.test_id = @test_id AND r.id = @id;

-- The row lock serializes calibrations of one recording; recordings are never updated.
-- name: LockRecordingRef :one
SELECT r.id, r.test_id, a.status AS video_status, t.paradigm_key, t.paradigm_version, t.environment_revision_id, e.apparatus
FROM misko.test_recordings r
JOIN misko.video_assets a ON a.id = r.video_asset_id
JOIN misko.tests t ON t.id = r.test_id
JOIN misko.environment_revisions e ON e.id = t.environment_revision_id
WHERE r.test_id = @test_id AND r.id = @id
FOR UPDATE OF r;

-- name: GetRevisionRef :one
SELECT id, environment_id, paradigm_key, paradigm_version, apparatus
FROM misko.environment_revisions
WHERE environment_id = @environment_id AND number = @number;

-- The row lock serializes calibrations of one revision. NO KEY UPDATE does not
-- block tests that only reference the revision.
-- name: LockRevisionRef :one
SELECT id, environment_id, paradigm_key, paradigm_version, apparatus
FROM misko.environment_revisions
WHERE environment_id = @environment_id AND number = @number
FOR NO KEY UPDATE;

-- name: ListCalibrations :many
SELECT * FROM misko.calibrations WHERE recording_id = @recording_id ORDER BY created_at, id;

-- name: ListEnvironmentCalibrations :many
SELECT * FROM misko.calibrations
WHERE environment_revision_id = @environment_revision_id AND recording_id IS NULL
ORDER BY created_at, id;

-- name: CreateCalibration :one
INSERT INTO misko.calibrations (recording_id, environment_revision_id, supersedes_id, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height,
    reference_frame_us, measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm,
    tolerance_cm, algorithm_version, status, rejection_reason, created_by)
VALUES (sqlc.narg(recording_id), @environment_revision_id, sqlc.narg(supersedes_id), @camera_id, @frame_width, @frame_height, @crop_x, @crop_y, @crop_width, @crop_height,
    sqlc.narg(reference_frame_us), @measurement_plane, @fit_points, @check_points, @transform, @fit_rms_error_cm, @check_rms_error_cm, @check_max_error_cm,
    @tolerance_cm, @algorithm_version, @status, sqlc.narg(rejection_reason), @created_by)
RETURNING *;
