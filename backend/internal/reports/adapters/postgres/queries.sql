-- Every filter is optional: NULL ids, empty keys and zero versions match all
-- rows. The sort key is allowlisted by the application; the trailing keys make
-- paging stable.

-- name: MetricRows :many
SELECT sqlc.embed(r), sqlc.embed(t),
       m.metric_key, m.unit, m.value, m.missing_reason
FROM misko.metric_results m
JOIN misko.report_runs r ON r.id = m.run_id
JOIN misko.report_tests t ON t.test_id = r.test_id
WHERE (NOT @latest_only::boolean OR r.latest)
  AND (sqlc.narg(experiment_id)::uuid IS NULL OR t.experiment_id = sqlc.narg(experiment_id)::uuid)
  AND (sqlc.narg(subject_id)::uuid IS NULL OR t.subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(group_id)::uuid IS NULL OR t.group_id = sqlc.narg(group_id)::uuid)
  AND (sqlc.narg(phase_id)::uuid IS NULL OR t.phase_id = sqlc.narg(phase_id)::uuid)
  AND (sqlc.narg(test_id)::uuid IS NULL OR t.test_id = sqlc.narg(test_id)::uuid)
  AND (sqlc.narg(environment_id)::uuid IS NULL OR t.environment_revision_id IN (
        SELECT er.id FROM misko.environment_revisions er WHERE er.environment_id = sqlc.narg(environment_id)::uuid))
  AND (sqlc.narg(video_id)::uuid IS NULL OR r.source_asset_id = sqlc.narg(video_id)::uuid)
  AND (sqlc.narg(run_id)::uuid IS NULL OR r.id = sqlc.narg(run_id)::uuid)
  AND (@paradigm_key::text = '' OR t.paradigm_key = @paradigm_key::text)
  AND (@paradigm_version::integer = 0 OR t.paradigm_version = @paradigm_version::integer)
  AND (@metric_engine_version::integer = 0 OR r.metric_engine_version = @metric_engine_version::integer)
  AND (@metric_key::text = '' OR m.metric_key = @metric_key::text)
  AND (sqlc.narg(disease_model_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM misko.subject_conditions c WHERE c.subject_id = t.subject_id AND c.disease_model_id = sqlc.narg(disease_model_id)::uuid))
  AND (sqlc.narg(substance_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM misko.administrations a
        WHERE a.subject_id = t.subject_id AND a.experiment_id = t.experiment_id AND a.substance_id = sqlc.narg(substance_id)::uuid))
