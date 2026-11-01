//go:build integration

package schema_test

import (
	"context"
	"errors"
	"testing"

	"github.com/HappyHackingSpace/Misko/backend/schema"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const user = "00000000-0000-7000-8000-000000000001"

type db struct {
	t    *testing.T
	pool *pgxpool.Pool
}

func (d db) exec(sql string, args ...any) {
	d.t.Helper()
	if _, err := d.pool.Exec(context.Background(), sql, args...); err != nil {
		d.t.Fatalf("%s: %v", sql, err)
	}
}

func (d db) id(sql string, args ...any) string {
	d.t.Helper()
	var id string
	if err := d.pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		d.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

// reject expects the statement to fail on the named constraint, trigger rule
// or index.
func (d db) reject(constraint, sql string, args ...any) {
	d.t.Helper()
	_, err := d.pool.Exec(context.Background(), sql, args...)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != constraint {
		d.t.Errorf("want %s, got %v (%s)", constraint, err, sql)
	}
}

// A whole study, from catalog to completion, with the rule each step enforces.
func TestStudyLifecycleRules(t *testing.T) {
	ctx := context.Background()
	pool := emptyDatabase(t)
	if _, err := schema.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	d := db{t, pool}

	// Catalog and apparatus.
	compound := d.id(`INSERT INTO misko.substances (name, kind) VALUES ('Compound X', 'COMPOUND') RETURNING id`)
	saline := d.id(`INSERT INTO misko.substances (name, kind) VALUES ('Saline', 'VEHICLE') RETURNING id`)
	mptp := d.id(`INSERT INTO misko.substances (name, kind) VALUES ('MPTP', 'INDUCER') RETURNING id`)
	d.reject("substances_name_key", `INSERT INTO misko.substances (name, kind) VALUES ('saline', 'VEHICLE')`)
	parkinson := d.id(`INSERT INTO misko.disease_models (name, induction_type) VALUES ('MPTP Parkinson', 'CHEMICAL') RETURNING id`)
	boxA := d.id(`INSERT INTO misko.environments (name, paradigm_key) VALUES ('Box A', 'MWM') RETURNING id`)
	boxB := d.id(`INSERT INTO misko.environments (name, paradigm_key) VALUES ('Box B', 'MWM') RETURNING id`)
	maze := d.id(`INSERT INTO misko.environments (name, paradigm_key) VALUES ('Maze', 'POLE') RETURNING id`)
	arenaA := d.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'MWM', 1, 1, '{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30}', $2) RETURNING id`, boxA, user)
	arenaB := d.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'MWM', 1, 1, '{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30}', $2) RETURNING id`, boxB, user)
	mazeRev := d.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'POLE', 1, 1, '{}', $2) RETURNING id`, maze, user)
	d.reject("environment_revisions_environment_fkey", `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'POLE', 1, 2, '{}', $2)`, boxA, user)
	d.reject("environment_revisions_immutable", `UPDATE misko.environment_revisions SET notes = 'x' WHERE id = $1`, arenaA)

	animal := func(code string) string {
		return d.id(`INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id`, code)
	}
	healthy, sick, treated, late := animal("M1"), animal("M2"), animal("M3"), animal("M4")

	// Design in DRAFT.
	exp := d.id(`INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Compound X in MPTP mice') RETURNING id`)
	d.reject("experiment_groups_disease_check", `INSERT INTO misko.experiment_groups (experiment_id, code, name, role, disease_model_id)
		VALUES ($1, 'G0', 'Wrong', 'HEALTHY_CONTROL', $2)`, exp, parkinson)
	d.reject("experiment_groups_disease_check", `INSERT INTO misko.experiment_groups (experiment_id, code, name, role)
		VALUES ($1, 'G0', 'Wrong', 'TREATMENT')`, exp)
	gHealthy := d.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role, target_size)
		VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL', 8) RETURNING id`, exp)
	gDisease := d.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role, disease_model_id, target_size)
		VALUES ($1, 'G2', 'MPTP + vehicle', 'DISEASE_CONTROL', $2, 8) RETURNING id`, exp, parkinson)
	gTreated := d.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role, disease_model_id, target_size)
		VALUES ($1, 'G3', 'MPTP + X 10', 'TREATMENT', $2, 8) RETURNING id`, exp, parkinson)

	regimen := func(group, purpose, substance, frequency string, start, end int) string {
		return d.id(`INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
			VALUES ($1, $2, $3, $4, 10000000, 'mg/kg', 'INTRAPERITONEAL', $5, $6, $7) RETURNING id`, exp, group, purpose, substance, frequency, start, end)
	}
	regimen(gDisease, "INDUCTION", mptp, "ONCE", 0, 0)
	regimen(gDisease, "VEHICLE", saline, "DAILY", 1, 14)
	regimen(gTreated, "INDUCTION", mptp, "ONCE", 0, 0)
	treatment := regimen(gTreated, "TREATMENT", compound, "DAILY", 1, 14)
	d.reject("regimen_items_treatment_key", `INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
		VALUES ($1, $2, 'TREATMENT', $3, 1, 'mg/kg', 'ORAL', 'DAILY', 1, 14)`, exp, gTreated, compound)
	d.reject("regimen_items_days_check", `INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
		VALUES ($1, $2, 'VEHICLE', $3, 1, 'mL/kg', 'ORAL', 'ONCE', 1, 3)`, exp, gHealthy, saline)
	d.reject("regimen_items_interval_check", `INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
		VALUES ($1, $2, 'VEHICLE', $3, 1, 'mL/kg', 'ORAL', 'EVERY_N_DAYS', 1, 3)`, exp, gHealthy, saline)

	baseline := d.id(`INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Baseline', -1, 1) RETURNING id`, exp)
	day14 := d.id(`INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Day 14', 14, 2) RETURNING id`, exp)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	protocol := ""
	if err := tx.QueryRow(ctx, `INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id`, exp).Scan(&protocol); err != nil {
		t.Fatal(err)
	}
	version := ""
	if err := tx.QueryRow(ctx, `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
		VALUES ($1, $2, 1, 1, $3) RETURNING id`, exp, protocol, user).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
		VALUES ($1, 1, 'MWM', 1, 'ACQUISITION', 1, '{}')`, version); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	d.exec(`INSERT INTO misko.endpoints (experiment_id, timepoint_id, baseline_timepoint_id, paradigm_key, metric_key, kind, direction)
		VALUES ($1, $2, $3, 'MWM', 'path_length_cm', 'PRIMARY', 'HIGHER_IS_BETTER')`, exp, day14, baseline)
	d.reject("endpoints_primary_key", `INSERT INTO misko.endpoints (experiment_id, timepoint_id, paradigm_key, metric_key, kind, direction)
		VALUES ($1, $2, 'MWM', 'escape_latency_s', 'PRIMARY', 'HIGHER_IS_BETTER')`, exp, day14)
	d.reject("endpoints_baseline_check", `INSERT INTO misko.endpoints (experiment_id, timepoint_id, baseline_timepoint_id, paradigm_key, metric_key, kind, direction)
		VALUES ($1, $2, $2, 'MWM', 'escape_latency_s', 'SECONDARY', 'HIGHER_IS_BETTER')`, exp, day14)

	// Allocation: one group per animal, ever; free to move while a draft.
	allocate := func(group, subject string) string {
		return d.id(`INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
			VALUES ($1, $2, $3, now() - interval '1 day', $4) RETURNING id`, exp, group, subject, user)
	}
	aHealthy, aSick, aTreated := allocate(gHealthy, healthy), allocate(gDisease, sick), allocate(gTreated, treated)
	d.reject("allocations_subject_key", `INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
		VALUES ($1, $2, $3, now(), $4)`, exp, gDisease, healthy, user)
	d.exec(`UPDATE misko.allocations SET group_id = $1 WHERE id = $2`, gDisease, aTreated)
	d.exec(`UPDATE misko.allocations SET group_id = $1 WHERE id = $2`, gTreated, aTreated)

	d.reject("tests_not_running", `INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version,
		planned_trials, scheduled_at, created_by) VALUES ($1, $2, $3, 1, 'MWM', 1, 1, now(), $4)`, exp, day14, version, user)
	d.reject("administrations_not_running", `INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, purpose, substance_id,
		dose_micro, unit, route, administered_at, recorded_by) VALUES ($1, $2, $3, $4, 'VEHICLE', $5, 1, 'mL', 'ORAL', now(), $6)`,
		exp, aSick, gDisease, sick, saline, user)

	// Start: day 0 is set and the design freezes.
	d.reject("experiments_lifecycle_check", `UPDATE misko.experiments SET status = 'RUNNING' WHERE id = $1`, exp)
	d.exec(`UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1`, exp)
	d.reject("experiment_groups_frozen", `INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G4', 'Late', 'HEALTHY_CONTROL')`, exp)
	d.reject("regimen_items_frozen", `UPDATE misko.regimen_items SET dose_micro = 1 WHERE id = $1`, treatment)
	d.reject("timepoints_frozen", `DELETE FROM misko.timepoints WHERE id = $1`, baseline)
	d.reject("allocations_frozen", `UPDATE misko.allocations SET group_id = $1 WHERE id = $2`, gDisease, aTreated)
	d.reject("allocations_frozen", `DELETE FROM misko.allocations WHERE id = $1`, aTreated)
	d.reject("experiments_transition", `UPDATE misko.experiments SET day_zero = current_date + 1 WHERE id = $1`, exp)
	d.reject("experiments_transition", `DELETE FROM misko.experiments WHERE id = $1`, exp)
	aLate := allocate(gHealthy, late)

	// Dosing and induction.
	weight := d.id(`INSERT INTO misko.weight_measurements (subject_id, body_milligrams, measured_at, recorded_by)
		VALUES ($1, 25000, now(), $2) RETURNING id`, treated, user)
	sickWeight := d.id(`INSERT INTO misko.weight_measurements (subject_id, body_milligrams, measured_at, recorded_by)
		VALUES ($1, 24000, now(), $2) RETURNING id`, sick, user)
	d.reject("administrations_per_kg_weight_check", `INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, regimen_item_id,
		purpose, substance_id, dose_micro, unit, route, administered_at, recorded_by)
		VALUES ($1, $2, $3, $4, $5, 'TREATMENT', $6, 10000000, 'mg/kg', 'INTRAPERITONEAL', now(), $7)`, exp, aTreated, gTreated, treated, treatment, compound, user)
	d.exec(`INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, regimen_item_id, purpose, substance_id,
		dose_micro, unit, route, weight_measurement_id, administered_at, recorded_by)
		VALUES ($1, $2, $3, $4, $5, 'TREATMENT', $6, 10000000, 'mg/kg', 'INTRAPERITONEAL', $7, now(), $8)`, exp, aTreated, gTreated, treated, treatment, compound, weight, user)
	d.reject("administrations_regimen_fkey", `INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, regimen_item_id, purpose,
		substance_id, dose_micro, unit, route, weight_measurement_id, administered_at, recorded_by)
		VALUES ($1, $2, $3, $4, $5, 'TREATMENT', $6, 1, 'mg/kg', 'ORAL', $7, now(), $8)`, exp, aSick, gDisease, sick, treatment, compound, sickWeight, user)
	d.reject("administrations_weight_fkey", `INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, purpose,
		substance_id, dose_micro, unit, route, weight_measurement_id, administered_at, recorded_by)
		VALUES ($1, $2, $3, $4, 'VEHICLE', $5, 1, 'mL/kg', 'ORAL', $6, now(), $7)`, exp, aSick, gDisease, sick, saline, weight, user)
	d.reject("administrations_group_fkey", `INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, purpose,
		substance_id, dose_micro, unit, route, administered_at, recorded_by)
		VALUES ($1, $2, $3, $4, 'VEHICLE', $5, 1, 'mL', 'ORAL', now(), $6)`, exp, aSick, gTreated, sick, saline, user)
	d.exec(`INSERT INTO misko.induction_outcomes (allocation_id, experiment_id, status, assessed_at, recorded_by)
		VALUES ($1, $2, 'CONFIRMED', now(), $3)`, aSick, exp, user)
	d.exec(`UPDATE misko.allocations SET excluded_at = now(), exclusion_reason = 'DIED' WHERE id = $1`, aLate)
	d.reject("allocations_frozen", `UPDATE misko.allocations SET excluded_at = NULL, exclusion_reason = NULL WHERE id = $1`, aLate)

	// A session with two animals in two arenas sharing one video.
	test := d.id(`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version,
		planned_trials, scheduled_at, created_by) VALUES ($1, $2, $3, 1, 'MWM', 1, 1, now(), $4) RETURNING id`, exp, day14, version, user)
	d.reject("tests_step_fkey", `INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version,
		planned_trials, scheduled_at, created_by) VALUES ($1, $2, $3, 1, 'MWM', 1, 3, now(), $4)`, exp, day14, version, user)
	join := func(allocation, arena string) string {
		return d.id(`INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
			VALUES ($1, $2, $3, $4, 'MWM', 1, $5) RETURNING id`, exp, test, allocation, arena, user)
	}
	first, second := join(aHealthy, arenaA), join(aSick, arenaB)
	insertSubject := `INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
		VALUES ($1, $2, $3, $4, $5, 1, $6)`
	d.reject("test_subjects_arena_key", insertSubject, exp, test, aTreated, arenaA, "MWM", user)
	d.reject("test_subjects_frozen", insertSubject, exp, test, aLate, arenaA, "MWM", user)
	d.reject("test_subjects_paradigm_fkey", insertSubject, exp, test, aTreated, mazeRev, "POLE", user)

	asset := d.id(`INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, created_by)
		VALUES ('ORIGINAL', 'misko-test', 'videos/1.mp4', 'video/mp4', 1000, 1, $1) RETURNING id`, user)
	d.exec(`UPDATE misko.video_assets SET status = 'VERIFIED', generation = 1, verified_at = now() WHERE id = $1`, asset)
	insertRecording := `INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, created_by) VALUES ($1, $2, $3, $4)`
	d.reject("test_recordings_test_started", insertRecording, exp, test, asset, user)
	d.exec(`UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() WHERE id = $1`, test)
	d.reject("test_subjects_frozen", `DELETE FROM misko.test_subjects WHERE id = $1`, second)
	recording := d.id(insertRecording+" RETURNING id", exp, test, asset, user)
	other := d.id(`INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, created_by)
		VALUES ('ORIGINAL', 'misko-test', 'videos/2.mp4', 'video/mp4', 1000, 1, $1) RETURNING id`, user)
	d.reject("test_recordings_first_key", insertRecording, exp, test, other, user)

	d.exec(`INSERT INTO misko.trials (test_id, test_subject_id, number, repetition, attempt, started_at, recorded_by)
		VALUES ($1, $2, 1, 1, 1, now(), $3)`, test, first, user)
	d.reject("trials_test_in_progress", `INSERT INTO misko.trials (test_id, test_subject_id, number, repetition, attempt, started_at, recorded_by)
		VALUES ($1, $2, 2, 2, 1, now(), $3)`, test, first, user)

	// Calibration: each arena has its own default; an override must use the
	// animal's own arena.
	calibrate := `INSERT INTO misko.calibrations (environment_revision_id, test_subject_id, recording_id, camera_id, frame_width, frame_height,
		crop_x, crop_y, crop_width, crop_height, reference_frame_us, measurement_plane, fit_points, check_points, transform,
		fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, created_by)
		VALUES ($1, $2, $3, 'cam', 1920, 1080, $4, 0, 960, 1080, $5, 'ARENA_FLOOR', '[1,2,3,4]', '[1,2,3]', '{1,0,0,0,1,0,0,0,1}',
		0.1, 0.1, 0.2, 2, 'v1', 'VALID', $6)`
	calA := d.id(calibrate+" RETURNING id", arenaA, nil, nil, 0, nil, user)
	calB := d.id(calibrate+" RETURNING id", arenaB, nil, nil, 960, nil, user)
	d.reject("calibrations_scope", calibrate, arenaB, first, recording, 0, 0, user)
	// An override names its video frame; the scope trigger runs first, so
	// this one passes it and fails on the shape check.
	d.reject("calibrations_owner_check", calibrate, arenaA, first, recording, 0, nil, user)

	var effective int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM misko.effective_calibrations
		WHERE (test_subject_id = $1 AND calibration_id = $2) OR (test_subject_id = $3 AND calibration_id = $4)`, first, calA, second, calB).Scan(&effective); err != nil || effective != 2 {
		t.Errorf("effective calibrations: %d %v", effective, err)
	}

	// The analysis service queues a job per animal with that animal's
	// calibration copied in; the experiment side links it to the animal.
	job := `INSERT INTO misko.analysis_jobs (requester_kind, requester_id, idempotency_key, paradigm_key, paradigm_version, metric_engine_version,
		result_schema_version, parameters, source_asset_id, source_generation, source_crc32c, clip_start_us, calibration_id, calibration_frame_width,
		calibration_frame_height, calibration_crop_x, calibration_crop_y, calibration_crop_width, calibration_crop_height, calibration_plane,
		calibration_transform, calibration_zones, trigger, status, max_attempts, available_at)
		VALUES ('TEST_SUBJECT', $1, $2, 'MWM', 1, 1, 1, '{}', $3, 1, 1, 0, $4, 1920, 1080, 0, 0, 960, 1080, 'ARENA_FLOOR', '{1,0,0,0,1,0,0,0,1}', '[]',
		'AUTOMATIC', 'QUEUED', 3, now())`
	queued := d.id(job+" RETURNING id", first, "key-1", asset, calA)
	d.reject("analysis_jobs_idempotency_key", job, first, "key-1", asset, calA)
	d.reject("analysis_jobs_calibration_check", `INSERT INTO misko.analysis_jobs (requester_kind, requester_id, paradigm_key, paradigm_version,
		metric_engine_version, result_schema_version, parameters, source_asset_id, source_generation, source_crc32c, clip_start_us, calibration_id,
		trigger, status, max_attempts, available_at) VALUES ('TRIAL', $1, 'MWM', 1, 1, 1, '{}', $2, 1, 1, 0, $3, 'MANUAL', 'QUEUED', 3, now())`, first, asset, calA)
	link := `INSERT INTO misko.test_subject_analyses (job_id, experiment_id, test_id, test_subject_id, recording_id, calibration_id, trigger)
		VALUES ($1, $2, $3, $4, $5, $6, 'AUTOMATIC')`
	d.reject("test_subject_analyses_requester", link, queued, exp, test, second, recording, calB)
	d.exec(link, queued, exp, test, first, recording, calA)
	d.reject("subject_results_succeeded", `INSERT INTO misko.subject_results (test_subject_id, job_id) VALUES ($1, $2)`, first, queued)
	d.reject("analysis_job_outputs_fenced", `INSERT INTO misko.analysis_job_metrics (job_id, attempt, metric_key, unit, value) VALUES ($1, 1, 'x', 'cm', 1)`, queued)

	var reported int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM misko.report_test_subjects
		WHERE test_id = $1 AND ((group_role = 'HEALTHY_CONTROL' AND disease_model_id IS NULL)
		   OR (group_role = 'DISEASE_CONTROL' AND disease_model_name = 'MPTP Parkinson' AND treatment_substance_id IS NULL))`, test).Scan(&reported); err != nil || reported != 2 {
		t.Errorf("report rows: %d %v", reported, err)
	}

	// Completing closes the experiment for good.
	d.exec(`UPDATE misko.tests SET status = 'COMPLETED', completed_at = now() WHERE id = $1`, test)
	d.exec(`UPDATE misko.experiments SET status = 'COMPLETED', completed_at = now() WHERE id = $1`, exp)
	d.reject("experiments_transition", `UPDATE misko.experiments SET title = 'Renamed' WHERE id = $1`, exp)
	d.reject("tests_not_running", `INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version,
		planned_trials, scheduled_at, created_by) VALUES ($1, $2, $3, 1, 'MWM', 1, 1, now(), $4)`, exp, day14, version, user)
	d.reject("allocations_frozen", `INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
		VALUES ($1, $2, $3, now(), $4)`, exp, gHealthy, animal("M5"), user)
	d.reject("protocols_closed", `INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Late battery')`, exp)
}

