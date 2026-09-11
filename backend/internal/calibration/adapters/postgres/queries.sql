-- name: GetRecordingRef :one
SELECT r.id, r.test_id, a.status AS video_status, t.paradigm_key, t.paradigm_version, e.apparatus
FROM misko.test_recordings r
JOIN misko.video_assets a ON a.id = r.video_asset_id
JOIN misko.tests t ON t.id = r.test_id
JOIN misko.environment_revisions e ON e.id = t.environment_revision_id
WHERE r.test_id = @test_id AND r.id = @id;

-- The row lock serializes calibrations of one recording; recordings are never updated.
-- name: LockRecordingRef :one
SELECT r.id, r.test_id, a.status AS video_status, t.paradigm_key, t.paradigm_version, e.apparatus
FROM misko.test_recordings r
JOIN misko.video_assets a ON a.id = r.video_asset_id
JOIN misko.tests t ON t.id = r.test_id
JOIN misko.environment_revisions e ON e.id = t.environment_revision_id
WHERE r.test_id = @test_id AND r.id = @id
FOR UPDATE OF r;

-- name: ListCalibrations :many
SELECT * FROM misko.calibrations WHERE recording_id = @recording_id ORDER BY created_at, id;

-- name: CreateCalibration :one
INSERT INTO misko.calibrations (recording_id, supersedes_id, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height,
    reference_frame_us, measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm,
    tolerance_cm, algorithm_version, status, rejection_reason, created_by)
VALUES (@recording_id, sqlc.narg(supersedes_id), @camera_id, @frame_width, @frame_height, @crop_x, @crop_y, @crop_width, @crop_height,
    @reference_frame_us, @measurement_plane, @fit_points, @check_points, @transform, @fit_rms_error_cm, @check_rms_error_cm, @check_max_error_cm,
    @tolerance_cm, @algorithm_version, @status, sqlc.narg(rejection_reason), @created_by)
RETURNING *;