ORDER BY
    CASE WHEN @sort_key::text = 'scheduledAt' AND NOT @descending::boolean THEN t.scheduled_at END ASC,
    CASE WHEN @sort_key::text = 'scheduledAt' AND @descending::boolean THEN t.scheduled_at END DESC,
    CASE WHEN @sort_key::text = 'subjectCode' AND NOT @descending::boolean THEN lower(t.subject_code) END ASC,
    CASE WHEN @sort_key::text = 'subjectCode' AND @descending::boolean THEN lower(t.subject_code) END DESC,
    CASE WHEN @sort_key::text = 'experimentCode' AND NOT @descending::boolean THEN lower(t.experiment_code) END ASC,
    CASE WHEN @sort_key::text = 'experimentCode' AND @descending::boolean THEN lower(t.experiment_code) END DESC,
    CASE WHEN @sort_key::text = 'metricKey' AND NOT @descending::boolean THEN m.metric_key END ASC,
    CASE WHEN @sort_key::text = 'metricKey' AND @descending::boolean THEN m.metric_key END DESC,
    CASE WHEN @sort_key::text = 'value' AND NOT @descending::boolean THEN m.value END ASC NULLS LAST,
    CASE WHEN @sort_key::text = 'value' AND @descending::boolean THEN m.value END DESC NULLS LAST,
    CASE WHEN @sort_key::text = 'runFinishedAt' AND NOT @descending::boolean THEN r.finished_at END ASC,
    CASE WHEN @sort_key::text = 'runFinishedAt' AND @descending::boolean THEN r.finished_at END DESC,
    t.test_id, r.id, m.metric_key
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountMetricRows :one
SELECT count(*)
FROM misko.metric_results m
JOIN misko.report_runs r ON r.id = m.run_id
JOIN misko.report_tests t ON t.test_id = r.test_id
WHERE (NOT @latest_only::boolean OR r.latest)
  AND (sqlc.narg(experiment_id)::uuid IS NULL OR t.experiment_id = sqlc.narg(experiment_id)::uuid)
  AND (sqlc.narg(subject_id)::uuid IS NULL OR t.subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(group_id)::uuid IS NULL OR t.group_id = sqlc.narg(group_id)::uuid)
  AND (sqlc.narg(phase_id)::uuid IS NULL OR t.phase_id = sqlc.narg(phase_id)::uuid)
  AND (sqlc.narg(test_id)::uuid IS NULL OR t.test_id = sqlc.narg(test_id)::uuid)
  AND (sqlc.narg(environment_id)::uuid IS NULL OR t.environment_revision_id IN (
        SELECT er.id FROM misko.environment_revisions er WHERE er.environment_id = sqlc.narg(environment_id)::uuid))
  AND (sqlc.narg(video_id)::uuid IS NULL OR r.source_asset_id = sqlc.narg(video_id)::uuid)
  AND (sqlc.narg(run_id)::uuid IS NULL OR r.id = sqlc.narg(run_id)::uuid)
  AND (@paradigm_key::text = '' OR t.paradigm_key = @paradigm_key::text)
  AND (@paradigm_version::integer = 0 OR t.paradigm_version = @paradigm_version::integer)
  AND (@metric_engine_version::integer = 0 OR r.metric_engine_version = @metric_engine_version::integer)
  AND (@metric_key::text = '' OR m.metric_key = @metric_key::text)
  AND (sqlc.narg(disease_model_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM misko.subject_conditions c WHERE c.subject_id = t.subject_id AND c.disease_model_id = sqlc.narg(disease_model_id)::uuid))
  AND (sqlc.narg(substance_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM misko.administrations a
        WHERE a.subject_id = t.subject_id AND a.experiment_id = t.experiment_id AND a.substance_id = sqlc.narg(substance_id)::uuid));

-- name: EventRows :many
SELECT sqlc.embed(r), sqlc.embed(t),
       ev.id AS event_id, ev.event_type, ev.kind, ev.start_us, ev.end_us, ev.confidence,
       ev.trial_id, tr.number AS trial_number, tr.repetition AS trial_repetition, tr.attempt AS trial_attempt
FROM misko.analysis_events ev
JOIN misko.report_runs r ON r.id = ev.run_id
JOIN misko.report_tests t ON t.test_id = r.test_id
LEFT JOIN misko.trials tr ON tr.id = ev.trial_id
WHERE (NOT @latest_only::boolean OR r.latest)
  AND (sqlc.narg(experiment_id)::uuid IS NULL OR t.experiment_id = sqlc.narg(experiment_id)::uuid)
  AND (sqlc.narg(subject_id)::uuid IS NULL OR t.subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(group_id)::uuid IS NULL OR t.group_id = sqlc.narg(group_id)::uuid)
  AND (sqlc.narg(phase_id)::uuid IS NULL OR t.phase_id = sqlc.narg(phase_id)::uuid)
  AND (sqlc.narg(test_id)::uuid IS NULL OR t.test_id = sqlc.narg(test_id)::uuid)
  AND (sqlc.narg(environment_id)::uuid IS NULL OR t.environment_revision_id IN (
        SELECT er.id FROM misko.environment_revisions er WHERE er.environment_id = sqlc.narg(environment_id)::uuid))
  AND (sqlc.narg(video_id)::uuid IS NULL OR r.source_asset_id = sqlc.narg(video_id)::uuid)
  AND (sqlc.narg(run_id)::uuid IS NULL OR r.id = sqlc.narg(run_id)::uuid)
  AND (@paradigm_key::text = '' OR t.paradigm_key = @paradigm_key::text)
  AND (@paradigm_version::integer = 0 OR t.paradigm_version = @paradigm_version::integer)
  AND (@metric_engine_version::integer = 0 OR r.metric_engine_version = @metric_engine_version::integer)
  AND (@event_type::text = '' OR ev.event_type = @event_type::text)
  AND (sqlc.narg(disease_model_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM misko.subject_conditions c WHERE c.subject_id = t.subject_id AND c.disease_model_id = sqlc.narg(disease_model_id)::uuid))
  AND (sqlc.narg(substance_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM misko.administrations a
        WHERE a.subject_id = t.subject_id AND a.experiment_id = t.experiment_id AND a.substance_id = sqlc.narg(substance_id)::uuid))