// A healthy control's items are periods without any substance (020): only a
// first and a last study day, never dosed, and only in a healthy control.
func TestNoSubstancePeriodRules(t *testing.T) {
	pool := emptyDatabase(t)
	if _, err := schema.Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	d := db{t, pool}
	saline := d.id(`INSERT INTO misko.substances (name, kind) VALUES ('Saline', 'VEHICLE') RETURNING id`)
	model := d.id(`INSERT INTO misko.disease_models (name, induction_type) VALUES ('MPTP Parkinson', 'CHEMICAL') RETURNING id`)
	exp := d.id(`INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id`)
	healthy := d.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id`, exp)
	sham := d.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role, disease_model_id) VALUES ($1, 'G2', 'Sham', 'SHAM', $2) RETURNING id`, exp, model)

	const substanceItem = `INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
		VALUES ($1, $2, $3, $4, 1000000, 'mL/kg', 'ORAL', 'DAILY', 1, 14)`
	period := d.id(`INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, start_day, end_day) VALUES ($1, $2, 'NONE', -7, 28) RETURNING id`, exp, healthy)
	d.exec(`INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, start_day, end_day) VALUES ($1, $2, 'NONE', 30, 30)`, exp, healthy)
	vehicle := d.id(substanceItem+" RETURNING id", exp, sham, "VEHICLE", saline)

	// Shape: a period has nothing but days; every other item has every field.
	d.reject("regimen_items_substance_check", `UPDATE misko.regimen_items SET substance_id = $2 WHERE id = $1`, period, saline)
	for _, set := range []string{"dose_micro = 1", "unit = 'mL'", "route = 'ORAL'", "frequency = 'DAILY'", "interval_days = 3"} {
		d.reject("regimen_items_substance_check", `UPDATE misko.regimen_items SET `+set+` WHERE id = $1`, period)
	}
	for _, column := range []string{"substance_id", "dose_micro", "unit", "route", "frequency"} {
		d.reject("regimen_items_substance_check", `UPDATE misko.regimen_items SET `+column+` = NULL WHERE id = $1`, vehicle)
	}
	d.reject("regimen_items_days_check", `INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, start_day, end_day) VALUES ($1, $2, 'NONE', 5, 4)`, exp, healthy)
	d.reject("regimen_items_purpose_check", `INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, start_day, end_day) VALUES ($1, $2, 'NAIVE', 0, 4)`, exp, healthy)
	d.reject("regimen_items_dose_micro_check", `UPDATE misko.regimen_items SET dose_micro = 0 WHERE id = $1`, vehicle)

	// Role: periods only in a healthy control, and a healthy control only periods.
	d.reject("regimen_items_role", `INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, start_day, end_day) VALUES ($1, $2, 'NONE', 0, 4)`, exp, sham)
	for _, purpose := range []string{"VEHICLE", "INDUCTION", "TREATMENT"} {
		d.reject("regimen_items_role", substanceItem, exp, healthy, purpose, saline)
	}
	d.reject("regimen_items_role", `UPDATE misko.regimen_items SET group_id = $2 WHERE id = $1`, period, sham)
	d.reject("regimen_items_role", `UPDATE misko.regimen_items SET group_id = $2 WHERE id = $1`, vehicle, healthy)
	d.reject("experiment_groups_regimen_role", `UPDATE misko.experiment_groups SET role = 'SHAM', disease_model_id = $2 WHERE id = $1`, healthy, model)
	d.reject("experiment_groups_regimen_role", `UPDATE misko.experiment_groups SET role = 'HEALTHY_CONTROL', disease_model_id = NULL WHERE id = $1`, sham)
	d.exec(`UPDATE misko.experiment_groups SET name = 'Naive' WHERE id = $1`, healthy)
	d.exec(`UPDATE misko.regimen_items SET start_day = -3 WHERE id = $1`, period)

	// Never administered, even with a substance of its own.
	subject := d.id(`INSERT INTO misko.subjects (code, species, sex) VALUES ('M1', 'MOUSE', 'MALE') RETURNING id`)
	allocation := d.id(`INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
		VALUES ($1, $2, $3, now() - interval '1 day', $4) RETURNING id`, exp, healthy, subject, user)
	d.exec(`UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1`, exp)
	d.reject("administrations_no_substance", `INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, regimen_item_id, purpose,
		substance_id, dose_micro, unit, route, administered_at, recorded_by)
		VALUES ($1, $2, $3, $4, $5, 'VEHICLE', $6, 1, 'mL', 'ORAL', now(), $7)`, exp, allocation, healthy, subject, period, saline, user)
	// An unplanned dose to the same animal is still recorded.
	d.exec(`INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, purpose, substance_id, dose_micro, unit, route,
		administered_at, recorded_by) VALUES ($1, $2, $3, $4, 'VEHICLE', $5, 1, 'mL', 'ORAL', now(), $6)`, exp, allocation, healthy, subject, saline, user)
}

