//go:build integration

package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"testing"

	mediadomain "github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/jackc/pgx/v5"
)

// trackedCase is one paradigm analysed end to end: an arena calibrated over
// HTTP with its required zones, a worker with the capability, a scheduler
// tick, a claim that carries the zones, and a hand-made trajectory whose
// metrics and events the engine computes.
type trackedCase struct {
	paradigm, trialType string
	// arena holds the environment revision's apparatus values; session the
	// protocol step's session values.
	arena, session string
	// world maps a full-frame pixel to the calibration's centimetres; plane
	// is the measurement plane the calibration is drawn on.
	world func(px, py float64) (float64, float64)
	plane string
	// fit and check are pixel positions of the calibration points.
	fit, check [][2]float64
	// zones are drawn in pixels.
	zones         []any
	requiredZones []string
	clipUs        int64
	samples       [][3]float64 // t (s), x (cm), y (cm)
}

type trackedResult struct {
	metrics map[string]map[string]any
	events  []map[string]any
}

func analyseTracked(t *testing.T, code string, c trackedCase) trackedResult {
	t.Helper()
	ctx := context.Background()
	objects := newStudyObjects()
	a := newStudyAPI(t, objects)
	w := newStudyWorld(t, a.pool, code)
	animal := w.allocate("G1", code+"-1")
	w.start()

	env := w.id("INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, $2) RETURNING id", c.paradigm+" rig", c.paradigm)
	arena := w.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, $2, 1, 1, $3, $4) RETURNING id`, env, c.paradigm, c.arena, studyAuthor)
	protocol := w.id("INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, $2) RETURNING id", w.exp, c.paradigm)
	var version string
	if err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
			VALUES ($1, $2, 1, 1, $3) RETURNING id`, w.exp, protocol, studyAuthor).Scan(&version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
			VALUES ($1, 1, $2, 1, $3, 1, $4)`, version, c.paradigm, c.trialType, c.session)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	test := w.id(`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
		VALUES ($1, $2, $3, 1, $4, 1, 1, now() - interval '1 hour', $5) RETURNING id`, w.exp, w.timepoints["Day 14"], version, c.paradigm, studyAuthor)
	subject := w.id(`INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
		VALUES ($1, $2, $3, $4, $5, 1, $6) RETURNING id`, w.exp, test, animal, arena, c.paradigm, studyAuthor)
	w.exec("UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() - interval '30 minutes' WHERE id = $1", test)
	asset := w.id(`INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ORIGINAL', 'misko-test-videos', $1, 'video/mp4', 4096, 1, 'VERIFIED', 42, now(), $2) RETURNING id`, "videos/"+w.exp+"/trial.mp4", studyAuthor)
	w.exec(`INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, clip_start_us, clip_end_us, created_by)
		VALUES ($1, $2, $3, 0, $4, $5)`, w.exp, test, asset, c.clipUs, studyAuthor)

	// The rig is calibrated over HTTP: without its zones it is refused, with
	// them the animal's status is calibrated.
	points := func(pixels [][2]float64) []any {
		var out []any
		for _, p := range pixels {
			x, y := c.world(p[0], p[1])
			out = append(out, map[string]any{"pixelX": p[0], "pixelY": p[1], "worldX": x, "worldY": y})
		}
		return out
	}
	body := map[string]any{"cameraId": "cam-" + c.paradigm, "frameWidth": 1920, "frameHeight": 1080, "crop": map[string]any{"x": 0, "y": 0, "width": 1920, "height": 1080},
		"measurementPlane": c.plane, "fitPoints": points(c.fit), "checkPoints": points(c.check)}
	calibrations := "/api/environments/" + env + "/revisions/1/calibrations"
	a.expect("calibrate without zones", "POST", calibrations, "SUPERADMIN", body, 400, "calibration.missingZone")
	body["zones"] = c.zones
	created := a.expect("calibrate the rig", "POST", calibrations, "SUPERADMIN", body, 201, "")
	if zones, _ := created["zones"].([]any); len(zones) != len(c.zones) || created["measurementPlane"] != c.plane {
		t.Fatalf("calibration: %v", created)
	}
	status := a.expect("status", "GET", "/api/tests/"+test+"/subjects/"+subject+"/calibration-status", "VIEWER", nil, 200, "")
	if status["status"] != "CALIBRATED" {
		t.Fatalf("calibrated rig: %v", status)
	}

	// A worker with the capability claims the run; the claim carries the zones.
	register := map[string]any{"name": c.paradigm + " tracker", "modelVersion": "tracker 3.0", "capabilities": []any{map[string]any{"paradigmKey": c.paradigm, "paradigmVersion": 1}}}
	token := a.expect("register", "POST", "/api/analysis/workers", "SUPERADMIN", register, 201, "")["token"].(string)
	worker := "Worker " + token
	if err := a.api.AnalysisTick(ctx); err != nil {
		t.Fatal(err)
	}
	code200, claim := a.send("POST", "/api/worker/claim", worker, map[string]any{})
	if code200 != 200 {
		t.Fatalf("claim: %d %v", code200, claim)
	}
	run := claim["run"].(map[string]any)
	var claimedKeys, requiredKeys []string
	for _, z := range claim["calibration"].(map[string]any)["zones"].([]any) {
		zone := z.(map[string]any)
		if zone["centimeters"] == nil || zone["pixels"] == nil {
			t.Fatalf("claimed zone without both shapes: %v", zone)
		}
		claimedKeys = append(claimedKeys, zone["key"].(string))
	}
	for _, k := range claim["contract"].(map[string]any)["requiredZones"].([]any) {
		requiredKeys = append(requiredKeys, k.(string))
	}
	slices.Sort(claimedKeys)
	wantKeys := slices.Sorted(slices.Values(c.requiredZones))
	if run["testSubjectId"] != subject || run["paradigmKey"] != c.paradigm || !slices.Equal(claimedKeys, wantKeys) || !slices.Equal(requiredKeys, c.requiredZones) {
		t.Fatalf("claim zones %v required %v: %v", claimedKeys, requiredKeys, claim)
	}

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
	for _, s := range c.samples {
		ts, xs, ys = append(ts, int64(math.Round(s[0]*1e6))), append(xs, s[1]), append(ys, s[2])
		tracked, confidence = append(tracked, true), append(confidence, 0.95)
	}
	trajectory, _ := json.Marshal(map[string]any{"schema": "misko.trajectory.v1", "runId": runID, "attempt": 1, "durationUs": c.clipUs,
		"samples": map[string]any{"tUs": ts, "xCm": xs, "yCm": ys, "tracked": tracked, "confidence": confidence}})
	if code, body := a.send("POST", runPath+"/outputs", worker, map[string]any{"attempt": 1, "kind": "TRAJECTORY", "fileName": "trajectory.json",
		"contentType": "application/json", "sizeBytes": len(trajectory), "crc32c": mediadomain.FormatCRC32C(6)}); code != 201 {
		t.Fatalf("trajectory output: %d %v", code, body)
	}
	objects.put(prefix+"trajectory.json", trajectory, int64(len(trajectory)), 6, "application/json")
	result := map[string]any{"attempt": 1, "modelVersion": "tracker 3.0",
		"artifacts": []any{map[string]any{"kind": "ANALYZED_VIDEO", "objectName": prefix + "overlay.mp4"}, map[string]any{"kind": "TRAJECTORY", "objectName": prefix + "trajectory.json"}},
		"pair":      map[string]any{"analyzedObjectName": prefix + "overlay.mp4", "sourceOffsetUs": 0, "outputOffsetUs": 0, "timeMappingVersion": "identity-v1"}}
	if code, body := a.send("POST", runPath+"/result", worker, result); code != 200 || body["status"] != "SUCCEEDED" {
		t.Fatalf("result: %d %v", code, body)
	}
	detail := a.expect("run detail", "GET", "/api/analysis-runs/"+runID, "VIEWER", nil, 200, "")
	if _, ok := detail["method"]; ok {
		t.Fatalf("runs no longer carry a method: %v", detail)
	}
	out := trackedResult{metrics: map[string]map[string]any{}}
	r := detail["result"].(map[string]any)
	for _, m := range r["metrics"].([]any) {
		out.metrics[m.(map[string]any)["key"].(string)] = m.(map[string]any)
	}
	for _, e := range r["events"].([]any) {
		out.events = append(out.events, e.(map[string]any))
	}
	// The result enters the review queue like any automated run.
	review := a.expect("review", "GET", "/api/analysis/results/"+subject, "VIEWER", nil, 200, "")
	if reviewed := review["run"].(map[string]any); review["status"] != "PENDING" || reviewed["id"] != runID {
		t.Fatalf("review: %v", review)
	} else if _, ok := reviewed["method"]; ok {
		t.Fatalf("reviewed runs no longer carry a method: %v", reviewed)
	}
	return out
}

