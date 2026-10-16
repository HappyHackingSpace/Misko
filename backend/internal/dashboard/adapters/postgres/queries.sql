-- Read-only aggregation across the experiments, tests, media, calibration and
-- analysis domains, same standing as the reports package: these queries never
-- write.

-- name: ExperimentStatusCounts :many
SELECT status, count(*) AS total FROM misko.experiments GROUP BY status;

-- name: TestStatusCounts :many
SELECT status, count(*) AS total FROM misko.tests GROUP BY status;

-- name: AnalysisStatusCounts :many
SELECT status, count(*) AS total FROM misko.analysis_runs
WHERE status IN ('QUEUED', 'RUNNING', 'FAILED')
GROUP BY status;

-- name: ResultStatusCounts :many
SELECT status, count(*) AS total FROM misko.subject_results GROUP BY status;

-- name: RecentSucceededRuns :many
SELECT r.id, r.finished_at, r.paradigm_key, r.paradigm_version, r.test_id, r.test_subject_id,
       s.code AS subject_code, e.code AS experiment_code
FROM misko.analysis_runs r
JOIN misko.test_subjects ts ON ts.id = r.test_subject_id
JOIN misko.allocations a ON a.id = ts.allocation_id
JOIN misko.subjects s ON s.id = a.subject_id
JOIN misko.experiments e ON e.id = r.experiment_id
WHERE r.status = 'SUCCEEDED'
ORDER BY r.finished_at DESC, r.id DESC
LIMIT @row_limit::integer;

-- name: UpcomingTests :many
SELECT t.id, t.scheduled_at, t.paradigm_key, t.paradigm_version, e.code AS experiment_code, tp.name AS timepoint_name,
       coalesce(array_agg(s.code ORDER BY lower(s.code)) FILTER (WHERE s.code IS NOT NULL), '{}')::text[] AS subject_codes
FROM misko.tests t
JOIN misko.experiments e ON e.id = t.experiment_id
JOIN misko.timepoints tp ON tp.id = t.timepoint_id
LEFT JOIN misko.test_subjects ts ON ts.test_id = t.id
LEFT JOIN misko.allocations a ON a.id = ts.allocation_id
LEFT JOIN misko.subjects s ON s.id = a.subject_id
WHERE t.status = 'PLANNED'
GROUP BY t.id, e.code, tp.name
ORDER BY t.scheduled_at ASC, t.id
LIMIT @row_limit::integer;

-- Every animal of a test that is not cancelled, whose current video is
-- verified, that is not excluded and whose arena has no valid effective
-- calibration (neither its own override for this video nor the arena default).
-- Whether the paradigm actually requires one is a rule of the paradigm catalog,
-- so it is decided in Go from this candidate set, the same way the scheduler
-- and the calibration context decide it.
-- name: TestSubjectsAwaitingCalibrationCheck :many
SELECT ts.id AS test_subject_id, t.paradigm_key, t.paradigm_version, er.parameters
FROM misko.test_subjects ts
JOIN misko.effective_calibrations ec ON ec.test_subject_id = ts.id
JOIN misko.test_recordings r ON r.id = ec.recording_id
JOIN misko.video_assets a ON a.id = r.video_asset_id
JOIN misko.tests t ON t.id = ts.test_id
JOIN misko.allocations al ON al.id = ts.allocation_id
JOIN misko.environment_revisions er ON er.id = ts.environment_revision_id
WHERE a.status = 'VERIFIED' AND ec.calibration_id IS NULL AND t.status <> 'CANCELLED' AND al.excluded_at IS NULL;
