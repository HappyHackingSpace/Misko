//go:build integration

package bootstrap

import (
	"context"
	calibration "github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCalibrationOverHTTP(t *testing.T) {
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

	subject := c.create("/api/subjects", admin, map[string]any{"code": "M-1", "species": "MOUSE", "sex": "MALE"})
	exp := c.create("/api/experiments", admin, map[string]any{"code": "EXP-C", "title": "Calibration"})
	base := "/api/experiments/" + id(exp)
	enrollment := c.create(base+"/enrollments", admin, map[string]any{"subjectId": id(subject), "enrolledAt": time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)})
	arena := c.create("/api/environments", admin, map[string]any{"name": "Arena", "paradigmKey": "OPEN_FIELD",
		"revision": map[string]any{"paradigmVersion": 1, "apparatus": map[string]any{"arena_width_cm": 50, "arena_height_cm": 40, "center_fraction": 0.5}}})
	rotarod := c.create("/api/environments", admin, map[string]any{"name": "Rod", "paradigmKey": "ROTAROD",
		"revision": map[string]any{"paradigmVersion": 1, "apparatus": map[string]any{"start_rpm": 4, "end_rpm": 40}}})
	protocol := c.create(base+"/protocols", admin, map[string]any{"name": "Battery", "version": map[string]any{"steps": []any{
		map[string]any{"position": 1, "paradigmKey": "OPEN_FIELD", "paradigmVersion": 1, "environmentRevisionId": id(arena["revision"].(map[string]any)), "trialType": "STANDARD", "trials": 1},
		map[string]any{"position": 2, "paradigmKey": "ROTAROD", "paradigmVersion": 1, "environmentRevisionId": id(rotarod["revision"].(map[string]any)), "trialType": "ACCELERATING", "trials": 1},
	}}})
	plan := func(step int) map[string]any {
		return c.create(base+"/tests", admin, map[string]any{"enrollmentId": id(enrollment), "protocolVersionId": id(protocol["version"].(map[string]any)),
			"stepPosition": step, "scheduledAt": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)})
	}
	video := map[string]any{"fileName": "trial.mp4", "contentType": "video/mp4", "sizeBytes": 4096, "crc32c": domain.FormatCRC32C(1)}
	upload := func(test map[string]any, finish bool) string {
		path := "/api/tests/" + id(test) + "/recordings"
		rec := c.create(path, tokens["TECHNICIAN"], video)["recording"].(map[string]any)
		if finish {
			objects.finishUpload(4096, 1)
			c.expect("finalize", c.call("POST", path+"/"+id(rec)+"/finalize", tokens["TECHNICIAN"], map[string]any{}), 200, "")
		}
		return path + "/" + id(rec)
	}
	openField := plan(1)
	recPath, pendingPath := upload(openField, true), upload(openField, false)

	truth := calibration.Homography{0.03, 0.002, -5, -0.001, 0.045, -3, 0.00001, 0.00002, 1}
	pt := func(px, py, dx float64) map[string]any {
		x, y, _ := truth.Apply(px, py)
		return map[string]any{"pixelX": px, "pixelY": py, "worldX": x + dx, "worldY": y}
	}
	body := func(supersedes string, checkOffset float64) map[string]any {
		b := map[string]any{"cameraId": "cam-top-1", "frameWidth": 1920, "frameHeight": 1080, "crop": map[string]any{"x": 0, "y": 0, "width": 1920, "height": 1080},
			"referenceFrameUs": 1500000, "measurementPlane": "ARENA_FLOOR",
			"fitPoints":   []any{pt(200, 150, 0), pt(1700, 120, 0), pt(1750, 1000, 0), pt(250, 980, 0)},
			"checkPoints": []any{pt(900, 500, checkOffset), pt(400, 700, 0), pt(1400, 300, 0)}}
		if supersedes != "" {
			b["supersedesId"] = supersedes
		}
		return b
	}

	status := func(path string) map[string]any {
		return c.expect("status "+path, c.call("GET", path+"/calibration-status", tokens["VIEWER"], nil), 200, "").body
	}
	if s := status(recPath); s["status"] != "WAITING_FOR_CALIBRATION" || s["calibration"] != nil {
		t.Fatalf("before calibration: %v", s)
	}
	c.expect("viewer calibrates", c.call("POST", recPath+"/calibrations", tokens["VIEWER"], body("", 0)), 403, "auth.forbidden")
	c.expect("unverified video", c.call("POST", pendingPath+"/calibrations", tokens["TECHNICIAN"], body("", 0)), 409, "calibration.videoNotVerified")
	degenerate := body("", 0)
	degenerate["fitPoints"] = []any{pt(200, 200, 0), pt(500, 400, 0), pt(800, 600, 0), pt(1100, 800, 0)}
	c.expect("collinear FIT points", c.call("POST", recPath+"/calibrations", tokens["TECHNICIAN"], degenerate), 400, "calibration.degenerateFit")
	fitOnly := body("", 0)
	fitOnly["checkPoints"] = []any{}
	c.expect("FIT without CHECK", c.call("POST", recPath+"/calibrations", tokens["TECHNICIAN"], fitOnly), 400, "calibration.insufficientCheckPoints")

	first := c.create(recPath+"/calibrations", tokens["TECHNICIAN"], body("", 0))
	if first["status"] != "VALID" || first["algorithmVersion"] != calibration.AlgorithmVersion || first["toleranceCm"] != float64(2) {
		t.Fatalf("first calibration: %v", first)
	}
	if s := status(recPath); s["status"] != "CALIBRATED" || s["calibration"].(map[string]any)["id"] != id(first) {
		t.Fatalf("calibrated: %v", s)
	}
	c.expect("second first calibration", c.call("POST", recPath+"/calibrations", tokens["TECHNICIAN"], body("", 0)), 409, "calibration.staleCorrection")
	rejected := c.create(recPath+"/calibrations", tokens["TECHNICIAN"], body(id(first), 3))
	if rejected["status"] != "REJECTED" || rejected["rejectionReason"] != "EXCESSIVE_CHECK_ERROR" {
		t.Fatalf("excessive CHECK error: %v", rejected)
	}
	if s := status(recPath); s["status"] != "WAITING_FOR_CALIBRATION" {
		t.Fatalf("after a rejected correction: %v", s)
	}
	if list := c.call("GET", recPath+"/calibrations", tokens["VIEWER"], nil).body["data"].([]any); len(list) != 2 {
		t.Fatalf("calibrations: %v", list)
	}
	rotarodPath := upload(plan(2), true)
	if s := status(rotarodPath); s["status"] != "NOT_REQUIRED" {
		t.Fatalf("observation-only paradigm: %v", s)
	}
	c.expect("calibrate a rotarod video", c.call("POST", rotarodPath+"/calibrations", tokens["TECHNICIAN"], body("", 0)), 409, "calibration.notRequired")
}
