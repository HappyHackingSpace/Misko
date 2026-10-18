//go:build integration

package bootstrap

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	mediadomain "github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
)

// completeClaim does a worker's part for one claimed job: it uploads the
// overlay and a trajectory of the samples (t s, x cm, y cm) and submits the
// result.
func completeClaim(t *testing.T, a *studyAPI, objects *studyObjects, worker string, claim map[string]any, clipUs int64, samples [][3]float64) {
	t.Helper()
	job := claim["run"].(map[string]any)
	id, prefix := job["id"].(string), claim["outputPrefix"].(string)
	path := "/api/worker/runs/" + id
	if code, body := a.send("POST", path+"/outputs", worker, map[string]any{"attempt": 1, "kind": "ANALYZED_VIDEO", "fileName": "overlay.mp4",
		"contentType": "video/mp4", "sizeBytes": 2048, "crc32c": mediadomain.FormatCRC32C(5)}); code != 201 {
		t.Fatalf("overlay output: %d %v", code, body)
	}
	objects.put(prefix+"overlay.mp4", nil, 2048, 5, "video/mp4")
	var ts []int64
	var xs, ys, confidence []float64
	var tracked []bool
	for _, s := range samples {
		ts, xs, ys = append(ts, int64(math.Round(s[0]*1e6))), append(xs, s[1]), append(ys, s[2])
		tracked, confidence = append(tracked, true), append(confidence, 0.95)
	}
	trajectory, _ := json.Marshal(map[string]any{"schema": "misko.trajectory.v1", "runId": id, "attempt": 1, "durationUs": clipUs,
		"samples": map[string]any{"tUs": ts, "xCm": xs, "yCm": ys, "tracked": tracked, "confidence": confidence}})
	if code, body := a.send("POST", path+"/outputs", worker, map[string]any{"attempt": 1, "kind": "TRAJECTORY", "fileName": "trajectory.json",
		"contentType": "application/json", "sizeBytes": len(trajectory), "crc32c": mediadomain.FormatCRC32C(6)}); code != 201 {
		t.Fatalf("trajectory output: %d %v", code, body)
	}
	objects.put(prefix+"trajectory.json", trajectory, int64(len(trajectory)), 6, "application/json")
	result := map[string]any{"attempt": 1, "modelVersion": "tracker 3.0",
		"artifacts": []any{map[string]any{"kind": "ANALYZED_VIDEO", "objectName": prefix + "overlay.mp4"}, map[string]any{"kind": "TRAJECTORY", "objectName": prefix + "trajectory.json"}},
		"pair":      map[string]any{"analyzedObjectName": prefix + "overlay.mp4", "sourceOffsetUs": 0, "outputOffsetUs": 0, "timeMappingVersion": "identity-v1"}}
	if code, body := a.send("POST", path+"/result", worker, result); code != 200 || body["status"] != "SUCCEEDED" {
		t.Fatalf("result: %d %v", code, body)
	}
}

// claimOne claims the next job and fails unless there is one.
func claimOne(t *testing.T, a *studyAPI, worker string) map[string]any {
	t.Helper()
	code, claim := a.send("POST", "/api/worker/claim", worker, map[string]any{})
	if code != 200 {
		t.Fatalf("claim: %d %v", code, claim)
	}
	return claim
}

// novelObjectRig is a 50 x 50 cm arena seen from above at 10 px per cm with
// the origin at pixel (460, 140), calibrated with both object zones.
func novelObjectCalibration(withZones bool) map[string]any {
	point := func(px, py float64) map[string]any {
		return map[string]any{"pixelX": px, "pixelY": py, "worldX": (px - 460) / 10, "worldY": (py - 140) / 10}
	}
	body := map[string]any{"cameraId": "trial-cam", "frameWidth": 1920, "frameHeight": 1080, "crop": map[string]any{"x": 0, "y": 0, "width": 1920, "height": 1080},
		"measurementPlane": "ARENA_FLOOR", "referenceFrameUs": 500000,
		"fitPoints":   []any{point(460, 140), point(960, 140), point(960, 640), point(460, 640)},
		"checkPoints": []any{point(710, 390), point(560, 540), point(860, 240)}}
	if withZones {
		body["zones"] = []any{
			map[string]any{"key": "novel_object", "circle": map[string]any{"x": 560, "y": 240, "radius": 40}},
			map[string]any{"key": "familiar_object", "polygon": polygon([2]float64{820, 200}, [2]float64{900, 200}, [2]float64{900, 280}, [2]float64{820, 280})},
		}
	}
	return body
}