func (r trackedResult) value(t *testing.T, key string, want float64) {
	t.Helper()
	v, ok := r.metrics[key]["value"].(float64)
	if !ok || math.Abs(v-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", key, r.metrics[key], want)
	}
}

func (r trackedResult) missing(t *testing.T, key, reason string) {
	t.Helper()
	if m := r.metrics[key]; m["value"] != nil || m["missingReason"] != reason {
		t.Errorf("%s = %v, want missing %s", key, m, reason)
	}
}

// eventList renders events as "type[:label]@startUs-endUs" for comparison.
func (r trackedResult) eventList() []string {
	var out []string
	for _, e := range r.events {
		s := e["type"].(string)
		if label, ok := e["label"].(string); ok {
			s += ":" + label
		}
		out = append(out, fmt.Sprintf("%s@%d-%d", s, int64(e["startUs"].(float64)), int64(e["endUs"].(float64))))
	}
	return out
}

// steadySamples returns samples every step seconds from `from` to `to` at one position.
func steadySamples(from, to, step, x, y float64) [][3]float64 {
	var out [][3]float64
	n := int(math.Round((to - from) / step))
	for i := 0; i <= n; i++ {
		out = append(out, [3]float64{from + float64(i)*step, x, y})
	}
	return out
}

func polygon(points ...[2]float64) []any {
	var out []any
	for _, p := range points {
		out = append(out, map[string]any{"x": p[0], "y": p[1]})
	}
	return out
}

