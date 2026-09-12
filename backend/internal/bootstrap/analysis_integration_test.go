//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	calibration "github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnalysisOverHTTP(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	setup, err := Setup(ctx, pool, config.Setup{AdminEmail: "admin@lab.io", LabName: "Lab", LabTimezone: "UTC", BcryptCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	objects := &fakeObjects{stored: map[string]domain.ObjectAttrs{}}
	api, err := NewAPI(pool, config.Config{ProbeTimeout: time.Second},
		config.Auth{JWTSecret: []byte(strings.Repeat("s", 32)), JWTIssuer: "misko", JWTAudience: "misko-api", TokenTTL: time.Hour, BcryptCost: bcrypt.MinCost},
		slog.New(slog.NewJSONHandler(io.Discard, nil)), WithStorage(objects, mediaapp.Settings{MaxBytes: 1 << 30, UploadTTL: time.Hour, ReadTTL: time.Minute}))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler)
	defer server.Close()
	c := client{t: t, url: server.URL, http: server.Client()}
	admin := c.login("admin@lab.io", setup.AdminPassword)
	tokens := map[string]string{"SUPERADMIN": admin}
	for _, role := range []string{"TECHNICIAN", "VIEWER"} {
		email := strings.ToLower(role) + "@lab.io"
		c.create("/api/users", admin, map[string]any{"email": email, "name": role, "role": role, "password": "Password-123"})
		tokens[role] = c.login(email, "Password-123")
	}
	workerCall := func(method, path, token string, body any) response {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		if token != "" {
			req.Header.Set("Authorization", "Worker "+token)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var decoded map[string]any
		_ = json.NewDecoder(res.Body).Decode(&decoded)
		return response{res.StatusCode, decoded}
	}

	// A calibrated, verified OPEN_FIELD recording of a test.
	subject := c.create("/api/subjects", admin, map[string]any{"code": "M-1", "species": "MOUSE", "sex": "MALE"})
	exp := c.create("/api/experiments", admin, map[string]any{"code": "EXP-A", "title": "Analysis"})
	base := "/api/experiments/" + id(exp)
	enrollment := c.create(base+"/enrollments", admin, map[string]any{"subjectId": id(subject), "enrolledAt": time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)})
	arena := c.create("/api/environments", admin, map[string]any{"name": "Arena", "paradigmKey": "OPEN_FIELD",
		"revision": map[string]any{"paradigmVersion": 1, "apparatus": map[string]any{"arena_width_cm": 50, "arena_height_cm": 40, "center_fraction": 0.5}}})
	protocol := c.create(base+"/protocols", admin, map[string]any{"name": "Open field", "version": map[string]any{"steps": []any{map[string]any{
		"position": 1, "paradigmKey": "OPEN_FIELD", "paradigmVersion": 1, "environmentRevisionId": id(arena["revision"].(map[string]any)), "trialType": "STANDARD", "trials": 1,
	}}}})
	test := c.create(base+"/tests", admin, map[string]any{"enrollmentId": id(enrollment), "protocolVersionId": id(protocol["version"].(map[string]any)),
		"stepPosition": 1, "scheduledAt": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)})
	recordings := "/api/tests/" + id(test) + "/recordings"
	rec := c.create(recordings, tokens["TECHNICIAN"], map[string]any{"fileName": "trial.mp4", "contentType": "video/mp4", "sizeBytes": 4096, "crc32c": domain.FormatCRC32C(1), "clipStartUs": 0, "clipEndUs": 6000000})["recording"].(map[string]any)
	recPath := recordings + "/" + id(rec)
	objects.finishUpload(4096, 1)
	source := c.expect("finalize", c.call("POST", recPath+"/finalize", tokens["TECHNICIAN"], map[string]any{}), 200, "").body["video"].(map[string]any)
	truth := calibration.Homography{0.03, 0.002, -5, -0.001, 0.045, -3, 0.00001, 0.00002, 1}
	pt := func(px, py float64) map[string]any {
		x, y, _ := truth.Apply(px, py)
		return map[string]any{"pixelX": px, "pixelY": py, "worldX": x, "worldY": y}
	}
	c.create(recPath+"/calibrations", tokens["TECHNICIAN"], map[string]any{"cameraId": "cam", "frameWidth": 1920, "frameHeight": 1080,
		"crop": map[string]any{"x": 0, "y": 0, "width": 1920, "height": 1080}, "referenceFrameUs": 0, "measurementPlane": "ARENA_FLOOR",
		"fitPoints": []any{pt(200, 150), pt(1700, 120), pt(1750, 1000), pt(250, 980)}, "checkPoints": []any{pt(900, 500), pt(400, 700), pt(1400, 300)}})

	// Workers are registered by lab managers and get their token once.
	register := map[string]any{"name": "tracker", "modelVersion": "tracker 1.0", "capabilities": []any{map[string]any{"paradigmKey": "OPEN_FIELD", "paradigmVersion": 1}}}
	c.expect("viewer registers a worker", c.call("POST", "/api/analysis/workers", tokens["VIEWER"], register), 403, "auth.forbidden")
	registered := c.create("/api/analysis/workers", admin, register)
	token := registered["token"].(string)
	if caps := c.call("GET", "/api/analysis/capabilities", tokens["VIEWER"], nil).body["data"].([]any); len(caps) != 1 {
		t.Fatalf("capabilities: %v", caps)
	}

	if err := api.AnalysisTick(ctx); err != nil {
		t.Fatal(err)
	}
	runs := c.call("GET", "/api/tests/"+id(test)+"/analysis-runs", tokens["VIEWER"], nil).body["data"].([]any)
	if len(runs) != 1 || runs[0].(map[string]any)["status"] != "QUEUED" || runs[0].(map[string]any)["calibrationId"] == nil {
		t.Fatalf("scheduled runs: %v", runs)
	}

	if r := workerCall("POST", "/api/worker/claim", "", map[string]any{}); r.status != 401 || r.body["code"] != "worker.unauthenticated" {
		t.Fatalf("claim without a worker token: %v", r)
	}
	c.expect("user token on a worker route", c.call("POST", "/api/worker/claim", admin, map[string]any{}), 401, "worker.unauthenticated")
	c.expect("worker token on a user route", c.call("GET", "/api/tests/"+id(test)+"/analysis-runs", token, nil), 401, "auth.unauthenticated")
	claim := workerCall("POST", "/api/worker/claim", token, map[string]any{})
	if claim.status != 200 {
		t.Fatalf("claim: %v", claim)
	}
	run := claim.body["run"].(map[string]any)
	prefix := claim.body["outputPrefix"].(string)
	contract := claim.body["contract"].(map[string]any)
	calibration, _ := claim.body["calibration"].(map[string]any)
	if run["attempt"] != float64(1) || len(contract["metrics"].([]any)) != 9 || len(contract["qc"].([]any)) != 3 ||
		claim.body["trajectorySchema"] != "misko.trajectory.v1" || calibration == nil || len(calibration["transform"].([]any)) != 9 || calibration["id"] != run["calibrationId"] {
		t.Fatalf("job: %v", claim.body)
	}
	if r := workerCall("POST", "/api/worker/claim", token, map[string]any{}); r.status != 204 {
		t.Fatalf("second claim: %d", r.status)
	}
	runPath := "/api/worker/runs/" + id(run)
	if r := workerCall("POST", runPath+"/source-url", token, map[string]any{"attempt": 1}); r.status != 200 || !strings.Contains(r.body["url"].(string), "generation="+source["generation"].(string)) {
		t.Fatalf("source URL: %v", r)
	}
	output := workerCall("POST", runPath+"/outputs", token, map[string]any{"attempt": 1, "kind": "ANALYZED_VIDEO", "fileName": "overlay.mp4", "contentType": "video/mp4", "sizeBytes": 2048, "crc32c": domain.FormatCRC32C(5)})
	if output.status != 201 || output.body["objectName"] != prefix+"overlay.mp4" {
		t.Fatalf("output upload: %v", output)
	}
	objects.finishUpload(2048, 5)

	// The worker walks 10 cm into the 50 x 40 cm arena's center, then stays still.
	var ts []int64
	var xs, ys, confidence []float64
	var tracked []bool
	for i := 0; i <= 25; i++ {
		ts, xs, ys = append(ts, int64(i)*100_000), append(xs, 5+float64(min(i, 10))), append(ys, 20)
		tracked, confidence = append(tracked, true), append(confidence, 0.95)
	}
	trajectory, err := json.Marshal(map[string]any{"schema": "misko.trajectory.v1", "runId": id(run), "attempt": 1, "durationUs": 6_000_000,
		"samples": map[string]any{"tUs": ts, "xCm": xs, "yCm": ys, "tracked": tracked, "confidence": confidence},
		"worker":  map[string]any{"algorithm": "test fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	declare := func(contentType string) response {
		return workerCall("POST", runPath+"/outputs", token, map[string]any{"attempt": 1, "kind": "TRAJECTORY", "fileName": "trajectory.json",
			"contentType": contentType, "sizeBytes": len(trajectory), "crc32c": domain.FormatCRC32C(6)})
	}
	if r := declare("application/vnd.apache.parquet"); r.status != 400 || r.body["code"] != "analysis.invalidOutput" {
		t.Fatalf("a trajectory the API cannot read: %v", r)
	}
	if r := declare("application/json"); r.status != 201 || r.body["objectName"] != prefix+"trajectory.json" {
		t.Fatalf("trajectory upload: %v", r)
	}
	result := map[string]any{"attempt": 1, "modelVersion": "tracker 1.0",
		"artifacts": []any{map[string]any{"kind": "ANALYZED_VIDEO", "objectName": prefix + "overlay.mp4"}, map[string]any{"kind": "TRAJECTORY", "objectName": prefix + "trajectory.json"}},
		"pair":      map[string]any{"analyzedObjectName": prefix + "overlay.mp4", "sourceOffsetUs": 0, "outputOffsetUs": 0, "timeMappingVersion": "identity-v1"}}
	if r := workerCall("POST", runPath+"/result", token, map[string]any{"attempt": 1, "modelVersion": "tracker 1.0", "metrics": []any{},
		"artifacts": result["artifacts"], "pair": result["pair"]}); r.status != 400 || r.body["code"] != "common.invalidJSON" {
		t.Fatalf("a worker cannot send metrics: %v", r)
	}
	if r := workerCall("POST", runPath+"/result", token, result); r.status != 409 || r.body["code"] != "analysis.outputNotVerified" {
		t.Fatalf("result before the trajectory upload: %v", r)
	}
	objects.finishUploadWith(trajectory, 6, "application/json")
	if r := workerCall("POST", runPath+"/result", token, result); r.status != 200 || r.body["status"] != "SUCCEEDED" {
		t.Fatalf("result: %v", r)
	}
	if r := workerCall("POST", runPath+"/result", token, result); r.status != 200 {
		t.Fatalf("duplicate delivery: %v", r)
	}

	detail := c.expect("run detail", c.call("GET", "/api/analysis-runs/"+id(run), tokens["VIEWER"], nil), 200, "").body
	published := detail["result"].(map[string]any)
	if detail["status"] != "SUCCEEDED" || len(published["metrics"].([]any)) != 9 || len(published["events"].([]any)) != 3 || published["pair"] == nil {
		t.Fatalf("detail: %v", detail)
	}
	for _, m := range published["metrics"].([]any) {
		if metric := m.(map[string]any); metric["key"] == "distance_cm" && metric["value"] != float64(10) {
			t.Fatalf("the engine's distance: %v", metric)
		}
	}
	pair := c.expect("video pair", c.call("GET", "/api/analysis-runs/"+id(run)+"/video-pair", tokens["VIEWER"], nil), 200, "").body
	if !strings.Contains(pair["originalUrl"].(string), "generation="+source["generation"].(string)) || !strings.Contains(pair["analyzedUrl"].(string), "overlay.mp4") {
		t.Fatalf("pair: %v", pair)
	}
	c.expect("viewer reanalyzes", c.call("POST", recPath+"/analysis-runs", tokens["VIEWER"], map[string]any{}), 403, "auth.forbidden")
	manual := c.create(recPath+"/analysis-runs", tokens["TECHNICIAN"], map[string]any{})
	if manual["trigger"] != "MANUAL" || manual["status"] != "QUEUED" {
		t.Fatalf("reanalysis: %v", manual)
	}
	if runs := c.call("GET", "/api/tests/"+id(test)+"/analysis-runs", tokens["VIEWER"], nil).body["data"].([]any); len(runs) != 2 {
		t.Fatalf("runs after reanalysis: %v", runs)
	}
}
