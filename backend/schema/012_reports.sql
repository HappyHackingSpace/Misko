-- Read-only reporting views. They never write; every row comes from tables
-- owned by their domains.

-- latest marks the newest succeeded run of a test for each metric engine
-- version, so reanalysis never counts a test twice within one version.
CREATE INDEX analysis_runs_report_idx ON misko.analysis_runs (test_id, metric_engine_version, finished_at, id) WHERE status = 'SUCCEEDED';
CREATE INDEX analysis_runs_source_idx ON misko.analysis_runs (source_asset_id);

CREATE VIEW misko.report_runs AS
SELECT r.id, r.experiment_id, r.test_id, r.recording_id, r.source_asset_id, r.calibration_id, r.paradigm_key, r.paradigm_version,
       r.metric_engine_version, r.result_schema_version, r.trigger, r.model_version, r.finished_at,
       NOT EXISTS (
           SELECT 1 FROM misko.analysis_runs n
           WHERE n.status = 'SUCCEEDED' AND n.test_id = r.test_id AND n.metric_engine_version = r.metric_engine_version
             AND (n.finished_at, n.id) > (r.finished_at, r.id)) AS latest
FROM misko.analysis_runs r
WHERE r.status = 'SUCCEEDED';

CREATE INDEX tests_group_idx ON misko.tests (group_id) WHERE group_id IS NOT NULL;
CREATE INDEX tests_phase_idx ON misko.tests (phase_id) WHERE phase_id IS NOT NULL;
CREATE INDEX tests_environment_revision_idx ON misko.tests (environment_revision_id);

-- A test with the subject, experiment, phase, group and environment it was run
-- in. The group and phase are the ones stored on the test, not today's. The
-- experiment and environment are scalar subqueries so report statements stay
-- within the planner's join collapse limit and filters reach the indexes.
CREATE VIEW misko.report_tests AS
SELECT t.id AS test_id, t.experiment_id,
       (SELECT e.code FROM misko.experiments e WHERE e.id = t.experiment_id) AS experiment_code,
       t.subject_id, s.code AS subject_code, s.species, s.sex,
       t.phase_id, p.name AS phase_name,
       t.group_id, g.name AS group_name, g.role AS group_role,
       t.paradigm_key, t.paradigm_version, t.environment_revision_id,
       (SELECT er.environment_id FROM misko.environment_revisions er WHERE er.id = t.environment_revision_id) AS environment_id,
       (SELECT env.name FROM misko.environment_revisions er JOIN misko.environments env ON env.id = er.environment_id
        WHERE er.id = t.environment_revision_id) AS environment_name,
       (SELECT er.number FROM misko.environment_revisions er WHERE er.id = t.environment_revision_id) AS environment_revision,
       t.status AS test_status, t.scheduled_at,
       (SELECT count(*) FROM misko.trials tr WHERE tr.test_id = t.id) AS trial_count
FROM misko.tests t
JOIN misko.subjects s ON s.id = t.subject_id
LEFT JOIN misko.experiment_phases p ON p.id = t.phase_id
LEFT JOIN misko.experiment_groups g ON g.id = t.group_id;

-- Writes go through the owning domains; the views reject them even where
-- PostgreSQL could forward them to a base table.
CREATE TRIGGER report_runs_read_only INSTEAD OF INSERT OR UPDATE OR DELETE ON misko.report_runs
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
CREATE TRIGGER report_tests_read_only INSTEAD OF INSERT OR UPDATE OR DELETE ON misko.report_tests
    FOR EACH ROW EXECUTE FUNCTION misko.reject_change();