// A top view of a 50 x 50 cm arena at 10 px per cm, origin at the corner
// pixel (460, 140). The novel object is a 4 cm disk at (10, 10), the familiar
// one the square [36, 44] x [6, 14]. The subject sits at the novel object for
// 0-1.0 s, at the familiar object for 1.1-2.0 s and in the open from 2.1 s:
// novel [0, 1.1) = 1.1 s, familiar [1.1, 2.1) = 1.0 s, DI = 0.1 / 2.1.
func TestNovelObjectAnalysedOverHTTP(t *testing.T) {
	samples := append(append(steadySamples(0, 1, 0.1, 10, 10), steadySamples(1.1, 2, 0.1, 40, 10)...), steadySamples(2.1, 3, 0.1, 25, 25)...)
	r := analyseTracked(t, "EXP-NOR", trackedCase{
		paradigm: "NOVEL_OBJECT", trialType: "TEST", arena: `{"arena_width_cm": 50, "arena_height_cm": 50}`, session: "{}",
		world: func(px, py float64) (float64, float64) { return (px - 460) / 10, (py - 140) / 10 }, plane: "ARENA_FLOOR",
		fit: [][2]float64{{460, 140}, {960, 140}, {960, 640}, {460, 640}}, check: [][2]float64{{710, 390}, {560, 540}, {860, 240}},
		zones: []any{
			map[string]any{"key": "novel_object", "circle": map[string]any{"x": 560, "y": 240, "radius": 40}},
			map[string]any{"key": "familiar_object", "polygon": polygon([2]float64{820, 200}, [2]float64{900, 200}, [2]float64{900, 280}, [2]float64{820, 280})},
		},
		requiredZones: []string{"novel_object", "familiar_object"},
		clipUs:        3_000_000, samples: samples,
	})
	r.value(t, "novel_object_time_s", 1.1)
	r.value(t, "familiar_object_time_s", 1.0)
	r.value(t, "total_exploration_time_s", 2.1)
	r.value(t, "discrimination_index", 0.1/2.1)
	if got, want := r.eventList(), []string{"exploration:novel@0-1100000", "exploration:familiar@1100000-2100000"}; !slices.Equal(got, want) {
		t.Errorf("events %v, want %v", got, want)
	}
}

