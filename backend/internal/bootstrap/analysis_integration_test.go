//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	mediadomain "github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// The helpers in this file build the study chain (experiment, groups,
// allocations, tests, animals, arenas, calibrations, videos) with plain SQL,
// because the contexts that own those tables are ported separately. Their
// names start with "study" so they never collide with other test files.

const studyAuthor = "01a00000-0000-7000-8000-00000000a0a0"

// studyAPI is the HTTP API with an administrator, a researcher, a technician
// and a viewer signed in.
type studyAPI struct {
	t      *testing.T
	pool   *pgxpool.Pool
	api    API
	server *httptest.Server
	tokens map[string]string
}

func newStudyAPI(t *testing.T, objects mediaapp.ObjectStore) *studyAPI {
	t.Helper()
	ctx := context.Background()
	pool := pgtest.Database(t)
	setup, err := Setup(ctx, pool, config.Setup{AdminEmail: "admin@lab.io", LabName: "Lab", LabTimezone: "UTC", BcryptCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	var opts []Option
	if objects != nil {
		opts = append(opts, WithStorage(objects, mediaapp.Settings{MaxBytes: 1 << 30, UploadTTL: time.Hour, ReadTTL: time.Minute}))
	}
	api, err := NewAPI(pool, config.Config{ProbeTimeout: time.Second},
		config.Auth{JWTSecret: []byte(strings.Repeat("s", 32)), JWTIssuer: "misko", JWTAudience: "misko-api", TokenTTL: time.Hour, BcryptCost: bcrypt.MinCost},
		slog.New(slog.NewJSONHandler(io.Discard, nil)), opts...)
	if err != nil {
		t.Fatal(err)
	}
	a := &studyAPI{t: t, pool: pool, api: api, server: httptest.NewServer(api.Handler), tokens: map[string]string{}}
	t.Cleanup(a.server.Close)
	a.tokens["SUPERADMIN"] = a.login("admin@lab.io", setup.AdminPassword)
	for _, role := range []string{"RESEARCHER", "TECHNICIAN", "VIEWER"} {
		email := strings.ToLower(role) + "@lab.io"
		a.expect("create "+role, "POST", "/api/users", "SUPERADMIN", map[string]any{"email": email, "name": role, "role": role, "password": "Password-123"}, 201, "")
		a.tokens[role] = a.login(email, "Password-123")
	}
	return a
}

func (a *studyAPI) login(email, password string) string {
	a.t.Helper()
	status, body := a.send("POST", "/api/auth/login", "", map[string]string{"email": email, "password": password})
	if status != 200 {
		a.t.Fatalf("login %s: %d %v", email, status, body)
	}
	return body["token"].(string)
}

// send calls the API with an Authorization header value ("" for none).
func (a *studyAPI) send(method, path, authorization string, body any) (int, map[string]any) {
	a.t.Helper()
	res := a.raw(method, path, authorization, body)
	defer res.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return res.StatusCode, decoded
}

func (a *studyAPI) raw(method, path, authorization string, body any) *http.Response {
	a.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, a.server.URL+path, reader)
	if authorization != "" {
		if !strings.Contains(authorization, " ") {
			authorization = "Bearer " + authorization
		}
		req.Header.Set("Authorization", authorization)
	}
	res, err := a.server.Client().Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	return res
}

// expect calls the API as a role ("" for anonymous) and checks the status and code.
func (a *studyAPI) expect(label, method, path, role string, body any, status int, code string) map[string]any {
	a.t.Helper()
	got, decoded := a.send(method, path, a.tokens[role], body)
	if got != status || (code != "" && decoded["code"] != code) {
		a.t.Fatalf("%s: status=%d body=%v want %d %s", label, got, decoded, status, code)
	}
	return decoded
}

// studyObjects is an in-memory object store; it proves the API contract only.
type studyObjects struct {
	mu       sync.Mutex
	stored   map[string]mediadomain.ObjectAttrs
	contents map[string][]byte
}

func newStudyObjects() *studyObjects {
	return &studyObjects{stored: map[string]mediadomain.ObjectAttrs{}, contents: map[string][]byte{}}
}

func (s *studyObjects) Bucket() string { return "misko-test-videos" }

func (s *studyObjects) SizedUploadURL(ctx context.Context, object, contentType string, _ int64, expires time.Time) (mediaapp.SignedRequest, error) {
	return s.UploadURL(ctx, object, contentType, expires)
}

func (s *studyObjects) UploadURL(_ context.Context, object, contentType string, expires time.Time) (mediaapp.SignedRequest, error) {
	return mediaapp.SignedRequest{Method: "POST", URL: "https://storage.test/" + object, ExpiresAt: expires, Headers: map[string]string{"Content-Type": contentType}}, nil
}

func (s *studyObjects) ReadURL(_ context.Context, object string, generation int64, _ time.Time) (string, error) {
	return "https://storage.test/" + object + "?generation=" + strconv.FormatInt(generation, 10), nil
}

func (s *studyObjects) Attrs(_ context.Context, object string) (mediadomain.ObjectAttrs, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if attrs, ok := s.stored[object]; ok {
		return attrs, nil
	}
	return mediadomain.ObjectAttrs{}, mediaapp.ErrObjectNotFound
}

func (s *studyObjects) Read(_ context.Context, object string, generation, limit int64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if attrs, ok := s.stored[object]; !ok || attrs.Generation != generation {
		return nil, mediaapp.ErrObjectNotFound
	}
	if int64(len(s.contents[object])) > limit {
		return nil, errors.New("object too large")
	}
	return s.contents[object], nil
}

func (s *studyObjects) put(object string, data []byte, size int64, crc uint32, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stored[object] = mediadomain.ObjectAttrs{Generation: 1757592000000000 + int64(len(s.stored)) + 1, Size: size, CRC32C: crc, ContentType: contentType}
	s.contents[object] = data
}

// studyWorld is one experiment built with SQL: a disease model study with a
// healthy control, a disease control, a positive control and two doses of
// compound X, an MWM protocol, Baseline and Day 14 timepoints, a
// primary endpoint (distance at day 14 against baseline) and three
// calibrated arenas.
type studyWorld struct {
	t          *testing.T
	pool       *pgxpool.Pool
	exp        string
	version    string
	timepoints map[string]string
	groups     map[string]string
	arenas     []string
	endpoint   string
	worker     string
	objects    int
}

func (w *studyWorld) id(sql string, args ...any) string {
	w.t.Helper()
	var out string
	if err := w.pool.QueryRow(context.Background(), sql, args...).Scan(&out); err != nil {
		w.t.Fatalf("%s: %v", sql, err)
	}
	return out
}

func (w *studyWorld) exec(sql string, args ...any) {
	w.t.Helper()
	if _, err := w.pool.Exec(context.Background(), sql, args...); err != nil {
		w.t.Fatalf("%s: %v", sql, err)
	}
}

func newStudyWorld(t *testing.T, pool *pgxpool.Pool, code string) *studyWorld {
	t.Helper()
	w := &studyWorld{t: t, pool: pool, timepoints: map[string]string{}, groups: map[string]string{}}
	substance := func(name, kind string) string {
		return w.id(`INSERT INTO misko.substances (name, kind) VALUES ($1, $2)
			ON CONFLICT (lower(name)) DO UPDATE SET kind = EXCLUDED.kind RETURNING id`, name, kind)
	}
	compound, reference, saline, mptp := substance("Compound X", "COMPOUND"), substance("Reference", "REFERENCE_DRUG"), substance("Saline", "VEHICLE"), substance("MPTP", "INDUCER")
	disease := w.id(`INSERT INTO misko.disease_models (name, induction_type) VALUES ('MPTP Parkinson', 'CHEMICAL')
		ON CONFLICT (lower(name)) DO UPDATE SET induction_type = EXCLUDED.induction_type RETURNING id`)
	w.exp = w.id("INSERT INTO misko.experiments (code, title) VALUES ($1, 'Compound X in MPTP mice') RETURNING id", code)
	group := func(code, role string, diseased bool) string {
		var model any
		if diseased {
			model = disease
		}
		w.groups[code] = w.id(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role, disease_model_id, target_size)
			VALUES ($1, $2, $2 || ' ' || $3, $3, $4, 8) RETURNING id`, w.exp, code, role, model)
		return w.groups[code]
	}
	regimen := func(group, purpose, substance string, doseMicro int64) {
		w.exec(`INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
			VALUES ($1, $2, $3, $4, $5, 'mg/kg', 'INTRAPERITONEAL', 'DAILY', 0, 14)`, w.exp, group, purpose, substance, doseMicro)
	}
	group("G1", "HEALTHY_CONTROL", false)
	regimen(group("G2", "DISEASE_CONTROL", true), "VEHICLE", saline, 10_000_000)
	regimen(w.groups["G2"], "INDUCTION", mptp, 20_000_000)
	regimen(group("G3", "POSITIVE_CONTROL", true), "TREATMENT", reference, 5_000_000)
	regimen(group("G4", "TREATMENT", true), "TREATMENT", compound, 1_000_000)
	regimen(group("G5", "TREATMENT", true), "TREATMENT", compound, 3_000_000)
	w.timepoints["Baseline"] = w.id("INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Baseline', -1, 1) RETURNING id", w.exp)
	w.timepoints["Day 14"] = w.id("INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Day 14', 14, 2) RETURNING id", w.exp)
	protocol := w.id("INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id", w.exp)
	if err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
			VALUES ($1, $2, 1, 1, $3) RETURNING id`, w.exp, protocol, studyAuthor).Scan(&w.version); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
			VALUES ($1, 1, 'MWM', 1, 'ACQUISITION', 1, '{}')`, w.version)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w.endpoint = w.id(`INSERT INTO misko.endpoints (experiment_id, timepoint_id, baseline_timepoint_id, paradigm_key, metric_key, kind, direction)
		VALUES ($1, $2, $3, 'MWM', 'path_length_cm', 'PRIMARY', 'HIGHER_IS_BETTER') RETURNING id`, w.exp, w.timepoints["Day 14"], w.timepoints["Baseline"])
	for i := range 3 {
		env := w.id("INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, 'MWM') RETURNING id", fmt.Sprintf("%s box %d", code, i+1))
		arena := w.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
			VALUES ($1, 'MWM', 1, 1, '{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30}', $2) RETURNING id`, env, studyAuthor)
		w.exec(`INSERT INTO misko.calibrations (environment_revision_id, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height,
			measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, created_by)
			VALUES ($1, 'cam', 1920, 1080, $2, 0, 640, 1080, 'ARENA_FLOOR', '[{},{},{},{}]', '[{},{},{}]', '{1,0,0,0,1,0,0,0,1}', 0.1, 0.1, 0.2, 2, 'v1', 'VALID', $3)`,
			arena, i*640, studyAuthor)
		w.arenas = append(w.arenas, arena)
	}
	return w
}

