//go:build integration

package bootstrap

import (
	"context"
	calibration "github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	study := seedMediaStudy(t, pool, "EXP-C", 2)
	first, secondAnimal := study.subjects[0], study.subjects[1]
	testPath := "/api/tests/" + study.test
	subjectPath := func(subject string) string { return testPath + "/subjects/" + subject }
	video := map[string]any{"fileName": "trial.mp4", "contentType": "video/mp4", "sizeBytes": 4096, "crc32c": domain.FormatCRC32C(1)}
	upload := func(test string, finish bool) string {
		path := "/api/tests/" + test + "/recordings"
		rec := c.create(path, tokens["TECHNICIAN"], video)["recording"].(map[string]any)
		if finish {
			objects.finishUpload(4096, 1)
			c.expect("finalize", c.call("POST", path+"/"+id(rec)+"/finalize", tokens["TECHNICIAN"], map[string]any{}), 200, "")
		}
		return id(rec)
	}

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
	status := func(subject string) map[string]any {
		return c.expect("status "+subject, c.call("GET", subjectPath(subject)+"/calibration-status", tokens["VIEWER"], nil), 200, "").body
	}

	if s := status(first); s["status"] != "WAITING_FOR_VIDEO" || s["recordingId"] != nil || s["testSubjectId"] != first {
		t.Fatalf("before any video: %v", s)
	}
	c.expect("override without a video", c.call("POST", subjectPath(first)+"/calibrations", tokens["TECHNICIAN"], body("", 0)), 409, "calibration.noVideo")
	pending := upload(study.test, false)
	// A video still uploading is not the test's current video yet.
	if s := status(first); s["status"] != "WAITING_FOR_VIDEO" || s["recordingId"] != nil || pending == "" {
		t.Fatalf("pending video: %v", s)
	}
	c.expect("unverified video", c.call("POST", subjectPath(first)+"/calibrations", tokens["TECHNICIAN"], body("", 0)), 409, "calibration.noVideo")
	rec := upload(study.test, true)
	if s := status(first); s["status"] != "WAITING_FOR_CALIBRATION" || s["calibration"] != nil || s["recordingId"] != rec || s["source"] != nil {
		t.Fatalf("before calibration: %v", s)
	}
	c.expect("viewer calibrates", c.call("POST", subjectPath(first)+"/calibrations", tokens["VIEWER"], body("", 0)), 403, "auth.forbidden")
	c.expect("unknown animal", c.call("POST", subjectPath("01a00000-0000-7000-8000-00000000dead")+"/calibrations", tokens["TECHNICIAN"], body("", 0)), 404, "test.subjectNotFound")
	c.expect("unknown test", c.call("GET", "/api/tests/01a00000-0000-7000-8000-00000000dead/calibration-status", tokens["VIEWER"], nil), 404, "test.notFound")
	degenerate := body("", 0)
	degenerate["fitPoints"] = []any{pt(200, 200, 0), pt(500, 400, 0), pt(800, 600, 0), pt(1100, 800, 0)}
	c.expect("collinear FIT points", c.call("POST", subjectPath(first)+"/calibrations", tokens["TECHNICIAN"], degenerate), 400, "calibration.degenerateFit")
	fitOnly := body("", 0)
	fitOnly["checkPoints"] = []any{}
	c.expect("FIT without CHECK", c.call("POST", subjectPath(first)+"/calibrations", tokens["TECHNICIAN"], fitOnly), 400, "calibration.insufficientCheckPoints")

	own := c.create(subjectPath(first)+"/calibrations", tokens["TECHNICIAN"], body("", 0))
	if own["status"] != "VALID" || own["scope"] != "OVERRIDE" || own["testSubjectId"] != first || own["recordingId"] != rec || own["referenceFrameUs"] != float64(1500000) ||
		own["algorithmVersion"] != calibration.AlgorithmVersion || own["toleranceCm"] != float64(2) || own["environmentRevisionId"] != study.arenas[0] {
		t.Fatalf("first override: %v", own)
	}
	if s := status(first); s["status"] != "CALIBRATED" || s["source"] != "OVERRIDE" || s["drift"] != "UNCHECKED" || s["calibration"].(map[string]any)["id"] != id(own) {
		t.Fatalf("calibrated: %v", s)
	}
	c.expect("second first override", c.call("POST", subjectPath(first)+"/calibrations", tokens["TECHNICIAN"], body("", 0)), 409, "calibration.staleCorrection")
	rejected := c.create(subjectPath(first)+"/calibrations", tokens["TECHNICIAN"], body(id(own), 3))
	if rejected["status"] != "REJECTED" || rejected["rejectionReason"] != "EXCESSIVE_CHECK_ERROR" {
		t.Fatalf("excessive CHECK error: %v", rejected)
	}
	if s := status(first); s["status"] != "WAITING_FOR_CALIBRATION" || s["source"] != "OVERRIDE" {
		t.Fatalf("after a rejected correction: %v", s)
	}
	if list := c.call("GET", subjectPath(first)+"/calibrations", tokens["VIEWER"], nil).body["data"].([]any); len(list) != 2 {
		t.Fatalf("overrides: %v", list)
	}

	// The default of each arena serves its animal of the shared video.
	arenaPath := "/api/environments/" + study.environments[1] + "/revisions/1"
	if s := status(secondAnimal); s["status"] != "WAITING_FOR_CALIBRATION" || s["source"] != nil || s["drift"] != nil {
		t.Fatalf("before the arena is calibrated: %v", s)
	}
	if s := c.expect("environment status", c.call("GET", arenaPath+"/calibration-status", tokens["VIEWER"], nil), 200, "").body; s["status"] != "WAITING_FOR_CALIBRATION" {
		t.Fatalf("uncalibrated environment: %v", s)
	}
	c.expect("technician calibrates the rig", c.call("POST", arenaPath+"/calibrations", tokens["TECHNICIAN"], body("", 0)), 403, "auth.forbidden")
	c.expect("unknown revision", c.call("POST", "/api/environments/"+study.environments[1]+"/revisions/9/calibrations", admin, body("", 0)), 404, "environment.revisionNotFound")
	defaults := c.create(arenaPath+"/calibrations", admin, body("", 0))
	if defaults["status"] != "VALID" || defaults["scope"] != "ENVIRONMENT" || defaults["recordingId"] != nil || defaults["testSubjectId"] != nil ||
		defaults["referenceFrameUs"] != nil || defaults["environmentRevisionId"] != study.arenas[1] {
		t.Fatalf("environment calibration: %v", defaults)
	}
	c.expect("second first environment calibration", c.call("POST", arenaPath+"/calibrations", admin, body("", 0)), 409, "calibration.staleCorrection")
	if list := c.call("GET", arenaPath+"/calibrations", tokens["VIEWER"], nil).body["data"].([]any); len(list) != 1 {
		t.Fatalf("environment calibrations: %v", list)
	}
	all := c.expect("test status", c.call("GET", testPath+"/calibration-status", tokens["VIEWER"], nil), 200, "").body["data"].([]any)
	byAnimal := map[string]map[string]any{}
	for _, row := range all {
		byAnimal[row.(map[string]any)["testSubjectId"].(string)] = row.(map[string]any)
	}
	if a, b := byAnimal[first], byAnimal[secondAnimal]; len(all) != 2 || a["status"] != "WAITING_FOR_CALIBRATION" || a["source"] != "OVERRIDE" ||
		b["status"] != "CALIBRATED" || b["source"] != "ENVIRONMENT" || b["calibration"].(map[string]any)["id"] != id(defaults) || b["environmentRevisionId"] != study.arenas[1] {
		t.Fatalf("test status: %v", all)
	}

	// A replacement video starts from the arena defaults again.
	replacement := upload(study.test, true)
	if s := status(first); s["status"] != "WAITING_FOR_CALIBRATION" || s["source"] != nil || s["recordingId"] != replacement {
		t.Fatalf("after a replacement video: %v", s)
	}
	moved := body(id(rejected), 0)
	moved["cameraId"] = "cam-top-2"
	again := c.create(subjectPath(first)+"/calibrations", tokens["TECHNICIAN"], moved)
	if again["recordingId"] != replacement || again["supersedesId"] != id(rejected) {
		t.Fatalf("override on the replacement: %v", again)
	}
	if s := status(first); s["status"] != "CALIBRATED" || s["calibration"].(map[string]any)["id"] != id(again) {
		t.Fatalf("calibrated on the replacement: %v", s)
	}

	// An observation-only paradigm needs no calibration.
	rotarod := seedRotarodSubject(t, pool, study)
	upload(rotarod.test, true)
	if s := c.expect("rotarod status", c.call("GET", "/api/tests/"+rotarod.test+"/subjects/"+rotarod.subject+"/calibration-status", tokens["VIEWER"], nil), 200, "").body; s["status"] != "NOT_REQUIRED" {
		t.Fatalf("observation-only paradigm: %v", s)
	}
	c.expect("calibrate a rotarod animal", c.call("POST", "/api/tests/"+rotarod.test+"/subjects/"+rotarod.subject+"/calibrations", tokens["TECHNICIAN"], body("", 0)), 409, "calibration.notRequired")
	c.expect("rotarod rig", c.call("POST", "/api/environments/"+rotarod.environment+"/revisions/1/calibrations", admin, body("", 0)), 409, "calibration.notRequired")
}

