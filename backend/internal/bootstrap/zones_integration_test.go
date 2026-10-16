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

// poolCalibration maps a 1920 x 1080 frame filmed at 10 px per cm to
// centimetres from the pool centre at pixel (960, 540).
func poolCalibration(zones []any) map[string]any {
	pt := func(px, py float64) map[string]any {
		return map[string]any{"pixelX": px, "pixelY": py, "worldX": (px - 960) / 10, "worldY": (py - 540) / 10}
	}
	body := map[string]any{"cameraId": "cam-pool", "frameWidth": 1920, "frameHeight": 1080, "crop": map[string]any{"x": 0, "y": 0, "width": 1920, "height": 1080},
		"measurementPlane": "APPARATUS_TOP",
		"fitPoints":        []any{pt(460, 140), pt(1460, 140), pt(1460, 940), pt(460, 940)},
		"checkPoints":      []any{pt(960, 540), pt(700, 700), pt(1200, 300)}}
	if zones != nil {
		body["zones"] = zones
	}
	return body
}

// No paradigm of the catalog declares calibrated zones: a water maze pool is
// calibrated without any, drawn shapes are refused, and the claim carries an
// empty zone list that the engine runs with.
func TestPoolWithoutCalibratedZonesOverHTTP(t *testing.T) {
	ctx := context.Background()
	objects := newStudyObjects()
	a := newStudyAPI(t, objects)
	w := newStudyWorld(t, a.pool, "EXP-Z")
	animal := w.allocate("G1", "Z-1")
	w.start()

	// A pool, a test of it with one animal and a verified video.
	env := w.id("INSERT INTO misko.environments (name, paradigm_key) VALUES ('Pool 1', 'MWM') RETURNING id")
	arena := w.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'MWM', 1, 1, '{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30}', $2) RETURNING id`, env, studyAuthor)
	protocol := w.id("INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Memory') RETURNING id", w.exp)
	var version string
	if err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
			VALUES ($1, $2, 1, 1, $3) RETURNING id`, w.exp, protocol, studyAuthor).Scan(&version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
			VALUES ($1, 1, 'MWM', 1, 'ACQUISITION', 1, '{}')`, version)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	test := w.id(`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
		VALUES ($1, $2, $3, 1, 'MWM', 1, 1, now() - interval '1 hour', $4) RETURNING id`, w.exp, w.timepoints["Day 14"], version, studyAuthor)
	subject := w.id(`INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
		VALUES ($1, $2, $3, $4, 'MWM', 1, $5) RETURNING id`, w.exp, test, animal, arena, studyAuthor)
	w.exec("UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() - interval '30 minutes' WHERE id = $1", test)
	asset := w.id(`INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ORIGINAL', 'misko-test-videos', $1, 'video/mp4', 4096, 1, 'VERIFIED', 42, now(), $2) RETURNING id`, "videos/"+w.exp+"/pool.mp4", studyAuthor)
	w.exec(`INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, clip_start_us, clip_end_us, created_by)
		VALUES ($1, $2, $3, 0, 6000000, $4)`, w.exp, test, asset, studyAuthor)

	// The pool has no calibrated zones: a drawn shape is refused, whatever its key.
	calibrations := "/api/environments/" + env + "/revisions/1/calibrations"
	circle := map[string]any{"key": "platform", "circle": map[string]any{"x": 1260, "y": 840, "radius": 50}}
	a.expect("derived zone drawn", "POST", calibrations, "SUPERADMIN", poolCalibration([]any{circle}), 400, "calibration.unknownZone")
	arm := map[string]any{"key": "arms", "polygon": []any{map[string]any{"x": 300, "y": 480}, map[string]any{"x": 900, "y": 480}, map[string]any{"x": 900, "y": 600}}}
	a.expect("undeclared zone", "POST", calibrations, "SUPERADMIN", poolCalibration([]any{arm}), 400, "calibration.unknownZone")
	status := "/api/tests/" + test + "/subjects/" + subject + "/calibration-status"
	if s := a.expect("status before", "GET", status, "VIEWER", nil, 200, ""); s["status"] != "WAITING_FOR_CALIBRATION" {
		t.Fatalf("before the pool is calibrated: %v", s)
	}
	created := a.expect("calibrate the pool", "POST", calibrations, "SUPERADMIN", poolCalibration(nil), 201, "")
	if zones, ok := created["zones"].([]any); !ok || len(zones) != 0 {
		t.Fatalf("calibration zones: %v", created["zones"])
	}
	s := a.expect("status after", "GET", status, "VIEWER", nil, 200, "")
	if zones, ok := s["calibration"].(map[string]any)["zones"].([]any); s["status"] != "CALIBRATED" || !ok || len(zones) != 0 {
		t.Fatalf("calibrated pool: %v", s)
	}

	// A worker tracks the pool; the claim carries an empty zone list.
	register := map[string]any{"name": "pool tracker", "modelVersion": "tracker 2.0", "capabilities": []any{map[string]any{"paradigmKey": "MWM", "paradigmVersion": 1}}}
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
	claimed, ok := claim["calibration"].(map[string]any)["zones"].([]any)
	required, _ := claim["contract"].(map[string]any)["requiredZones"].([]any)
	if run["testSubjectId"] != subject || !ok || len(claimed) != 0 || len(required) != 0 {
		t.Fatalf("claimed zones: %v", claim)
	}

	// The animal swims 20 cm east along y = 30 cm, onto the platform edge at 1.5 s.
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
		ts, xs, ys = append(ts, int64(i)*100_000), append(xs, 10+float64(i)), append(ys, 30)
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
	if math.Abs(value("path_length_cm")-20) > 1e-9 || math.Abs(value("duration_s")-2) > 1e-9 || value("platform_crossings_count") != 1 {
		t.Fatalf("water maze metrics: %v", metrics)
	}
	if metrics["escape_latency_s"]["missingReason"] != "EVENT_NOT_OBSERVED" {
		t.Fatalf("a 0.5 s visit is no escape: %v", metrics["escape_latency_s"])
	}
}
