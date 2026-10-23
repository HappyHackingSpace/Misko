//go:build integration

package bootstrap

import (
	"context"
	"slices"
	"strings"
	"testing"

	mediadomain "github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
)

// swim moves 6 cm in 6 s along a line inside the tank.
func swim() [][3]float64 {
	var out [][3]float64
	for i := 0; i <= 60; i++ {
		out = append(out, [3]float64{float64(i) / 10, float64(i) / 10, 20})
	}
	return out
}

// The experiment flow through the analysis service's port: the orchestrator
// submits a job per ready animal, a worker analyses it, the first success
// opens the review, accepted results reach the report and the dashboard.
// A paradigm trial in the same arena, analysed by the same worker at the
// same time, never appears in review, reports or the dashboard.
func TestExperimentAnalysisThroughThePort(t *testing.T) {
	ctx := context.Background()
	objects := newStudyObjects()
	a := newStudyAPI(t, objects)
	w := newStudyWorld(t, a.pool, "EXP-SV")
	m1, m2 := w.allocate("G1", "M-1"), w.allocate("G2", "M-2")
	w.start()
	test, subjects := w.session("Day 14", m1, m2)
	register := map[string]any{"name": "mwm tracker", "modelVersion": "tracker 3.0", "capabilities": []any{map[string]any{"paradigmKey": "MWM", "paradigmVersion": 1}}}
	worker := "Worker " + a.expect("register", "POST", "/api/analysis/workers", "SUPERADMIN", register, 201, "")["token"].(string)

	// A trial in the first animal's arena, whose default calibration it uses.
	trial := a.expect("trial", "POST", "/api/trials", "RESEARCHER", map[string]any{"name": "Pool check", "paradigmKey": "MWM", "paradigmVersion": 1,
		"environmentRevisionId": w.arenas[0]}, 201, "")["id"].(string)
	started := a.expect("upload", "POST", "/api/trials/"+trial+"/video", "RESEARCHER", map[string]any{"fileName": "pool.mp4", "contentType": "video/mp4",
		"sizeBytes": 4096, "crc32c": mediadomain.FormatCRC32C(1), "clipEndUs": 6_000_000}, 201, "")
	objects.put(strings.TrimPrefix(started["upload"].(map[string]any)["url"].(string), "https://storage.test/"), nil, 4096, 1, "video/mp4")
	detail := a.expect("finalize", "POST", "/api/trials/"+trial+"/video/finalize", "RESEARCHER", map[string]any{}, 200, "")
	if analyses := detail["analyses"].([]any); len(analyses) != 1 || analyses[0].(map[string]any)["calibrationId"] == nil {
		t.Fatalf("the verified trial video is analysed at once with the arena's calibration: %v", detail)
	}

	if err := a.api.AnalysisTick(ctx); err != nil {
		t.Fatal(err)
	}
	runs := a.expect("runs", "GET", "/api/tests/"+test+"/analysis-runs", "VIEWER", nil, 200, "")["data"].([]any)
	if len(runs) != 2 {
		t.Fatalf("one job per animal: %v", runs)
	}
	for _, r := range runs {
		run := r.(map[string]any)
		if run["trigger"] != "AUTOMATIC" || run["status"] != "QUEUED" || run["id"] != run["jobId"] || !slices.Contains(subjects, run["testSubjectId"].(string)) {
			t.Fatalf("run: %v", run)
		}
	}
	// Before the worker finishes, the dashboard counts only the experiment's queue.
	if summary := a.expect("dashboard", "GET", "/api/dashboard/summary", "VIEWER", nil, 200, ""); summary["analysisQueued"] != float64(2) {
		t.Fatalf("dashboard counts a trial job: %v", summary)
	}
	for range 3 {
		completeClaim(t, a, objects, worker, claimOne(t, a, worker), 6_000_000, swim())
	}

	// The first success of each animal opened its review; the trial's did not.
	results := a.expect("review", "GET", "/api/analysis/results?status=ALL", "VIEWER", nil, 200, "")
	if results["total"] != float64(2) {
		t.Fatalf("review: %v", results)
	}
	for _, i := range results["data"].([]any) {
		item := i.(map[string]any)
		if item["status"] != "PENDING" || item["experimentId"] != w.exp || len(item["run"].(map[string]any)["metrics"].([]any)) != 8 ||
			item["run"].(map[string]any)["modelVersion"] != "tracker 3.0" || item["newerRunAvailable"] != false {
			t.Fatalf("review item: %v", item)
		}
	}
	a.expect("accept", "POST", "/api/analysis/results/bulk-accept", "RESEARCHER", map[string]any{"testSubjectIds": subjects}, 200, "")

	report := a.expect("report", "GET", "/api/experiments/"+w.exp+"/report", "VIEWER", nil, 200, "")
	codes := map[string]string{}
	for _, g := range report["groups"].([]any) {
		codes[g.(map[string]any)["id"].(string)] = g.(map[string]any)["code"].(string)
	}
	counted := 0
	for _, g := range report["sections"].([]any)[0].(map[string]any)["groups"].([]any) {
		group := g.(map[string]any)
		if n := group["n"].(float64); n > 0 {
			counted += int(n)
			if codes[group["groupId"].(string)] != "G1" && codes[group["groupId"].(string)] != "G2" {
				t.Fatalf("group with a measurement: %v", group)
			}
		}
	}
	if counted != 2 {
		t.Fatalf("report counts %d accepted animals: %v", counted, report)
	}
	var trialRows int
	if err := a.pool.QueryRow(ctx, `SELECT count(*) FROM misko.report_accepted_metrics m
		JOIN misko.analysis_jobs j ON j.id = m.result_job_id WHERE j.requester_kind = 'TRIAL'`).Scan(&trialRows); err != nil || trialRows != 0 {
		t.Fatalf("trial rows in the report view: %d %v", trialRows, err)
	}
	summary := a.expect("dashboard", "GET", "/api/dashboard/summary", "VIEWER", nil, 200, "")
	if activity := summary["recentActivity"].([]any); len(activity) != 2 {
		t.Fatalf("dashboard activity lists trial jobs: %v", activity)
	}

	// Reanalysis goes through the port too; choosing the newer job reopens the review.
	manual := a.expect("reanalyse", "POST", "/api/analysis/test-subjects/"+subjects[0]+"/runs", "TECHNICIAN", map[string]any{}, 201, "")
	a.expect("reanalyse again", "POST", "/api/analysis/test-subjects/"+subjects[0]+"/runs", "TECHNICIAN", map[string]any{}, 409, "analysis.runPending")
	completeClaim(t, a, objects, worker, claimOne(t, a, worker), 6_000_000, swim())
	result := a.expect("result", "GET", "/api/analysis/results/"+subjects[0], "VIEWER", nil, 200, "")
	if result["status"] != "ACCEPTED" || result["latestRunId"] != manual["id"] || result["newerRunAvailable"] != true || len(result["runs"].([]any)) != 2 {
		t.Fatalf("newer job: %v", result)
	}
	chosen := a.expect("choose", "POST", "/api/analysis/results/"+subjects[0]+"/run", "TECHNICIAN", map[string]any{"jobId": manual["id"]}, 200, "")
	if chosen["status"] != "PENDING" || chosen["run"].(map[string]any)["id"] != manual["id"] {
		t.Fatalf("chosen: %v", chosen)
	}
	trialJob := detail["analyses"].([]any)[0].(map[string]any)["jobId"]
	a.expect("a trial job is no run of an animal", "POST", "/api/analysis/results/"+subjects[0]+"/run", "TECHNICIAN", map[string]any{"jobId": trialJob}, 404, "analysis.runNotFound")
}

