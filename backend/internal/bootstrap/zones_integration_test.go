//go:build integration

package bootstrap

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	mediadomain "github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/jackc/pgx/v5"
)

// zonesPx is an elevated plus maze drawn on a 1920 x 1080 frame filmed at
// 10 px per cm: open arms left and right, closed arms up and down, the center
// square where they meet.
func zonesPx() []any {
	rect := func(x0, y0, x1, y1 float64) []any {
		return []any{map[string]any{"x": x0, "y": y0}, map[string]any{"x": x1, "y": y0}, map[string]any{"x": x1, "y": y1}, map[string]any{"x": x0, "y": y1}}
	}
	return []any{
		map[string]any{"key": "open_arms", "polygon": rect(300, 480, 900, 600)},
		map[string]any{"key": "open_arms", "polygon": rect(1020, 480, 1620, 600)},
		map[string]any{"key": "closed_arms", "polygon": rect(900, 0, 1020, 480)},
		map[string]any{"key": "closed_arms", "polygon": rect(900, 600, 1020, 1080)},
		map[string]any{"key": "center", "polygon": rect(900, 480, 1020, 600)},
	}
}

// mazeCalibration maps pixels to centimetres at 10 px per cm.
func mazeCalibration(zones []any) map[string]any {
	pt := func(px, py float64) map[string]any {
		return map[string]any{"pixelX": px, "pixelY": py, "worldX": px / 10, "worldY": py / 10}
	}
	body := map[string]any{"cameraId": "cam-maze", "frameWidth": 1920, "frameHeight": 1080, "crop": map[string]any{"x": 0, "y": 0, "width": 1920, "height": 1080},
		"measurementPlane": "APPARATUS_TOP",
		"fitPoints":        []any{pt(100, 100), pt(1800, 100), pt(1800, 1000), pt(100, 1000)},
		"checkPoints":      []any{pt(960, 540), pt(400, 700), pt(1500, 300)}}
	if zones != nil {
		body["zones"] = zones
	}
	return body
}

