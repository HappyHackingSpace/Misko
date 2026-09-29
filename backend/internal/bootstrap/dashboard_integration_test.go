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

// The dashboard reads across the tests, media, calibration and analysis
// domains without owning any of their tables, so what it needs proving is
// that its counts and lists actually track what those domains hold.
func TestDashboardSummaryOverHTTP(t *testing.T) {
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

	// A fresh database: the dashboard starts at zero.
	empty := c.expect("empty dashboard", c.call("GET", "/api/dashboard/summary", admin, nil), 200, "").body
	tests := empty["tests"].(map[string]any)
	if tests["planned"] != float64(0) || empty["calibrationWaiting"] != float64(0) ||
		len(empty["recentActivity"].([]any)) != 0 || len(empty["upcomingTests"].([]any)) != 0 {
		t.Fatalf("empty dashboard: %v", empty)
	}

	subject := c.create("/api/subjects", admin, map[string]any{"code": "M-1", "species": "MOUSE", "sex": "MALE"})
	exp := c.create("/api/experiments", admin, map[string]any{"code": "EXP-D", "title": "Dashboard"})
	base := "/api/experiments/" + id(exp)
	enrollment := c.create(base+"/enrollments", admin, map[string]any{"subjectId": id(subject), "enrolledAt": time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)})
	arena := c.create("/api/environments", admin, map[string]any{"name": "Arena", "paradigmKey": "OPEN_FIELD",
		"revision": map[string]any{"paradigmVersion": 1, "apparatus": map[string]any{"arena_width_cm": 50, "arena_height_cm": 40, "center_fraction": 0.5}}})
	protocol := c.create(base+"/protocols", admin, map[string]any{"name": "Open field", "version": map[string]any{"steps": []any{map[string]any{
		"position": 1, "paradigmKey": "OPEN_FIELD", "paradigmVersion": 1, "environmentRevisionId": id(arena["revision"].(map[string]any)), "trialType": "STANDARD", "trials": 1,
	}}}})

	// One test kept planned, in the near future, so it is both a status count
	// and the dashboard's one upcoming test.
	soon := time.Now().UTC().Add(2 * time.Hour)
	planned := c.create(base+"/tests", admin, map[string]any{"enrollmentId": id(enrollment), "protocolVersionId": id(protocol["version"].(map[string]any)),
		"stepPosition": 1, "scheduledAt": soon.Format(time.RFC3339)})

	// A second test is started, uploaded and verified, but never calibrated:
	// it should be the dashboard's one recording waiting for calibration.
	running := c.create(base+"/tests", admin, map[string]any{"enrollmentId": id(enrollment), "protocolVersionId": id(protocol["version"].(map[string]any)),
		"stepPosition": 1, "scheduledAt": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)})
	c.expect("start", c.call("POST", "/api/tests/"+id(running)+"/start", admin, map[string]any{}), 200, "")
	recordings := "/api/tests/" + id(running) + "/recordings"
	rec := c.create(recordings, admin, map[string]any{"fileName": "trial.mp4", "contentType": "video/mp4", "sizeBytes": 4096, "crc32c": domain.FormatCRC32C(1), "clipStartUs": 0, "clipEndUs": 6000000})["recording"].(map[string]any)
	recPath := recordings + "/" + id(rec)
	objects.finishUpload(4096, 1)
	c.expect("finalize", c.call("POST", recPath+"/finalize", admin, map[string]any{}), 200, "")

	after := c.expect("dashboard after planning", c.call("GET", "/api/dashboard/summary", admin, nil), 200, "").body
	tests = after["tests"].(map[string]any)
	if tests["planned"] != float64(1) || tests["inProgress"] != float64(1) || after["calibrationWaiting"] != float64(1) {
		t.Fatalf("counts: %v", after)
	}
	upcoming := after["upcomingTests"].([]any)[0].(map[string]any)
	if upcoming["id"] != id(planned) || upcoming["subjectCode"] != "M-1" || upcoming["experimentCode"] != "EXP-D" || upcoming["paradigmKey"] != "OPEN_FIELD" {
		t.Fatalf("upcoming test: %v", upcoming)
	}

	// Calibrating the recording (OPEN_FIELD requires one) drops it out of the
	// waiting count and, once a worker exists, queues its analysis.
	truth := calibration.Homography{0.03, 0.002, -5, -0.001, 0.045, -3, 0.00001, 0.00002, 1}
	pt := func(px, py float64) map[string]any {
		x, y, _ := truth.Apply(px, py)
		return map[string]any{"pixelX": px, "pixelY": py, "worldX": x, "worldY": y}
	}
	c.create(recPath+"/calibrations", admin, map[string]any{"cameraId": "cam", "frameWidth": 1920, "frameHeight": 1080,
		"crop": map[string]any{"x": 0, "y": 0, "width": 1920, "height": 1080}, "referenceFrameUs": 0, "measurementPlane": "ARENA_FLOOR",
		"fitPoints": []any{pt(200, 150), pt(1700, 120), pt(1750, 1000), pt(250, 980)}, "checkPoints": []any{pt(900, 500), pt(400, 700), pt(1400, 300)}})
	c.create("/api/analysis/workers", admin, map[string]any{"name": "tracker", "modelVersion": "tracker 1.0",
		"capabilities": []any{map[string]any{"paradigmKey": "OPEN_FIELD", "paradigmVersion": 1}}})
	if err := api.AnalysisTick(ctx); err != nil {
		t.Fatal(err)
	}

	calibrated := c.expect("dashboard after calibrating", c.call("GET", "/api/dashboard/summary", admin, nil), 200, "").body
	if calibrated["calibrationWaiting"] != float64(0) || calibrated["analysisQueued"] != float64(1) {
		t.Fatalf("after calibration: %v", calibrated)
	}

	// Reading the dashboard needs only *:read: a viewer sees it too.
	c.create("/api/users", admin, map[string]any{"email": "viewer@lab.io", "name": "Viewer", "role": "VIEWER", "password": "Password-123"})
	viewer := c.login("viewer@lab.io", "Password-123")
	c.expect("viewer reads the dashboard", c.call("GET", "/api/dashboard/summary", viewer, nil), 200, "")
	c.expect("anonymous cannot", c.call("GET", "/api/dashboard/summary", "", nil), 401, "auth.unauthenticated")
}
