//go:build integration

package bootstrap

import (
	"context"
	"testing"
)

// The dashboard reads across the experiments, tests, calibration and analysis
// domains without owning any of their tables, so what it needs proving is
// that its counts and lists track what those domains hold.
func TestDashboardSummaryOverHTTP(t *testing.T) {
	a := newStudyAPI(t, newStudyObjects())
	empty := a.expect("empty dashboard", "GET", "/api/dashboard/summary", "VIEWER", nil, 200, "")
	if empty["tests"].(map[string]any)["planned"] != float64(0) || empty["calibrationWaiting"] != float64(0) ||
		empty["results"].(map[string]any)["pending"] != float64(0) || len(empty["upcomingTests"].([]any)) != 0 {
		t.Fatalf("empty dashboard: %v", empty)
	}

	newStudyWorld(t, a.pool, "EXP-DRAFT")
	w := newStudyWorld(t, a.pool, "EXP-D")
	m1, m2, m3 := w.allocate("G1", "M-1"), w.allocate("G2", "M-2"), w.allocate("G3", "M-3")
	w.start()
	// A planned session in the near future with two animals.
	planned := w.id(`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
		VALUES ($1, $2, $3, 1, 'MWM', 1, 1, now() + interval '2 hours', $4) RETURNING id`, w.exp, w.timepoints["Day 14"], w.version, studyAuthor)
	for i, allocation := range []string{m2, m1} {
		w.exec(`INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
			VALUES ($1, $2, $3, $4, 'MWM', 1, $5)`, w.exp, planned, allocation, w.arenas[i], studyAuthor)
	}
	// A session in progress: the third animal's override was rejected, so it waits for a calibration.
	_, subjects := w.session("Baseline", m1, m2, m3)
	w.exec(`INSERT INTO misko.calibrations (environment_revision_id, test_subject_id, recording_id, reference_frame_us, camera_id, frame_width, frame_height,
		crop_x, crop_y, crop_width, crop_height, measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm,
		tolerance_cm, algorithm_version, status, rejection_reason, created_by)
		SELECT ts.environment_revision_id, ts.id, r.id, 0, 'cam', 1920, 1080, 1280, 0, 640, 1080, 'ARENA_FLOOR', '[{},{},{},{}]', '[{},{},{}]', '{1,0,0,0,1,0,0,0,1}',
		5, 5, 5, 2, 'v1', 'REJECTED', 'EXCESSIVE_CHECK_ERROR', $2
		FROM misko.test_subjects ts JOIN misko.current_recordings r ON r.test_id = ts.test_id WHERE ts.id = $1`, subjects[2], studyAuthor)
	w.publish(subjects[0], distance(10), "PENDING")

	a.expect("register", "POST", "/api/analysis/workers", "SUPERADMIN", map[string]any{"name": "tracker", "modelVersion": "tracker 1.0",
		"capabilities": []any{map[string]any{"paradigmKey": "MWM", "paradigmVersion": 1}}}, 201, "")
	if err := a.api.AnalysisTick(context.Background()); err != nil {
		t.Fatal(err)
	}

	summary := a.expect("dashboard", "GET", "/api/dashboard/summary", "VIEWER", nil, 200, "")
	experiments, tests, results := summary["experiments"].(map[string]any), summary["tests"].(map[string]any), summary["results"].(map[string]any)
	// Two animals have a calibrated arena; the first one also has its own published run.
	if experiments["draft"] != float64(1) || experiments["running"] != float64(1) || tests["planned"] != float64(1) || tests["inProgress"] != float64(1) ||
		summary["calibrationWaiting"] != float64(1) || summary["analysisQueued"] != float64(2) || results["pending"] != float64(1) {
		t.Fatalf("counts: %v", summary)
	}
	upcoming := summary["upcomingTests"].([]any)[0].(map[string]any)
	if upcoming["id"] != planned || upcoming["experimentCode"] != "EXP-D" || upcoming["timepointName"] != "Day 14" ||
		len(upcoming["subjectCodes"].([]any)) != 2 || upcoming["subjectCodes"].([]any)[0] != "M-1" {
		t.Fatalf("upcoming: %v", upcoming)
	}
	activity := summary["recentActivity"].([]any)
	if len(activity) != 1 || activity[0].(map[string]any)["subjectCode"] != "M-1" || activity[0].(map[string]any)["testSubjectId"] != subjects[0] {
		t.Fatalf("activity: %v", activity)
	}
	a.expect("anonymous", "GET", "/api/dashboard/summary", "", nil, 401, "auth.unauthenticated")
}