// 020 leaves the items written before it as they are: a healthy control's
// vehicle in a started study is kept and still dosed.
func TestNoSubstancePeriodsKeepEarlierItems(t *testing.T) {
	ctx := context.Background()
	pool := emptyDatabase(t)
	migrations := installUpTo(t, pool, 19)
	if migrations[19].Name != "020_no_substance_periods.sql" {
		t.Fatalf("this test seeds the schema of 019, got %s", migrations[19].Name)
	}
	d := db{t, pool}
	saline := d.id(`INSERT INTO misko.substances (name, kind) VALUES ('Saline', 'VEHICLE') RETURNING id`)
	exp := d.id(`INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Started') RETURNING id`)
	draft := d.id(`INSERT INTO misko.experiments (code, title) VALUES ('E2', 'Draft') RETURNING id`)
	healthy := d.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id`, exp)
	draftHealthy := d.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id`, draft)
	const vehicle = `INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
		VALUES ($1, $2, 'VEHICLE', $3, 1000000, 'mL', 'ORAL', 'DAILY', 1, 14) RETURNING id`
	started := d.id(vehicle, exp, healthy, saline)
	kept := d.id(vehicle, draft, draftHealthy, saline)
	subject := d.id(`INSERT INTO misko.subjects (code, species, sex) VALUES ('M1', 'MOUSE', 'MALE') RETURNING id`)
	allocation := d.id(`INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
		VALUES ($1, $2, $3, now() - interval '1 day', $4) RETURNING id`, exp, healthy, subject, user)
	d.exec(`UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1`, exp)

	if applied, err := schema.Migrate(ctx, pool); err != nil || len(applied) != len(migrations)-19 || applied[0] != "020_no_substance_periods.sql" {
		t.Fatalf("migrate: %v %v", applied, err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM misko.regimen_items WHERE id IN ($1, $2) AND purpose = 'VEHICLE' AND substance_id = $3
		AND dose_micro = 1000000 AND frequency = 'DAILY'`, started, kept, saline).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("earlier items: %d %v", rows, err)
	}
	d.exec(`INSERT INTO misko.administrations (experiment_id, allocation_id, group_id, subject_id, regimen_item_id, purpose, substance_id,
		dose_micro, unit, route, administered_at, recorded_by) VALUES ($1, $2, $3, $4, $5, 'VEHICLE', $6, 1000000, 'mL', 'ORAL', now(), $7)`,
		exp, allocation, healthy, subject, started, saline, user)
	// The draft's vehicle cannot change except into a period, and can be deleted.
	d.reject("regimen_items_role", `UPDATE misko.regimen_items SET dose_micro = 2000000 WHERE id = $1`, kept)
	d.exec(`UPDATE misko.regimen_items SET purpose = 'NONE', substance_id = NULL, dose_micro = NULL, unit = NULL, route = NULL, frequency = NULL
		WHERE id = $1`, kept)
	d.exec(`DELETE FROM misko.regimen_items WHERE id = $1`, kept)
}
