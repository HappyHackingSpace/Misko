-- Read-only reporting views. They never write; every row comes from tables
-- owned by their contexts, and the views reject writes even where PostgreSQL
-- could forward them.

-- Every animal of every test with the design it belongs to: group role,
-- disease model, the substance and dose of its group's treatment, the
-- timepoint and study day, the arena, whether the animal is excluded and the
-- review state of its result.
CREATE VIEW misko.report_test_subjects AS
SELECT ts.id AS test_subject_id, ts.test_id, ts.experiment_id,
       e.code AS experiment_code, e.blinded,
       a.id AS allocation_id, s.id AS subject_id, s.code AS subject_code, s.species, s.sex,
       g.id AS group_id, g.code AS group_code, g.name AS group_name, g.role AS group_role,
       g.disease_model_id, dm.name AS disease_model_name,
       tr.substance_id AS treatment_substance_id, sub.name AS treatment_substance_name,
       tr.dose_micro AS treatment_dose_micro, tr.unit AS treatment_unit,
       a.excluded_at IS NOT NULL AS excluded,
       tp.id AS timepoint_id, tp.name AS timepoint_name, tp.study_day,
       t.paradigm_key, t.paradigm_version, t.status AS test_status, t.scheduled_at,
       ts.environment_revision_id, er.environment_id, env.name AS environment_name, er.number AS environment_revision,
       res.status AS result_status, res.run_id AS result_run_id
FROM misko.test_subjects ts
JOIN misko.tests t ON t.id = ts.test_id
JOIN misko.experiments e ON e.id = ts.experiment_id
JOIN misko.allocations a ON a.id = ts.allocation_id
JOIN misko.subjects s ON s.id = a.subject_id
JOIN misko.experiment_groups g ON g.id = a.group_id
LEFT JOIN misko.disease_models dm ON dm.id = g.disease_model_id
LEFT JOIN misko.regimen_items tr ON tr.group_id = g.id AND tr.purpose = 'TREATMENT'
LEFT JOIN misko.substances sub ON sub.id = tr.substance_id
JOIN misko.timepoints tp ON tp.id = t.timepoint_id
JOIN misko.environment_revisions er ON er.id = ts.environment_revision_id
JOIN misko.environments env ON env.id = er.environment_id
LEFT JOIN misko.subject_results res ON res.test_subject_id = ts.id;

-- Accepted measurements only: the numbers statistics are computed from.
CREATE VIEW misko.report_accepted_metrics AS
SELECT rts.*, m.metric_key, m.unit, m.value, m.missing_reason,
       r.metric_engine_version, r.result_schema_version, r.model_version, r.finished_at
FROM misko.report_test_subjects rts
JOIN misko.analysis_runs r ON r.id = rts.result_run_id
JOIN misko.metric_results m ON m.run_id = r.id
WHERE rts.result_status = 'ACCEPTED' AND NOT rts.excluded;

CREATE TRIGGER report_test_subjects_read_only INSTEAD OF INSERT OR UPDATE OR DELETE ON misko.report_test_subjects
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER report_accepted_metrics_read_only INSTEAD OF INSERT OR UPDATE OR DELETE ON misko.report_accepted_metrics
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
