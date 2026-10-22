-- Read-only queries over the study design and the reporting views of
-- schema/012_reports.sql. Statistics use only accepted results of animals that
-- are not excluded (report_accepted_metrics); report_test_subjects supplies the
-- pending, rejected and unanalysed animals so a half-reviewed report is never
-- mistaken for a final one.

-- name: ReportExperiment :one
SELECT id, code, title, status, blinded
FROM misko.experiments
WHERE id = @id;

-- name: ReportGroups :many
SELECT g.id, g.code, g.name, g.role, g.disease_model_id, dm.name AS disease_model_name,
       tr.substance_id AS treatment_substance_id, sub.name AS treatment_substance_name,
       tr.dose_micro AS treatment_dose_micro, tr.unit AS treatment_unit,
       (SELECT count(*) FROM misko.allocations a WHERE a.group_id = g.id)::integer AS allocated,
       (SELECT count(*) FROM misko.allocations a WHERE a.group_id = g.id AND a.excluded_at IS NOT NULL)::integer AS excluded
FROM misko.experiment_groups g
LEFT JOIN misko.disease_models dm ON dm.id = g.disease_model_id
LEFT JOIN misko.regimen_items tr ON tr.group_id = g.id AND tr.purpose = 'TREATMENT'
LEFT JOIN misko.substances sub ON sub.id = tr.substance_id
WHERE g.experiment_id = @experiment_id
ORDER BY lower(g.code), g.id;

-- name: ReportTimepoints :many
SELECT id, name, study_day, position
FROM misko.timepoints
WHERE experiment_id = @experiment_id
ORDER BY position;

-- name: ReportEndpoints :many
SELECT id, timepoint_id, baseline_timepoint_id, paradigm_key, metric_key, kind, direction
FROM misko.endpoints
WHERE experiment_id = @experiment_id
ORDER BY CASE kind WHEN 'PRIMARY' THEN 0 ELSE 1 END, paradigm_key, metric_key, created_at, id;

-- ReportMeasurements returns every animal filmed for one paradigm in tests of
-- the experiment that were not cancelled, at every timepoint, with its review
-- state and, when its result is accepted and the animal is not excluded, the
-- value of one metric.
-- name: ReportMeasurements :many
SELECT rts.test_subject_id, rts.test_id, rts.allocation_id, rts.subject_id, rts.subject_code, rts.group_id, rts.excluded::boolean AS excluded,
       rts.result_status, rts.result_job_id, rts.timepoint_id, rts.paradigm_version, rts.environment_name, rts.scheduled_at,
       m.unit AS metric_unit, m.value AS metric_value, m.missing_reason, m.metric_engine_version
FROM misko.report_test_subjects rts
LEFT JOIN misko.report_accepted_metrics m ON m.test_subject_id = rts.test_subject_id AND m.metric_key = @metric_key::text
WHERE rts.experiment_id = @experiment_id AND rts.paradigm_key = @paradigm_key::text AND rts.test_status <> 'CANCELLED'
ORDER BY rts.scheduled_at, lower(rts.subject_code), rts.test_subject_id
LIMIT @row_limit::integer;
