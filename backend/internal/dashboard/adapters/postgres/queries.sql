-- Read-only aggregation across the tests, analysis and calibration domains,
-- same standing as the reports package: these queries never write.

-- name: TestStatusCounts :many
SELECT status, count(*) AS total FROM misko.tests GROUP BY status;

-- name: AnalysisStatusCounts :many
SELECT status, count(*) AS total FROM misko.analysis_runs
WHERE status IN ('QUEUED', 'RUNNING', 'FAILED')
GROUP BY status;

-- name: RecentSucceededRuns :many
SELECT r.id, r.finished_at, r.paradigm_key, r.paradigm_version, s.code AS subject_code, e.code AS experiment_code, t.id AS test_id
FROM misko.analysis_runs r
JOIN misko.tests t ON t.id = r.test_id
JOIN misko.subjects s ON s.id = t.subject_id
JOIN misko.experiments e ON e.id = t.experiment_id
WHERE r.status = 'SUCCEEDED'
ORDER BY r.finished_at DESC, r.id DESC
LIMIT @row_limit::integer;

-- name: UpcomingTests :many
SELECT t.id, t.scheduled_at, t.paradigm_key, t.paradigm_version, s.code AS subject_code, e.code AS experiment_code
FROM misko.tests t
JOIN misko.subjects s ON s.id = t.subject_id
JOIN misko.experiments e ON e.id = t.experiment_id
WHERE t.status = 'PLANNED'
ORDER BY t.scheduled_at ASC, t.id
LIMIT @row_limit::integer;

-- name: RecordingsAwaitingCalibrationCheck :many
-- Every verified recording without an effective calibration: neither its own
-- latest calibration nor, when it has none, its environment default is VALID. Whether the paradigm
-- actually requires calibration for it is a rule of the paradigm catalog, not of
-- the database, so it is decided in Go from this candidate set (mirrors the single-recording calibration status check).
SELECT r.id AS recording_id, t.paradigm_key, t.paradigm_version, er.apparatus
FROM misko.test_recordings r
JOIN misko.video_assets a ON a.id = r.video_asset_id
JOIN misko.tests t ON t.id = r.test_id
JOIN misko.environment_revisions er ON er.id = t.environment_revision_id
JOIN misko.effective_calibrations ec ON ec.recording_id = r.id
WHERE a.status = 'VERIFIED' AND ec.calibration_id IS NULL;