// An elevated plus maze: its zones are drawn with the arena's calibration,
// travel with the claim and produce the open arm metrics of the result.
func TestCalibratedZonesOverHTTP(t *testing.T) {
	ctx := context.Background()
	objects := newStudyObjects()
	a := newStudyAPI(t, objects)
	w := newStudyWorld(t, a.pool, "EXP-Z")
	animal := w.allocate("G1", "Z-1")
	w.start()

	// An EPM arena, a test of it with one animal and a verified video.
	env := w.id("INSERT INTO misko.environments (name, paradigm_key) VALUES ('Maze 1', 'EPM') RETURNING id")
	arena := w.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'EPM', 1, 1, '{"arm_length_cm": 60, "arm_width_cm": 12}', $2) RETURNING id`, env, studyAuthor)
	protocol := w.id("INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Anxiety') RETURNING id", w.exp)
	var version string
	if err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
			VALUES ($1, $2, 1, 1, $3) RETURNING id`, w.exp, protocol, studyAuthor).Scan(&version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
			VALUES ($1, 1, 'EPM', 1, 'STANDARD', 1, '{}')`, version)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	test := w.id(`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
		VALUES ($1, $2, $3, 1, 'EPM', 1, 1, now() - interval '1 hour', $4) RETURNING id`, w.exp, w.timepoints["Day 14"], version, studyAuthor)
	subject := w.id(`INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
		VALUES ($1, $2, $3, $4, 'EPM', 1, $5) RETURNING id`, w.exp, test, animal, arena, studyAuthor)
	w.exec("UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() - interval '30 minutes' WHERE id = $1", test)
	asset := w.id(`INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ORIGINAL', 'misko-test-videos', $1, 'video/mp4', 4096, 1, 'VERIFIED', 42, now(), $2) RETURNING id`, "videos/"+w.exp+"/maze.mp4", studyAuthor)
	w.exec(`INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, clip_start_us, clip_end_us, created_by)
		VALUES ($1, $2, $3, 0, 6000000, $4)`, w.exp, test, asset, studyAuthor)

	// The arena's calibration must draw every required zone of the paradigm.
	calibrations := "/api/environments/" + env + "/revisions/1/calibrations"
	a.expect("no zones", "POST", calibrations, "SUPERADMIN", mazeCalibration(nil), 400, "calibration.missingZone")
	unknown := append(zonesPx(), map[string]any{"key": "platform", "circle": map[string]any{"x": 100, "y": 100, "radius": 10}})
	a.expect("unknown zone", "POST", calibrations, "SUPERADMIN", mazeCalibration(unknown), 400, "calibration.unknownZone")
	outside := append(zonesPx(), map[string]any{"key": "center", "circle": map[string]any{"x": 1900, "y": 540, "radius": 40}})
	a.expect("zone off the frame", "POST", calibrations, "SUPERADMIN", mazeCalibration(outside), 400, "calibration.zoneOutsideFrame")
	line := append(zonesPx(), map[string]any{"key": "center", "polygon": []any{map[string]any{"x": 1, "y": 1}, map[string]any{"x": 2, "y": 2}}})
	a.expect("two vertices", "POST", calibrations, "SUPERADMIN", mazeCalibration(line), 400, "calibration.invalidZone")
	status := "/api/tests/" + test + "/subjects/" + subject + "/calibration-status"
	if s := a.expect("status before", "GET", status, "VIEWER", nil, 200, ""); s["status"] != "WAITING_FOR_CALIBRATION" {
		t.Fatalf("before the maze is calibrated: %v", s)
	}
	created := a.expect("calibrate the maze", "POST", calibrations, "SUPERADMIN", mazeCalibration(zonesPx()), 201, "")
	zones := created["zones"].([]any)
	center := zones[0].(map[string]any)
	cm := center["centimeters"].(map[string]any)["polygon"].([]any)[0].(map[string]any)
	if len(zones) != 5 || center["key"] != "center" || center["pixels"].(map[string]any)["polygon"] == nil ||
		math.Abs(cm["x"].(float64)-90) > 1e-6 || math.Abs(cm["y"].(float64)-48) > 1e-6 {
		t.Fatalf("calibration zones: %v", created["zones"])
	}
	s := a.expect("status after", "GET", status, "VIEWER", nil, 200, "")
	if s["status"] != "CALIBRATED" || len(s["calibration"].(map[string]any)["zones"].([]any)) != 5 {
		t.Fatalf("calibrated maze: %v", s)
	}

	// A worker tracks the maze; the claim carries the zones.
	register := map[string]any{"name": "maze tracker", "modelVersion": "tracker 2.0", "capabilities": []any{map[string]any{"paradigmKey": "EPM", "paradigmVersion": 1}}}
	token := a.expect("register", "POST", "/api/analysis/workers", "SUPERADMIN", register, 201, "")["token"].(string)
	worker := "Worker " + token
	if err := a.api.AnalysisTick(ctx); err != nil {
		t.Fatal(err)
	}
	code, claim := a.send("POST", "/api/worker/claim", worker, map[string]any{})
	if code != 200 {
		t.Fatalf("claim: %d %v", code, claim)
	}
	run := claim["run"].(map[string]any)
	claimed := claim["calibration"].(map[string]any)["zones"].([]any)
	arm := claimed[3].(map[string]any)
	if run["testSubjectId"] != subject || len(claimed) != 5 || arm["key"] != "open_arms" ||
		arm["pixels"].(map[string]any)["polygon"].([]any)[0].(map[string]any)["x"] != float64(300) ||
		math.Abs(arm["centimeters"].(map[string]any)["polygon"].([]any)[0].(map[string]any)["x"].(float64)-30) > 1e-6 {
		t.Fatalf("claimed zones: %v", claimed)
	}

	// The animal walks 10 cm along the left open arm for two seconds.
	runID, prefix := run["id"].(string), claim["outputPrefix"].(string)
	runPath := "/api/worker/runs/" + runID
	if code, body := a.send("POST", runPath+"/outputs", worker, map[string]any{"attempt": 1, "kind": "ANALYZED_VIDEO", "fileName": "overlay.mp4",
		"contentType": "video/mp4", "sizeBytes": 2048, "crc32c": mediadomain.FormatCRC32C(5)}); code != 201 {
		t.Fatalf("overlay output: %d %v", code, body)
	}
	objects.put(prefix+"overlay.mp4", nil, 2048, 5, "video/mp4")
	var ts []int64
	var xs, ys, confidence []float64
	var tracked []bool
	for i := 0; i <= 20; i++ {
		ts, xs, ys = append(ts, int64(i)*100_000), append(xs, 40+float64(i)*0.5), append(ys, 54)
		tracked, confidence = append(tracked, true), append(confidence, 0.95)
	}
	trajectory, _ := json.Marshal(map[string]any{"schema": "misko.trajectory.v1", "runId": runID, "attempt": 1, "durationUs": 6_000_000,
		"samples": map[string]any{"tUs": ts, "xCm": xs, "yCm": ys, "tracked": tracked, "confidence": confidence}})
	if code, body := a.send("POST", runPath+"/outputs", worker, map[string]any{"attempt": 1, "kind": "TRAJECTORY", "fileName": "trajectory.json",
		"contentType": "application/json", "sizeBytes": len(trajectory), "crc32c": mediadomain.FormatCRC32C(6)}); code != 201 {
		t.Fatalf("trajectory output: %d %v", code, body)
	}
	objects.put(prefix+"trajectory.json", trajectory, int64(len(trajectory)), 6, "application/json")
	result := map[string]any{"attempt": 1, "modelVersion": "tracker 2.0",
		"artifacts": []any{map[string]any{"kind": "ANALYZED_VIDEO", "objectName": prefix + "overlay.mp4"}, map[string]any{"kind": "TRAJECTORY", "objectName": prefix + "trajectory.json"}},
		"pair":      map[string]any{"analyzedObjectName": prefix + "overlay.mp4", "sourceOffsetUs": 0, "outputOffsetUs": 0, "timeMappingVersion": "identity-v1"}}
	if code, body := a.send("POST", runPath+"/result", worker, result); code != 200 || body["status"] != "SUCCEEDED" {
		t.Fatalf("result: %d %v", code, body)
	}
	detail := a.expect("run detail", "GET", "/api/analysis-runs/"+runID, "VIEWER", nil, 200, "")
	metrics := map[string]map[string]any{}
	for _, m := range detail["result"].(map[string]any)["metrics"].([]any) {
		metrics[m.(map[string]any)["key"].(string)] = m.(map[string]any)
	}
	value := func(key string) float64 {
		v, ok := metrics[key]["value"].(float64)
		if !ok {
			t.Fatalf("metric %s: %v", key, metrics[key])
		}
		return v
	}
	if math.Abs(value("open_arm_time_s")-2) > 1e-9 || value("closed_arm_time_s") != 0 || value("center_time_s") != 0 ||
		math.Abs(value("open_arm_time_ratio")-1) > 1e-9 || math.Abs(value("distance_cm")-10) > 1e-9 {
		t.Fatalf("open arm metrics: %v", metrics)
	}
	if metrics["risk_assessment_count"]["missingReason"] != "NOT_SCORED" {
		t.Fatalf("unscored risk assessment: %v", metrics["risk_assessment_count"])
	}
}