// allocate puts a new animal into a group.
func (w *studyWorld) allocate(group, code string) string {
	subject := w.id("INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id", code)
	return w.id(`INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
		VALUES ($1, $2, $3, now() - interval '3 days', $4) RETURNING id`, w.exp, w.groups[group], subject, studyAuthor)
}

func (w *studyWorld) start() {
	w.exec("UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1", w.exp)
}

// session plans a test at a timepoint with up to three animals, one per arena,
// starts it and attaches a verified video. It returns the test and its animals.
func (w *studyWorld) session(timepoint string, allocations ...string) (string, []string) {
	w.t.Helper()
	test := w.id(`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
		VALUES ($1, $2, $3, 1, 'MWM', 1, 1, now() - interval '1 hour', $4) RETURNING id`, w.exp, w.timepoints[timepoint], w.version, studyAuthor)
	var subjects []string
	for i, allocation := range allocations {
		subjects = append(subjects, w.id(`INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
			VALUES ($1, $2, $3, $4, 'MWM', 1, $5) RETURNING id`, w.exp, test, allocation, w.arenas[i], studyAuthor))
	}
	w.exec("UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() - interval '30 minutes' WHERE id = $1", test)
	w.objects++
	asset := w.id(`INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ORIGINAL', 'misko-test-videos', $1, 'video/mp4', 4096, 1, 'VERIFIED', 42, now(), $2) RETURNING id`, fmt.Sprintf("videos/%s/%d.mp4", w.exp, w.objects), studyAuthor)
	w.exec(`INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, clip_start_us, clip_end_us, created_by)
		VALUES ($1, $2, $3, 0, 6000000, $4)`, w.exp, test, asset, studyAuthor)
	return test, subjects
}

