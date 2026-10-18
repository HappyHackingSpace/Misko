-- Observer scoring is removed: every paradigm is analysed automatically from
-- the video and a person only reviews the result. This drops what
-- 015_scoring.sql created and restores the automated-only run rules of
-- 011_analysis.sql. Analysis events gain a label, which names what an event
-- refers to when its type declares labels (the novel or the familiar object).

DROP TABLE misko.scored_observations;
DROP TABLE misko.scoring_sheets;
DROP FUNCTION misko.check_scored_observation_change();
DROP FUNCTION misko.check_scoring_sheet_change();

DROP TRIGGER analysis_runs_scored_finished ON misko.analysis_runs;
DROP FUNCTION misko.check_scored_run_finished();
DROP TRIGGER analysis_runs_method_fixed ON misko.analysis_runs;
DROP FUNCTION misko.check_analysis_run_method();

ALTER TABLE misko.analysis_runs DROP CONSTRAINT analysis_runs_state_check;
ALTER TABLE misko.analysis_runs DROP COLUMN method, DROP COLUMN scored_by;
ALTER TABLE misko.analysis_runs ADD CONSTRAINT analysis_runs_state_check CHECK (
    (status = 'QUEUED' AND worker_id IS NULL AND lease_expires_at IS NULL AND finished_at IS NULL)
    OR (status = 'RUNNING' AND worker_id IS NOT NULL AND lease_expires_at IS NOT NULL AND attempt >= 1 AND finished_at IS NULL)
    OR (status = 'SUCCEEDED' AND worker_id IS NOT NULL AND model_version IS NOT NULL AND attempt >= 1 AND lease_expires_at IS NULL AND finished_at IS NOT NULL)
    OR (status = 'FAILED' AND lease_expires_at IS NULL AND failure_reason IS NOT NULL AND finished_at IS NOT NULL));

-- A run becomes SUCCEEDED only with its video pair and analysed video in the
-- same transaction.
CREATE OR REPLACE FUNCTION misko.check_analysis_published() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status = 'SUCCEEDED' AND NOT EXISTS (
        SELECT 1 FROM misko.analysis_video_pairs p
        JOIN misko.analysis_artifacts a ON a.run_id = p.run_id AND a.video_asset_id = p.analyzed_asset_id
        WHERE p.run_id = NEW.id AND p.attempt = NEW.attempt) THEN
        RAISE EXCEPTION 'run % succeeded without its video pair', NEW.id
            USING ERRCODE = 'integrity_constraint_violation', CONSTRAINT = 'analysis_runs_published';
    END IF;
    RETURN NULL;
END;
$$;

ALTER TABLE misko.analysis_events
    ADD COLUMN label text CONSTRAINT analysis_events_label_check CHECK (label ~ '^[a-z][a-z0-9_]{0,39}$');