type rotarodSubject struct{ test, subject, environment string }

// seedRotarodSubject films one more animal of the study on the rotarod step.
func seedRotarodSubject(t *testing.T, pool *pgxpool.Pool, study mediaStudy) rotarodSubject {
	t.Helper()
	ctx := context.Background()
	var out rotarodSubject
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		steps := []struct {
			sql  string
			args []any
			dest *string
		}{
			{`INSERT INTO misko.environments (name, paradigm_key) VALUES ('Rod', 'ROTAROD') RETURNING id`, nil, &out.environment},
			{`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
				SELECT experiment_id, timepoint_id, protocol_version_id, 2, 'ROTAROD', 1, 1, now(), created_by FROM misko.tests WHERE id = $1 RETURNING id`, []any{study.test}, &out.test},
		}
		for _, step := range steps {
			if err := tx.QueryRow(ctx, step.sql, step.args...).Scan(step.dest); err != nil {
				return err
			}
		}
		var revision string
		if err := tx.QueryRow(ctx, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
			VALUES ($1, 'ROTAROD', 1, 1, '{}', $2) RETURNING id`, out.environment, seedUser).Scan(&revision); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
			SELECT experiment_id, $1, allocation_id, $2, 'ROTAROD', 1, created_by FROM misko.test_subjects WHERE id = $3 RETURNING id`,
			out.test, revision, study.subjects[0]).Scan(&out.subject); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() WHERE id = $1", out.test)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