// An arena calibration triggers an automatic job only for animals without
// one on the current video; an animal's own override triggers a new one.
func TestArenaCalibrationSubmitsOnceAndOverridesSubmitAgain(t *testing.T) {
	ctx := context.Background()
	a := newStudyAPI(t, newStudyObjects())
	w := newStudyWorld(t, a.pool, "EXP-ONCE")
	m1 := w.allocate("G1", "M-1")
	w.start()
	test, subjects := w.session("Day 14", m1)
	tick := func() {
		t.Helper()
		if err := a.api.AnalysisTick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	jobs := func() int {
		t.Helper()
		return len(a.expect("runs", "GET", "/api/tests/"+test+"/analysis-runs", "VIEWER", nil, 200, "")["data"].([]any))
	}
	tick()
	tick()
	if n := jobs(); n != 1 {
		t.Fatalf("one automatic job: %d", n)
	}
	// A corrected arena default does not reanalyse silently.
	w.exec(`INSERT INTO misko.calibrations (environment_revision_id, supersedes_id, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height,
		measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, created_by)
		SELECT environment_revision_id, id, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height, measurement_plane, fit_points,
		check_points, transform, 0.1, 0.1, 0.2, 2, 'v1', 'VALID', created_by FROM misko.calibrations WHERE environment_revision_id = $1`, w.arenas[0])
	tick()
	if n := jobs(); n != 1 {
		t.Fatalf("an arena correction reanalysed: %d", n)
	}
	// The animal's own override on this video does.
	w.exec(`INSERT INTO misko.calibrations (environment_revision_id, test_subject_id, recording_id, reference_frame_us, camera_id, frame_width, frame_height,
		crop_x, crop_y, crop_width, crop_height, measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm,
		tolerance_cm, algorithm_version, status, created_by)
		SELECT ts.environment_revision_id, ts.id, r.id, 0, 'cam', 1920, 1080, 0, 0, 640, 1080, 'ARENA_FLOOR', '[{},{},{},{}]', '[{},{},{}]', '{1,0,0,0,1,0,0,0,1}',
		0.1, 0.1, 0.2, 2, 'v1', 'VALID', $2
		FROM misko.test_subjects ts JOIN misko.current_recordings r ON r.test_id = ts.test_id WHERE ts.id = $1`, subjects[0], studyAuthor)
	tick()
	tick()
	if n := jobs(); n != 2 {
		t.Fatalf("an override submits once more: %d", n)
	}
}