// publish stores a succeeded run of one animal with a distance value (nil
// for a missing value) the way the analysis context does, opens its result
// and records the review status.
func (w *studyWorld) publish(testSubject string, value *float64, status string) string {
	w.t.Helper()
	ctx := context.Background()
	if w.worker == "" {
		hash := make([]byte, 32)
		_, _ = rand.Read(hash)
		w.worker = w.id(`INSERT INTO misko.analysis_workers (name, model_version, token_sha256, created_by) VALUES ($1, 'fixture 1', $2, $3) RETURNING id`,
			"fixture-"+w.exp, hash, studyAuthor)
	}
	var run string
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO misko.analysis_runs (experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c,
				clip_start_us, clip_end_us, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, parameters, trigger, status, attempt,
				max_attempts, worker_id, lease_expires_at, available_at)
			SELECT ts.experiment_id, ts.test_id, ts.id, r.id, a.id, a.generation, a.crc32c, r.clip_start_us, r.clip_end_us, 'MWM', 1, 1, 1, '{}', 'MANUAL',
				'RUNNING', 1, 3, $2, now() + interval '1 hour', now()
			FROM misko.test_subjects ts JOIN misko.current_recordings r ON r.test_id = ts.test_id JOIN misko.video_assets a ON a.id = r.video_asset_id
			WHERE ts.id = $1 RETURNING id`, testSubject, w.worker).Scan(&run); err != nil {
			return err
		}
		object := "runs/" + run + "/attempts/1/overlay.mp4"
		var asset string
		for _, step := range []func() error{
			func() error {
				_, err := tx.Exec(ctx, `INSERT INTO misko.analysis_output_uploads (run_id, attempt, object_name, kind, content_type, size_bytes, crc32c)
					VALUES ($1, 1, $2, 'ANALYZED_VIDEO', 'video/mp4', 10, 1)`, run, object)
				return err
			},
			func() error {
				return tx.QueryRow(ctx, `INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
					VALUES ('ANALYZED', 'misko-test-videos', $1, 'video/mp4', 10, 1, 'VERIFIED', 1, now(), $2) RETURNING id`, object, w.worker).Scan(&asset)
			},
			func() error {
				_, err := tx.Exec(ctx, `INSERT INTO misko.analysis_artifacts (run_id, attempt, object_name, kind, bucket, generation, size_bytes, crc32c, content_type, video_asset_id)
					VALUES ($1, 1, $2, 'ANALYZED_VIDEO', 'misko-test-videos', 1, 10, 1, 'video/mp4', $3)`, run, object, asset)
				return err
			},
			func() error {
				_, err := tx.Exec(ctx, `INSERT INTO misko.analysis_video_pairs (run_id, attempt, source_asset_id, source_generation, analyzed_asset_id, source_offset_us, output_offset_us, time_mapping_version)
					SELECT id, 1, source_asset_id, source_generation, $2, 0, 0, 'identity-v1' FROM misko.analysis_runs WHERE id = $1`, run, asset)
				return err
			},
			func() error {
				var missing any
				if value == nil {
					missing = "NO_TRACKING"
				}
				_, err := tx.Exec(ctx, `INSERT INTO misko.metric_results (run_id, attempt, metric_key, unit, value, missing_reason) VALUES ($1, 1, 'path_length_cm', 'cm', $2, $3)`,
					run, value, missing)
				return err
			},
			func() error {
				_, err := tx.Exec(ctx, `UPDATE misko.analysis_runs SET status = 'SUCCEEDED', model_version = 'fixture 1', lease_expires_at = NULL, finished_at = now() WHERE id = $1`, run)
				return err
			},
			func() error {
				_, err := tx.Exec(ctx, `INSERT INTO misko.subject_results (test_subject_id, run_id) VALUES ($1, $2) ON CONFLICT (test_subject_id) DO NOTHING`, testSubject, run)
				return err
			},
		} {
			if err := step(); err != nil {
				return err
			}
		}
		if status == "PENDING" {
			return nil
		}
		var reason any
		if status == "REJECTED" {
			reason = "OTHER"
		}
		_, err := tx.Exec(ctx, `UPDATE misko.subject_results SET status = $2, reason = $3, reviewed_by = $4, reviewed_at = now() WHERE test_subject_id = $1`,
			testSubject, status, reason, studyAuthor)
		return err
	})
	if err != nil {
		w.t.Fatalf("publish %s: %v", testSubject, err)
	}
	return run
}

func distance(v float64) *float64 { return &v }

// The worker protocol for one animal of a shared video, then the review of
// its result, over HTTP.
func TestAnalysisOverHTTP(t *testing.T) {
	ctx := context.Background()
	objects := newStudyObjects()
	a := newStudyAPI(t, objects)
	w := newStudyWorld(t, a.pool, "EXP-A")
	m1, m2, m3 := w.allocate("G1", "M-1"), w.allocate("G2", "M-2"), w.allocate("G4", "M-3")
	w.start()
	test, subjects := w.session("Day 14", m1, m2, m3)
	// The third animal's arena has a rejected override for this video: it waits.
	w.exec(`INSERT INTO misko.calibrations (environment_revision_id, test_subject_id, recording_id, reference_frame_us, camera_id, frame_width, frame_height,
		crop_x, crop_y, crop_width, crop_height, measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm,
		tolerance_cm, algorithm_version, status, rejection_reason, created_by)
		SELECT ts.environment_revision_id, ts.id, r.id, 0, 'cam', 1920, 1080, 1280, 0, 640, 1080, 'ARENA_FLOOR', '[{},{},{},{}]', '[{},{},{}]', '{1,0,0,0,1,0,0,0,1}',
		5, 5, 5, 2, 'v1', 'REJECTED', 'EXCESSIVE_CHECK_ERROR', $2
		FROM misko.test_subjects ts JOIN misko.current_recordings r ON r.test_id = ts.test_id WHERE ts.id = $1`, subjects[2], studyAuthor)

	// Workers are registered by lab managers and get their token once.
	register := map[string]any{"name": "tracker", "modelVersion": "tracker 1.0", "capabilities": []any{map[string]any{"paradigmKey": "MWM", "paradigmVersion": 1}}}
	a.expect("viewer registers a worker", "POST", "/api/analysis/workers", "VIEWER", register, 403, "auth.forbidden")
	token := a.expect("register", "POST", "/api/analysis/workers", "SUPERADMIN", register, 201, "")["token"].(string)
	worker := "Worker " + token

	if err := a.api.AnalysisTick(ctx); err != nil {
		t.Fatal(err)
	}
	runs := a.expect("runs", "GET", "/api/tests/"+test+"/analysis-runs", "VIEWER", nil, 200, "")["data"].([]any)
	if len(runs) != 2 {
		t.Fatalf("one run per calibrated animal: %v", runs)
	}
	one := a.expect("runs of one animal", "GET", "/api/tests/"+test+"/analysis-runs?testSubjectId="+subjects[0], "VIEWER", nil, 200, "")["data"].([]any)
	if len(one) != 1 || one[0].(map[string]any)["testSubjectId"] != subjects[0] || one[0].(map[string]any)["calibrationId"] == nil {
		t.Fatalf("runs of the first animal: %v", one)
	}

	if status, body := a.send("POST", "/api/worker/claim", "", map[string]any{}); status != 401 || body["code"] != "worker.unauthenticated" {
		t.Fatalf("claim without a worker token: %d %v", status, body)
	}
	a.expect("user token on a worker route", "POST", "/api/worker/claim", "SUPERADMIN", map[string]any{}, 401, "worker.unauthenticated")
	if status, _ := a.send("GET", "/api/tests/"+test+"/analysis-runs", worker, nil); status != 401 {
		t.Fatalf("worker token on a user route: %d", status)
	}
	var claim map[string]any
	for range 2 {
		status, body := a.send("POST", "/api/worker/claim", worker, map[string]any{})
		if status != 200 {
			t.Fatalf("claim: %d %v", status, body)
		}
		if body["run"].(map[string]any)["testSubjectId"] == subjects[0] {
			claim = body
		}
	}
	if status, _ := a.send("POST", "/api/worker/claim", worker, map[string]any{}); status != 204 {
		t.Fatalf("third claim: %d", status)
	}
	run := claim["run"].(map[string]any)
	runID, prefix := run["id"].(string), claim["outputPrefix"].(string)
	calibration := claim["calibration"].(map[string]any)
	subject := claim["testSubject"].(map[string]any)
	if run["attempt"] != float64(1) || subject["id"] != subjects[0] || subject["environmentRevisionId"] != w.arenas[0] ||
		calibration["id"] != run["calibrationId"] || calibration["crop"].(map[string]any)["width"] != float64(640) ||
		len(claim["contract"].(map[string]any)["metrics"].([]any)) != 8 || claim["trajectorySchema"] != "misko.trajectory.v1" {
		t.Fatalf("job: %v", claim)
	}
	runPath := "/api/worker/runs/" + runID
	if status, body := a.send("POST", runPath+"/source-url", worker, map[string]any{"attempt": 1}); status != 200 || !strings.Contains(body["url"].(string), "generation=42") {
		t.Fatalf("source URL: %d %v", status, body)
	}
	if status, body := a.send("POST", runPath+"/heartbeat", worker, map[string]any{"attempt": 2}); status != 409 || body["code"] != "analysis.staleAttempt" {
		t.Fatalf("stale heartbeat: %d %v", status, body)
	}
	if status, body := a.send("POST", runPath+"/outputs", worker, map[string]any{"attempt": 1, "kind": "ANALYZED_VIDEO", "fileName": "overlay.mp4",
		"contentType": "video/mp4", "sizeBytes": 2048, "crc32c": mediadomain.FormatCRC32C(5)}); status != 201 || body["objectName"] != prefix+"overlay.mp4" {
		t.Fatalf("output: %d %v", status, body)
	}
	objects.put(prefix+"overlay.mp4", nil, 2048, 5, "video/mp4")

	// The worker swims 10 cm across the 120 cm pool, then stays still.
	var ts []int64
	var xs, ys, confidence []float64
	var tracked []bool
	for i := 0; i <= 25; i++ {
		ts, xs, ys = append(ts, int64(i)*100_000), append(xs, 5+float64(min(i, 10))), append(ys, 20)
		tracked, confidence = append(tracked, true), append(confidence, 0.95)
	}
	trajectory, _ := json.Marshal(map[string]any{"schema": "misko.trajectory.v1", "runId": runID, "attempt": 1, "durationUs": 6_000_000,
		"samples": map[string]any{"tUs": ts, "xCm": xs, "yCm": ys, "tracked": tracked, "confidence": confidence}})
	if status, body := a.send("POST", runPath+"/outputs", worker, map[string]any{"attempt": 1, "kind": "TRAJECTORY", "fileName": "trajectory.json",
		"contentType": "application/json", "sizeBytes": len(trajectory), "crc32c": mediadomain.FormatCRC32C(6)}); status != 201 {
		t.Fatalf("trajectory output: %d %v", status, body)
	}
	objects.put(prefix+"trajectory.json", trajectory, int64(len(trajectory)), 6, "application/json")
	result := map[string]any{"attempt": 1, "modelVersion": "tracker 1.0",
		"artifacts": []any{map[string]any{"kind": "ANALYZED_VIDEO", "objectName": prefix + "overlay.mp4"}, map[string]any{"kind": "TRAJECTORY", "objectName": prefix + "trajectory.json"}},
		"pair":      map[string]any{"analyzedObjectName": prefix + "overlay.mp4", "sourceOffsetUs": 0, "outputOffsetUs": 0, "timeMappingVersion": "identity-v1"}}
	if status, body := a.send("POST", runPath+"/result", worker, result); status != 200 || body["status"] != "SUCCEEDED" {
		t.Fatalf("result: %d %v", status, body)
	}
	detail := a.expect("run detail", "GET", "/api/analysis-runs/"+runID, "VIEWER", nil, 200, "")
	if published := detail["result"].(map[string]any); len(published["metrics"].([]any)) != 8 || published["pair"] == nil {
		t.Fatalf("detail: %v", detail)
	}

	// The first success opened the animal's result for review.
	results := "/api/analysis/results"
	page := a.expect("pending results", "GET", results+"?experimentId="+w.exp, "VIEWER", nil, 200, "")
	items := page["data"].([]any)
	if page["total"] != float64(1) || len(items) != 1 {
		t.Fatalf("pending: %v", page)
	}
	item := items[0].(map[string]any)
	if item["testSubjectId"] != subjects[0] || item["status"] != "PENDING" || item["subjectCode"] != "M-1" || item["blinded"] != false ||
		item["group"].(map[string]any)["code"] != "G1" || item["arena"].(map[string]any)["environmentRevisionId"] != w.arenas[0] ||
		len(item["run"].(map[string]any)["metrics"].([]any)) != 8 || item["newerRunAvailable"] != false {
		t.Fatalf("review item: %v", item)
	}
	a.expect("bad status", "GET", results+"?status=DONE", "VIEWER", nil, 400, "analysis.invalidQuery")
	resultPath := results + "/" + subjects[0]
	a.expect("viewer accepts", "POST", resultPath+"/accept", "VIEWER", map[string]any{}, 403, "auth.forbidden")
	a.expect("reject without reason", "POST", resultPath+"/reject", "TECHNICIAN", map[string]any{}, 400, "analysis.invalidRejection")
	rejected := a.expect("reject", "POST", resultPath+"/reject", "TECHNICIAN", map[string]any{"reason": "TRACKING_LOST", "note": "tail tracked"}, 200, "")
	if rejected["status"] != "REJECTED" || rejected["reason"] != "TRACKING_LOST" || rejected["reviewedBy"] == nil {
		t.Fatalf("rejected: %v", rejected)
	}
	a.expect("no result yet", "POST", results+"/"+subjects[1]+"/accept", "TECHNICIAN", map[string]any{}, 404, "analysis.resultNotFound")

	// Reanalysis per animal; a new run never replaces the reviewed one.
	reanalyse := "/api/analysis/test-subjects/" + subjects[0] + "/runs"
	a.expect("viewer reanalyses", "POST", reanalyse, "VIEWER", map[string]any{}, 403, "auth.forbidden")
	a.expect("waiting arena", "POST", "/api/analysis/test-subjects/"+subjects[2]+"/runs", "TECHNICIAN", map[string]any{}, 409, "analysis.notReady")
	a.expect("unknown animal", "POST", "/api/analysis/test-subjects/"+w.exp+"/runs", "TECHNICIAN", map[string]any{}, 404, "analysis.testSubjectNotFound")
	manual := a.expect("reanalyse", "POST", reanalyse, "TECHNICIAN", map[string]any{}, 201, "")
	if manual["trigger"] != "MANUAL" || manual["testSubjectId"] != subjects[0] {
		t.Fatalf("manual run: %v", manual)
	}
	a.expect("reanalyse again", "POST", reanalyse, "TECHNICIAN", map[string]any{}, 409, "analysis.runPending")
	a.expect("choose an unfinished run", "POST", resultPath+"/run", "TECHNICIAN", map[string]any{"runId": manual["id"]}, 409, "analysis.runNotChosen")
	a.expect("choose an unknown run", "POST", resultPath+"/run", "TECHNICIAN", map[string]any{"runId": w.exp}, 404, "analysis.runNotFound")
	newer := w.publish(subjects[0], distance(30), "PENDING")
	if got := a.expect("result", "GET", resultPath, "VIEWER", nil, 200, ""); got["newerRunAvailable"] != true || got["latestRunId"] != newer ||
		got["status"] != "REJECTED" || len(got["runs"].([]any)) != 2 {
		t.Fatalf("newer run available: %v", got)
	}
	chosen := a.expect("choose the newer run", "POST", resultPath+"/run", "TECHNICIAN", map[string]any{"runId": newer}, 200, "")
	if chosen["status"] != "PENDING" || chosen["run"].(map[string]any)["id"] != newer || chosen["reason"] != nil {
		t.Fatalf("chosen: %v", chosen)
	}

	// In a blinded study the reviewer never sees the group.
	w.publish(subjects[1], distance(12), "PENDING")
	w.exec("UPDATE misko.experiments SET blinded = true WHERE id = $1", w.exp)
	blind := a.expect("blinded list", "GET", results+"?experimentId="+w.exp, "VIEWER", nil, 200, "")["data"].([]any)
	if len(blind) != 2 {
		t.Fatalf("blinded list: %v", blind)
	}
	for _, i := range blind {
		if i.(map[string]any)["group"] != nil || i.(map[string]any)["blinded"] != true {
			t.Fatalf("blinded item reveals its group: %v", i)
		}
	}
	a.expect("bulk with a duplicate", "POST", results+"/bulk-accept", "TECHNICIAN", map[string]any{"testSubjectIds": []string{subjects[0], subjects[0]}}, 400, "analysis.invalidBulk")
	bulk := a.expect("bulk accept", "POST", results+"/bulk-accept", "RESEARCHER", map[string]any{"testSubjectIds": []string{subjects[0], subjects[1]}, "note": "clean"}, 200, "")["data"].([]any)
	if len(bulk) != 2 || bulk[0].(map[string]any)["status"] != "ACCEPTED" || bulk[1].(map[string]any)["group"] != nil {
		t.Fatalf("bulk: %v", bulk)
	}
	if left := a.expect("nothing pending", "GET", results, "VIEWER", nil, 200, ""); left["total"] != float64(0) {
		t.Fatalf("pending after bulk: %v", left)
	}

	// Completing the experiment freezes every decision.
	w.exec("UPDATE misko.tests SET status = 'COMPLETED', completed_at = now() WHERE id = $1", test)
	w.exec("UPDATE misko.experiments SET status = 'COMPLETED', completed_at = now() WHERE id = $1", w.exp)
	a.expect("review after completion", "POST", resultPath+"/reject", "TECHNICIAN", map[string]any{"reason": "OTHER"}, 409, "analysis.experimentClosed")
}
