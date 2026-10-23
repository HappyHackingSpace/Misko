//go:build integration

package schema_test

import (
	"context"
	"slices"
	"testing"

	"github.com/HappyHackingSpace/Misko/backend/schema"
)

// 018_analysis_jobs.sql moves analysis runs into jobs: every run keeps its id
// as a job of its test subject with the calibration copied into the snapshot,
// its outputs, metrics and events move along, it gets a link on the
// experiment side and the reviewed result points to the job.
func TestAnalysisRunsMoveIntoJobs(t *testing.T) {
	ctx := context.Background()
	pool := emptyDatabase(t)
	migrations := installUpTo(t, pool, 17)
	if migrations[16].Name != "017_side_view_plane.sql" {
		t.Fatalf("this test seeds the schema of 017, got %s", migrations[16].Name)
	}
	d := db{t, pool}

	env := d.id(`INSERT INTO misko.environments (name, paradigm_key) VALUES ('Box', 'NOVEL_OBJECT') RETURNING id`)
	arena := d.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'NOVEL_OBJECT', 1, 1, '{"arena_width_cm": 40, "arena_height_cm": 40}', $2) RETURNING id`, env, user)
	subject := d.id(`INSERT INTO misko.subjects (code, species, sex) VALUES ('M1', 'MOUSE', 'MALE') RETURNING id`)
	exp := d.id(`INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Migration') RETURNING id`)
	group := d.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id`, exp)
	timepoint := d.id(`INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Day 1', 1, 1) RETURNING id`, exp)
	protocol := d.id(`INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id`, exp)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var version string
	if err := tx.QueryRow(ctx, `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
		VALUES ($1, $2, 1, 1, $3) RETURNING id`, exp, protocol, user).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
		VALUES ($1, 1, 'NOVEL_OBJECT', 1, 'TEST', 1, '{}')`, version); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	allocation := d.id(`INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
		VALUES ($1, $2, $3, now() - interval '1 day', $4) RETURNING id`, exp, group, subject, user)
	d.exec(`UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1`, exp)
	test := d.id(`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version,
		planned_trials, scheduled_at, created_by) VALUES ($1, $2, $3, 1, 'NOVEL_OBJECT', 1, 1, now(), $4) RETURNING id`, exp, timepoint, version, user)
	testSubject := d.id(`INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
		VALUES ($1, $2, $3, $4, 'NOVEL_OBJECT', 1, $5) RETURNING id`, exp, test, allocation, arena, user)
	d.exec(`UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() WHERE id = $1`, test)
	asset := d.id(`INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ORIGINAL', 'misko-test', 'videos/1.mp4', 'video/mp4', 1000, 1, 'VERIFIED', 7, now(), $1) RETURNING id`, user)
	recording := d.id(`INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, created_by) VALUES ($1, $2, $3, $4) RETURNING id`, exp, test, asset, user)
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var calibration string
	if err := tx.QueryRow(ctx, `INSERT INTO misko.calibrations (environment_revision_id, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width,
		crop_height, measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm,
		algorithm_version, status, created_by)
		VALUES ($1, 'cam', 640, 480, 0, 0, 640, 480, 'ARENA_FLOOR', '[1,2,3,4]', '[1,2,3]', '{0.1,0,0,0,0.1,0,0,0,1}', 0.1, 0.1, 0.2, 2, 'v1', 'VALID', $2)
		RETURNING id`, arena, user).Scan(&calibration); err != nil {
		t.Fatal(err)
	}
	for i, key := range []string{"novel_object", "familiar_object"} {
		if _, err := tx.Exec(ctx, `INSERT INTO misko.calibration_zones (calibration_id, zone_key, position, kind, pixels, centimeters)
			VALUES ($1, $2, 0, 'CIRCLE', $3, '{"x": 10, "y": 10, "radius": 2}')`, calibration, key, `{"x": `+[]string{"100", "300"}[i]+`, "y": 100, "radius": 20}`); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	worker := d.id(`INSERT INTO misko.analysis_workers (name, model_version, token_sha256, created_by) VALUES ('w', 'm 1', $1, $2) RETURNING id`,
		make([]byte, 32), user)

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var run string
	steps := []struct {
		sql  string
		args func() []any
	}{
		{`INSERT INTO misko.analysis_runs (experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c,
			clip_start_us, calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, parameters, trigger, status, attempt,
			max_attempts, worker_id, lease_expires_at, available_at)
			VALUES ($1, $2, $3, $4, $5, 7, 1, 0, $6, 'NOVEL_OBJECT', 1, 1, 1, '{"min_bout_s": 0.3}', 'AUTOMATIC', 'RUNNING', 1, 3, $7, now() + interval '1 hour', now())
			RETURNING id`, func() []any { return []any{exp, test, testSubject, recording, asset, calibration, worker} }},
		{`INSERT INTO misko.analysis_output_uploads (run_id, attempt, object_name, kind, content_type, size_bytes, crc32c)
			VALUES ($1, 1, 'runs/x/overlay.mp4', 'ANALYZED_VIDEO', 'video/mp4', 10, 1)`, func() []any { return []any{run} }},
		{`INSERT INTO misko.video_assets (id, kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
			VALUES ('01a00000-0000-7000-8000-00000000aaaa', 'ANALYZED', 'misko-test', 'runs/x/overlay.mp4', 'video/mp4', 10, 1, 'VERIFIED', 3, now(), $1)`,
			func() []any { return []any{worker} }},
		{`INSERT INTO misko.analysis_artifacts (run_id, attempt, object_name, kind, bucket, generation, size_bytes, crc32c, content_type, video_asset_id)
			VALUES ($1, 1, 'runs/x/overlay.mp4', 'ANALYZED_VIDEO', 'misko-test', 3, 10, 1, 'video/mp4', '01a00000-0000-7000-8000-00000000aaaa')`,
			func() []any { return []any{run} }},
		{`INSERT INTO misko.analysis_video_pairs (run_id, attempt, source_asset_id, source_generation, analyzed_asset_id, source_offset_us, output_offset_us,
			time_mapping_version) VALUES ($1, 1, $2, 7, '01a00000-0000-7000-8000-00000000aaaa', 0, 0, 'identity-v1')`, func() []any { return []any{run, asset} }},
		{`INSERT INTO misko.metric_results (run_id, attempt, metric_key, unit, value) VALUES ($1, 1, 'discrimination_index', 'ratio', 0.25)`,
			func() []any { return []any{run} }},
		{`INSERT INTO misko.analysis_events (run_id, attempt, test_subject_id, event_type, label, kind, start_us, end_us, confidence)
			VALUES ($1, 1, $2, 'exploration', 'novel', 'INTERVAL', 0, 500000, 0.9)`, func() []any { return []any{run, testSubject} }},
		{`UPDATE misko.analysis_runs SET status = 'SUCCEEDED', model_version = 'm 1', lease_expires_at = NULL, finished_at = now() WHERE id = $1`,
			func() []any { return []any{run} }},
		{`INSERT INTO misko.subject_results (test_subject_id, run_id) VALUES ($1, $2)`, func() []any { return []any{testSubject, run} }},
		{`UPDATE misko.subject_results SET status = 'ACCEPTED', reviewed_by = $2, reviewed_at = now() WHERE test_subject_id = $1`,
			func() []any { return []any{testSubject, user} }},
	}
	for i, step := range steps {
		if i == 0 {
			err = tx.QueryRow(ctx, step.sql, step.args()...).Scan(&run)
		} else {
			_, err = tx.Exec(ctx, step.sql, step.args()...)
		}
		if err != nil {
			t.Fatalf("seed step %d: %v", i, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	applied, err := schema.Migrate(ctx, pool)
	pending := []string{}
	for _, m := range migrations[17:] {
		pending = append(pending, m.Name)
	}
	if err != nil || !slices.Equal(applied, pending) || applied[0] != "018_analysis_jobs.sql" {
		t.Fatalf("migrate: %v %v", applied, err)
	}

	var kind, requester, status, key, calibrationID string
	var zones int
	if err := pool.QueryRow(ctx, `SELECT requester_kind, requester_id, status, idempotency_key, calibration_id, jsonb_array_length(calibration_zones)
		FROM misko.analysis_jobs WHERE id = $1`, run).Scan(&kind, &requester, &status, &key, &calibrationID, &zones); err != nil {
		t.Fatal(err)
	}
	if kind != "TEST_SUBJECT" || requester != testSubject || status != "SUCCEEDED" || calibrationID != calibration || zones != 2 ||
		key != "test-subject:"+testSubject+":recording:"+recording+":calibration:"+calibration {
		t.Fatalf("job: %s %s %s %s %s %d", kind, requester, status, key, calibrationID, zones)
	}
	var link, result, metric, pairs, events int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM misko.test_subject_analyses WHERE job_id = $1 AND test_subject_id = $2 AND recording_id = $3 AND trigger = 'AUTOMATIC'),
		(SELECT count(*) FROM misko.subject_results WHERE job_id = $1 AND status = 'ACCEPTED'),
		(SELECT count(*) FROM misko.report_accepted_metrics WHERE result_job_id = $1 AND metric_key = 'discrimination_index' AND value = 0.25),
		(SELECT count(*) FROM misko.analysis_job_pairs WHERE job_id = $1 AND analyzed_object_name = 'runs/x/overlay.mp4'),
		(SELECT count(*) FROM misko.analysis_job_events WHERE job_id = $1 AND label = 'novel')`, run, testSubject, recording).
		Scan(&link, &result, &metric, &pairs, &events); err != nil {
		t.Fatal(err)
	}
	if link != 1 || result != 1 || metric != 1 || pairs != 1 || events != 1 {
		t.Fatalf("moved rows: link %d result %d metric %d pairs %d events %d", link, result, metric, pairs, events)
	}
	var old bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('misko.analysis_runs') IS NOT NULL OR to_regclass('misko.metric_results') IS NOT NULL`).Scan(&old); err != nil || old {
		t.Fatalf("old tables kept: %v %v", old, err)
	}
	// The moved job is as immutable as a new one.
	d.reject("analysis_jobs_transition", `UPDATE misko.analysis_jobs SET status = 'FAILED', failure_reason = 'x' WHERE id = $1`, run)
	d.reject("analysis_job_metrics_immutable", `DELETE FROM misko.analysis_job_metrics WHERE job_id = $1`, run)
}
