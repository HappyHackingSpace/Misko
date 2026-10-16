-- name: GetTestRef :one
SELECT id, experiment_id, status FROM misko.tests WHERE id = @id;

-- The row lock serializes videos attached to one test, so each new recording
-- supersedes the one that was current. NO KEY UPDATE leaves rows that only
-- reference the test unblocked.
-- name: LockTestRef :one
SELECT id, experiment_id, status FROM misko.tests WHERE id = @id FOR NO KEY UPDATE;

-- CreateAsset names the object after its test and its own id, so every upload
-- gets a new object key.
-- name: CreateAsset :one
WITH new_asset AS (SELECT uuidv7() AS id)
INSERT INTO misko.video_assets (id, kind, bucket, object_name, content_type, file_name, size_bytes, crc32c, created_by)
SELECT n.id, 'ORIGINAL', @bucket::text, 'tests/' || @test_id::text || '/originals/' || n.id::text,
       @content_type::text, sqlc.narg(file_name)::text, @size_bytes::bigint, @crc32c::bigint, @created_by::uuid
FROM new_asset n
RETURNING *;

-- name: CreateRecording :one
INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, supersedes_id, clip_start_us, clip_end_us, created_by)
VALUES (@experiment_id, @test_id, @video_asset_id, sqlc.narg(supersedes_id), @clip_start_us, sqlc.narg(clip_end_us), @created_by)
RETURNING *;

-- The end of the test's chain, verified or not: every new upload supersedes it.
-- name: GetNewestRecordingID :one
SELECT r.id FROM misko.test_recordings r
WHERE r.test_id = @test_id AND NOT EXISTS (SELECT 1 FROM misko.test_recordings n WHERE n.supersedes_id = r.id);

-- name: ListRecordings :many
SELECT r.id, r.experiment_id, r.test_id, r.supersedes_id, r.clip_start_us, r.clip_end_us, r.created_by, r.created_at,
       EXISTS (SELECT 1 FROM misko.current_recordings c WHERE c.id = r.id) AS current,
       a.id AS asset_id, a.kind, a.bucket, a.object_name, a.content_type, a.file_name, a.size_bytes, a.crc32c,
       a.status, a.generation, a.rejection_reason, a.verified_at, a.created_by AS asset_created_by, a.created_at AS asset_created_at
FROM misko.test_recordings r
JOIN misko.video_assets a ON a.id = r.video_asset_id
WHERE r.test_id = @test_id
ORDER BY r.created_at, r.id;

-- name: GetCurrentRecording :one
SELECT r.id, r.experiment_id, r.test_id, r.supersedes_id, r.clip_start_us, r.clip_end_us, r.created_by, r.created_at,
       true AS current,
       a.id AS asset_id, a.kind, a.bucket, a.object_name, a.content_type, a.file_name, a.size_bytes, a.crc32c,
       a.status, a.generation, a.rejection_reason, a.verified_at, a.created_by AS asset_created_by, a.created_at AS asset_created_at
FROM misko.current_recordings r
JOIN misko.video_assets a ON a.id = r.video_asset_id
WHERE r.test_id = @test_id;

-- name: GetRecording :one
SELECT r.id, r.experiment_id, r.test_id, r.supersedes_id, r.clip_start_us, r.clip_end_us, r.created_by, r.created_at,
       EXISTS (SELECT 1 FROM misko.current_recordings c WHERE c.id = r.id) AS current,
       a.id AS asset_id, a.kind, a.bucket, a.object_name, a.content_type, a.file_name, a.size_bytes, a.crc32c,
       a.status, a.generation, a.rejection_reason, a.verified_at, a.created_by AS asset_created_by, a.created_at AS asset_created_at
FROM misko.test_recordings r
JOIN misko.video_assets a ON a.id = r.video_asset_id
WHERE r.test_id = @test_id AND r.id = @id;

-- name: LockRecording :one
SELECT r.id, r.experiment_id, r.test_id, r.supersedes_id, r.clip_start_us, r.clip_end_us, r.created_by, r.created_at,
       EXISTS (SELECT 1 FROM misko.current_recordings c WHERE c.id = r.id) AS current,
       a.id AS asset_id, a.kind, a.bucket, a.object_name, a.content_type, a.file_name, a.size_bytes, a.crc32c,
       a.status, a.generation, a.rejection_reason, a.verified_at, a.created_by AS asset_created_by, a.created_at AS asset_created_at
FROM misko.test_recordings r
JOIN misko.video_assets a ON a.id = r.video_asset_id
WHERE r.test_id = @test_id AND r.id = @id
FOR UPDATE OF a;

-- name: FinishAsset :one
UPDATE misko.video_assets
SET status = @status, generation = @generation, rejection_reason = sqlc.narg(rejection_reason), verified_at = sqlc.narg(verified_at)
WHERE id = @id AND status = 'PENDING'
RETURNING *;