ORDER BY
    CASE WHEN @sort_key::text = 'scheduledAt' AND NOT @descending::boolean THEN t.scheduled_at END ASC,
    CASE WHEN @sort_key::text = 'scheduledAt' AND @descending::boolean THEN t.scheduled_at END DESC,
    CASE WHEN @sort_key::text = 'subjectCode' AND NOT @descending::boolean THEN lower(t.subject_code) END ASC,
    CASE WHEN @sort_key::text = 'subjectCode' AND @descending::boolean THEN lower(t.subject_code) END DESC,
    CASE WHEN @sort_key::text = 'eventType' AND NOT @descending::boolean THEN ev.event_type END ASC,
    CASE WHEN @sort_key::text = 'eventType' AND @descending::boolean THEN ev.event_type END DESC,
    t.test_id, r.id, ev.start_us, ev.id
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountEventRows :one
SELECT count(*)
FROM misko.analysis_events ev
JOIN misko.report_runs r ON r.id = ev.run_id
JOIN misko.report_tests t ON t.test_id = r.test_id
WHERE (NOT @latest_only::boolean OR r.latest)
  AND (sqlc.narg(experiment_id)::uuid IS NULL OR t.experiment_id = sqlc.narg(experiment_id)::uuid)
  AND (sqlc.narg(subject_id)::uuid IS NULL OR t.subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(group_id)::uuid IS NULL OR t.group_id = sqlc.narg(group_id)::uuid)
  AND (sqlc.narg(phase_id)::uuid IS NULL OR t.phase_id = sqlc.narg(phase_id)::uuid)
  AND (sqlc.narg(test_id)::uuid IS NULL OR t.test_id = sqlc.narg(test_id)::uuid)
  AND (sqlc.narg(environment_id)::uuid IS NULL OR t.environment_revision_id IN (
        SELECT er.id FROM misko.environment_revisions er WHERE er.environment_id = sqlc.narg(environment_id)::uuid))
  AND (sqlc.narg(video_id)::uuid IS NULL OR r.source_asset_id = sqlc.narg(video_id)::uuid)
  AND (sqlc.narg(run_id)::uuid IS NULL OR r.id = sqlc.narg(run_id)::uuid)
  AND (@paradigm_key::text = '' OR t.paradigm_key = @paradigm_key::text)
  AND (@paradigm_version::integer = 0 OR t.paradigm_version = @paradigm_version::integer)
  AND (@metric_engine_version::integer = 0 OR r.metric_engine_version = @metric_engine_version::integer)
  AND (@event_type::text = '' OR ev.event_type = @event_type::text)
  AND (sqlc.narg(disease_model_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM misko.subject_conditions c WHERE c.subject_id = t.subject_id AND c.disease_model_id = sqlc.narg(disease_model_id)::uuid))
  AND (sqlc.narg(substance_id)::uuid IS NULL OR EXISTS (
        SELECT 1 FROM misko.administrations a
        WHERE a.subject_id = t.subject_id AND a.experiment_id = t.experiment_id AND a.substance_id = sqlc.narg(substance_id)::uuid));