// novelObjectSamples: 1.1 s at the novel object, 1.0 s at the familiar one,
// then in the open.
func novelObjectSamples() [][3]float64 {
	return append(append(steadySamples(0, 1, 0.1, 10, 10), steadySamples(1.1, 2, 0.1, 40, 10)...), steadySamples(2.1, 3, 0.1, 25, 25)...)
}

func metricOf(t *testing.T, analysis map[string]any, key string) any {
	t.Helper()
	for _, m := range analysis["metrics"].([]any) {
		if m.(map[string]any)["key"] == key {
			return m.(map[string]any)["value"]
		}
	}
	t.Fatalf("metric %s missing from %v", key, analysis)
	return nil
}

// A paradigm trial end to end: create, upload, calibrate with zones on the
// trial's video, the automatic job, a worker's result, the result on the
// trial page, a second analysis with a changed threshold and both kept for
// comparison, and a soft delete that keeps the jobs.
func TestParadigmTrialOverHTTP(t *testing.T) {
	ctx := context.Background()
	objects := newStudyObjects()
	a := newStudyAPI(t, objects)
	w := &studyWorld{t: t, pool: a.pool}
	env := w.id("INSERT INTO misko.environments (name, paradigm_key) VALUES ('NOR box', 'NOVEL_OBJECT') RETURNING id")
	revision := w.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'NOVEL_OBJECT', 1, 1, '{"arena_width_cm": 50, "arena_height_cm": 50}', $2) RETURNING id`, env, studyAuthor)
	register := map[string]any{"name": "nor tracker", "modelVersion": "tracker 3.0", "capabilities": []any{map[string]any{"paradigmKey": "NOVEL_OBJECT", "paradigmVersion": 1}}}
	worker := "Worker " + a.expect("register", "POST", "/api/analysis/workers", "SUPERADMIN", register, 201, "")["token"].(string)

	create := map[string]any{"name": "Object test, camera 2", "paradigmKey": "NOVEL_OBJECT", "paradigmVersion": 1, "environmentRevisionId": revision}
	a.expect("viewer creates", "POST", "/api/trials", "VIEWER", create, 403, "auth.forbidden")
	a.expect("other paradigm", "POST", "/api/trials", "TECHNICIAN", map[string]any{"name": "x", "paradigmKey": "MWM", "paradigmVersion": 1,
		"environmentRevisionId": revision}, 400, "trial.paradigmMismatch")
	a.expect("no name", "POST", "/api/trials", "TECHNICIAN", map[string]any{"name": " ", "paradigmKey": "NOVEL_OBJECT", "paradigmVersion": 1,
		"environmentRevisionId": revision}, 400, "trial.invalidName")
	a.expect("unknown arena", "POST", "/api/trials", "TECHNICIAN", map[string]any{"name": "x", "paradigmKey": "NOVEL_OBJECT", "paradigmVersion": 1,
		"environmentRevisionId": env}, 404, "environment.revisionNotFound")
	trial := a.expect("create", "POST", "/api/trials", "TECHNICIAN", create, 201, "")
	id := trial["id"].(string)
	path := "/api/trials/" + id
	if list := a.expect("list", "GET", "/api/trials?search=CAMERA&paradigmKey=NOVEL_OBJECT", "VIEWER", nil, 200, ""); list["total"] != float64(1) ||
		list["data"].([]any)[0].(map[string]any)["arena"].(map[string]any)["environmentName"] != "NOR box" {
		t.Fatalf("list: %v", list)
	}
	if list := a.expect("search", "GET", "/api/trials?search=maze", "VIEWER", nil, 200, ""); list["total"] != float64(0) {
		t.Fatalf("search: %v", list)
	}
	a.expect("analyse without a video", "POST", path+"/analyses", "TECHNICIAN", map[string]any{}, 409, "trial.noVideo")

	// Upload, then verify.
	upload := map[string]any{"fileName": "trial.mp4", "contentType": "video/mp4", "sizeBytes": 4096, "crc32c": mediadomain.FormatCRC32C(1), "clipEndUs": 3_000_000}
	a.expect("bad clip", "POST", path+"/video", "TECHNICIAN", map[string]any{"fileName": "trial.mp4", "contentType": "video/mp4", "sizeBytes": 4096,
		"crc32c": mediadomain.FormatCRC32C(1), "clipEndUs": 0}, 400, "trial.invalidClip")
	a.expect("bad content type", "POST", path+"/video", "TECHNICIAN", map[string]any{"fileName": "trial.avi", "contentType": "video/avi", "sizeBytes": 4096,
		"crc32c": mediadomain.FormatCRC32C(1)}, 400, "trial.invalidUpload")
	started := a.expect("start upload", "POST", path+"/video", "TECHNICIAN", upload, 201, "")
	object := strings.TrimPrefix(started["upload"].(map[string]any)["url"].(string), "https://storage.test/")
	if !strings.HasPrefix(object, "trials/"+id+"/originals/") || started["video"].(map[string]any)["status"] != "PENDING" {
		t.Fatalf("upload: %v", started)
	}
	a.expect("finalize too early", "POST", path+"/video/finalize", "TECHNICIAN", map[string]any{}, 409, "media.uploadIncomplete")
	objects.put(object, nil, 4096, 1, "video/mp4")
	detail := a.expect("finalize", "POST", path+"/video/finalize", "TECHNICIAN", map[string]any{}, 200, "")
	if detail["video"].(map[string]any)["status"] != "VERIFIED" || len(detail["analyses"].([]any)) != 0 || detail["calibrationId"] != nil {
		t.Fatalf("verified without calibration, nothing analysed: %v", detail)
	}
	if link := a.expect("video url", "GET", path+"/video/read-url", "VIEWER", nil, 200, ""); !strings.Contains(link["url"].(string), object) {
		t.Fatalf("video url: %v", link)
	}
	if status := a.expect("status", "GET", path+"/calibration-status", "VIEWER", nil, 200, ""); status["status"] != "WAITING_FOR_CALIBRATION" {
		t.Fatalf("status before calibration: %v", status)
	}
	if err := a.api.AnalysisTick(ctx); err != nil {
		t.Fatal(err)
	}
	if code, _ := a.send("POST", "/api/worker/claim", worker, map[string]any{}); code != 204 {
		t.Fatalf("a trial waiting for its calibration was analysed: %d", code)
	}

	// Calibrate on the trial's own video; the environment stays uncalibrated.
	calibrations := path + "/calibrations"
	a.expect("viewer calibrates", "POST", calibrations, "VIEWER", novelObjectCalibration(true), 403, "auth.forbidden")
	a.expect("calibrate without zones", "POST", calibrations, "TECHNICIAN", novelObjectCalibration(false), 400, "calibration.missingZone")
	calibration := a.expect("calibrate", "POST", calibrations, "TECHNICIAN", novelObjectCalibration(true), 201, "")
	if calibration["scope"] != "TRIAL" || calibration["trialId"] != id || calibration["videoAssetId"] == nil || calibration["testSubjectId"] != nil {
		t.Fatalf("trial calibration: %v", calibration)
	}
	if list := a.expect("environment calibrations", "GET", "/api/environments/"+env+"/revisions/1/calibrations", "VIEWER", nil, 200, ""); len(list["data"].([]any)) != 0 {
		t.Fatalf("the trial changed its environment: %v", list)
	}
	if status := a.expect("status", "GET", path+"/calibration-status", "VIEWER", nil, 200, ""); status["status"] != "CALIBRATED" || status["source"] != "TRIAL" {
		t.Fatalf("calibrated trial: %v", status)
	}

	// The automatic job: submitted once, claimed with the snapshot, published.
	for range 2 {
		if err := a.api.AnalysisTick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	detail = a.expect("trial", "GET", path, "VIEWER", nil, 200, "")
	analyses := detail["analyses"].([]any)
	if len(analyses) != 1 || analyses[0].(map[string]any)["trigger"] != "AUTOMATIC" || detail["calibrationId"] != calibration["id"] {
		t.Fatalf("automatic analysis: %v", detail)
	}
	claim := claimOne(t, a, worker)
	if claim["testSubject"] != nil || claim["requester"].(map[string]any)["kind"] != "TRIAL" || claim["requester"].(map[string]any)["id"] != id ||
		claim["calibration"].(map[string]any)["id"] != calibration["id"] || len(claim["calibration"].(map[string]any)["zones"].([]any)) != 2 {
		t.Fatalf("trial claim: %v", claim)
	}
	completeClaim(t, a, objects, worker, claim, 3_000_000, novelObjectSamples())
	first := a.expect("trial", "GET", path, "VIEWER", nil, 200, "")["analyses"].([]any)[0].(map[string]any)
	if first["job"].(map[string]any)["status"] != "SUCCEEDED" || metricOf(t, first, "novel_object_time_s") != 1.1 || first["qc"] == nil ||
		first["qc"].(map[string]any)["passed"] != true {
		t.Fatalf("first result: %v", first)
	}
	jobID := first["jobId"].(string)
	job := a.expect("job", "GET", "/api/analysis/jobs/"+jobID, "VIEWER", nil, 200, "")
	if len(job["result"].(map[string]any)["events"].([]any)) != 2 || job["requester"].(map[string]any)["kind"] != "TRIAL" {
		t.Fatalf("job detail: %v", job)
	}
	if track := a.expect("track", "GET", "/api/analysis/jobs/"+jobID+"/outputs/TRAJECTORY/read-url", "VIEWER", nil, 200, ""); !strings.Contains(track["url"].(string), "trajectory.json") {
		t.Fatalf("track url: %v", track)
	}
	a.expect("video pair", "GET", "/api/analysis/jobs/"+jobID+"/video-pair", "VIEWER", nil, 200, "")

	// Analyse again with a longer minimum bout: a new job, the first kept.
	a.expect("apparatus override", "POST", path+"/analyses", "TECHNICIAN", map[string]any{"overrides": map[string]any{"arena_width_cm": 60}}, 400, "trial.invalidOverrides")
	a.expect("viewer analyses", "POST", path+"/analyses", "VIEWER", map[string]any{}, 403, "auth.forbidden")
	again := a.expect("analyse again", "POST", path+"/analyses", "TECHNICIAN", map[string]any{"overrides": map[string]any{"min_bout_s": 1.5}}, 201, "")
	if again["trigger"] != "MANUAL" || again["job"].(map[string]any)["parameters"].(map[string]any)["min_bout_s"] != 1.5 ||
		again["overrides"].(map[string]any)["min_bout_s"] != 1.5 {
		t.Fatalf("second analysis: %v", again)
	}
	completeClaim(t, a, objects, worker, claimOne(t, a, worker), 3_000_000, novelObjectSamples())
	analyses = a.expect("trial", "GET", path, "VIEWER", nil, 200, "")["analyses"].([]any)
	if len(analyses) != 2 {
		t.Fatalf("both analyses kept: %v", analyses)
	}
	newest, oldest := analyses[0].(map[string]any), analyses[1].(map[string]any)
	if newest["jobId"] != again["jobId"] || oldest["jobId"] != jobID || metricOf(t, newest, "novel_object_time_s") != 0.0 ||
		metricOf(t, oldest, "novel_object_time_s") != 1.1 {
		t.Fatalf("comparison: %v", analyses)
	}
	if list := a.expect("list", "GET", "/api/trials", "VIEWER", nil, 200, ""); list["data"].([]any)[0].(map[string]any)["latestJob"].(map[string]any)["id"] != again["jobId"] {
		t.Fatalf("latest job in the list: %v", list)
	}

	// No trial job ever reaches the experiment side.
	if results := a.expect("review", "GET", "/api/analysis/results?status=ALL", "VIEWER", nil, 200, ""); results["total"] != float64(0) {
		t.Fatalf("a trial job entered review: %v", results)
	}

	// A soft delete hides the trial and keeps its jobs.
	a.expect("viewer deletes", "DELETE", path, "VIEWER", nil, 403, "auth.forbidden")
	if code, _ := a.send("DELETE", path, a.tokens["TECHNICIAN"], nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	a.expect("deleted", "GET", path, "VIEWER", nil, 404, "trial.notFound")
	a.expect("analyse a deleted trial", "POST", path+"/analyses", "TECHNICIAN", map[string]any{}, 404, "trial.notFound")
	if list := a.expect("list", "GET", "/api/trials", "VIEWER", nil, 200, ""); list["total"] != float64(0) {
		t.Fatalf("deleted trial listed: %v", list)
	}
	a.expect("job of a deleted trial", "GET", "/api/analysis/jobs/"+jobID, "VIEWER", nil, 200, "")
}