// A side view of the pole at 10 px per cm: x = 0 at pixel column 960, the
// base (y = 0) at pixel row 1000, y up. The subject turns at 1.5 s (2 cm
// below its 47 cm peak) and walks into the base at 3.5 s: descent 2 s at
// 50 / 2 = 25 cm/s, no fall.
func TestPoleAnalysedOverHTTP(t *testing.T) {
	r := analyseTracked(t, "EXP-POLE", trackedCase{
		paradigm: "POLE", trialType: "STANDARD", arena: `{"pole_length_cm": 50}`, session: `{"max_trial_duration_s": 60}`,
		world: func(px, py float64) (float64, float64) { return (px - 960) / 10, (1000 - py) / 10 }, plane: "APPARATUS_SIDE",
		fit: [][2]float64{{760, 500}, {1160, 500}, {1160, 1000}, {760, 1000}}, check: [][2]float64{{960, 750}, {860, 600}, {1060, 900}},
		zones: []any{
			map[string]any{"key": "pole", "polygon": polygon([2]float64{940, 500}, [2]float64{980, 500}, [2]float64{980, 1000}, [2]float64{940, 1000})},
			map[string]any{"key": "base", "polygon": polygon([2]float64{660, 990}, [2]float64{1260, 990}, [2]float64{1260, 1050}, [2]float64{660, 1050})},
		},
		requiredZones: []string{"pole", "base"},
		clipUs:        5_000_000,
		samples:       [][3]float64{{0, 0, 45}, {0.5, 0, 47}, {1, 0, 46}, {1.5, 0, 45}, {2, 0, 35}, {2.5, 0, 20}, {3, 0, 8}, {3.5, 0, 0.5}, {4, 0, 0.5}},
	})
	r.value(t, "t_turn_s", 1.5)
	r.value(t, "t_total_s", 3.5)
	r.value(t, "descent_time_s", 2)
	r.value(t, "descent_speed_cm_s", 25)
	r.value(t, "fall_detected", 0)
	if got, want := r.eventList(), []string{"turn_complete@1500000-1500000", "base_reached@3500000-3500000"}; !slices.Equal(got, want) {
		t.Errorf("events %v, want %v", got, want)
	}
}

// A side view of the lane at 10 px per cm: x = 0 (the grid end) at pixel
// column 200, y = 0 at pixel row 900, y up; the grid covers x in [0, 5]. On a
// belt at 10 cm/s the subject touches the grid at 10 s (0.5 s, a shock) and
// stays on it from 20 s to 26.5 s (exhaustion at 20 s): run 20 s, 200 cm,
// one shock before the end.
func TestTreadmillAnalysedOverHTTP(t *testing.T) {
	var samples [][3]float64
	for _, part := range [][][3]float64{steadySamples(0, 9.5, 0.5, 20, 3), steadySamples(10, 10, 1, 2, 3), steadySamples(10.5, 19.5, 0.5, 20, 3),
		steadySamples(20, 26, 0.5, 2, 3), steadySamples(26.5, 30, 0.5, 20, 3)} {
		samples = append(samples, part...)
	}
	r := analyseTracked(t, "EXP-RUN", trackedCase{
		paradigm: "TREADMILL", trialType: "FIXED_SPEED", arena: `{"lane_length_cm": 40}`,
		session: `{"accelerating": 0, "start_speed_cm_s": 10, "end_speed_cm_s": 40, "acceleration_duration_s": 600, "max_trial_duration_s": 60}`,
		world:   func(px, py float64) (float64, float64) { return (px - 200) / 10, (900 - py) / 10 }, plane: "APPARATUS_SIDE",
		fit: [][2]float64{{200, 700}, {600, 700}, {600, 900}, {200, 900}}, check: [][2]float64{{400, 800}, {300, 750}, {500, 850}},
		zones: []any{
			map[string]any{"key": "shock_grid", "polygon": polygon([2]float64{200, 800}, [2]float64{250, 800}, [2]float64{250, 920}, [2]float64{200, 920})},
		},
		requiredZones: []string{"shock_grid"},
		clipUs:        30_000_000, samples: samples,
	})
	r.value(t, "latency_to_exhaustion_s", 20)
	r.value(t, "run_time_s", 20)
	r.value(t, "run_distance_cm", 200)
	r.value(t, "shock_count", 1)
	if got, want := r.eventList(), []string{"shock@10000000-10000000", "exhaustion@20000000-20000000", "shock@20000000-20000000"}; !slices.Equal(got, want) {
		t.Errorf("events %v, want %v", got, want)
	}
}
