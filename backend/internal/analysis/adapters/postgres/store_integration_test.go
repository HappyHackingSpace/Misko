//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/catalog"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/token"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	ctx        = context.Background()
	manager    = access.Actor{UserID: "01a00000-0000-7000-8000-000000000001", Role: access.LabManager}
	technician = access.Actor{UserID: "01a00000-0000-7000-8000-000000000002", Role: access.Technician}
	viewer     = access.Actor{UserID: "01a00000-0000-7000-8000-000000000003", Role: access.Viewer}
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func (c *clock) advance(d time.Duration) { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }

// objects is an in-memory object store; it is not evidence of GCS behavior.
type objects struct {
	mu       sync.Mutex
	stored   map[string]application.ObjectAttrs
	contents map[string][]byte
	gen      int64
}

func (o *objects) Bucket() string { return "misko-test-videos" }

func (o *objects) UploadURL(_ context.Context, object, contentType string, expires time.Time) (application.SignedRequest, error) {
	return application.SignedRequest{Method: "POST", URL: "https://storage.test/" + object, ExpiresAt: expires}, nil
}

func (o *objects) ReadURL(_ context.Context, object string, generation int64, _ time.Time) (string, error) {
	return fmt.Sprintf("https://storage.test/%s?generation=%d", object, generation), nil
}

func (o *objects) Attrs(_ context.Context, object string) (application.ObjectAttrs, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if a, ok := o.stored[object]; ok {
		return a, nil
	}
	return application.ObjectAttrs{}, application.ErrObjectNotFound
}

func (o *objects) Read(_ context.Context, object string, generation, limit int64) ([]byte, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	a, ok := o.stored[object]
	if !ok || a.Generation != generation {
		return nil, application.ErrObjectNotFound
	}
	if int64(len(o.contents[object])) > limit {
		return nil, errors.New("object too large")
	}
	return o.contents[object], nil
}

func (o *objects) put(object string, size int64, crc uint32, contentType string) {
	o.putData(object, nil, size, crc, contentType)
}

func (o *objects) putData(object string, data []byte, size int64, crc uint32, contentType string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.gen++
	o.stored[object] = application.ObjectAttrs{Generation: 1_757_600_000_000_000 + o.gen, Size: size, CRC32C: crc, ContentType: contentType}
	o.contents[object] = data
}

func id(t *testing.T, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, sql string, args ...any) string {
	t.Helper()
	var out string
	if err := q.QueryRow(ctx, sql, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// fixture is a running experiment with one MWM session (test) in
// progress: three animals in three arenas share one verified video (clip 1 s
// to 9 s). Arenas A and B have a valid default calibration, arena C none.
type fixture struct {
	t                        *testing.T
	pool                     *pgxpool.Pool
	clock                    *clock
	objects                  *objects
	s                        *application.Service
	exp, group, timepoint    string
	version, test, recording string
	a, b, c                  string
	arenaA, arenaB, arenaC   string
	allocA, allocB, allocC   string
	calA, calB               string
}

const arenaParameters = `{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 15, "platform_y_cm": 25}`

// calibrationSQL inserts a calibration: $1 arena, $2 test subject or NULL, $3
// recording or NULL, $4 reference frame or NULL, $5 crop x, $6 supersedes or
// NULL, $7 status, $8 rejection reason or NULL, $9 author.
const calibrationSQL = `INSERT INTO misko.calibrations (environment_revision_id, test_subject_id, recording_id, reference_frame_us, supersedes_id,
	camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height, measurement_plane, fit_points, check_points, transform,
	fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, rejection_reason, created_by)
	VALUES ($1, $2, $3, $4, $6, 'cam', 1920, 1080, $5, 0, 640, 1080, 'ARENA_FLOOR', '[{},{},{},{}]', '[{},{},{}]', '{1,0,0,0,1,0,0,0,1}',
	0, 0, 0, 2, 'test', $7, $8, $9) RETURNING id`

func newFixture(t *testing.T) *fixture {
	pool := pgtest.Database(t)
	f := &fixture{t: t, pool: pool, clock: &clock{now: time.Now().UTC().Truncate(time.Second)}, objects: &objects{stored: map[string]application.ObjectAttrs{}, contents: map[string][]byte{}}}
	f.s = application.New(postgres.NewStore(pool), f.objects, catalog.New(), token.New(),
		application.Settings{Lease: time.Minute, MaxAttempts: 2, UploadTTL: time.Hour, ReadTTL: time.Minute, MaxOutputBytes: 1 << 30, MaxTrajectoryBytes: 1 << 20}, f.clock.Now)
	f.exp = id(t, pool, "INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id")
	f.group = id(t, pool, "INSERT INTO misko.experiment_groups (experiment_id, code, name, role, target_size) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL', 8) RETURNING id", f.exp)
	f.timepoint = id(t, pool, "INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Day 7', 7, 1) RETURNING id", f.exp)
	protocol := id(t, pool, "INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id", f.exp)
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		f.version = id(t, tx, "INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by) VALUES ($1, $2, 1, 1, $3) RETURNING id", f.exp, protocol, manager.UserID)
		_, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
			VALUES ($1, 1, 'MWM', 1, 'ACQUISITION', 1, '{"max_sample_gap_s": 0.25}')`, f.version)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.arenaA, f.arenaB, f.arenaC = f.arena("Box A", arenaParameters), f.arena("Box B", arenaParameters), f.arena("Box C", arenaParameters)
	f.allocA, f.allocB, f.allocC = f.allocate("M1"), f.allocate("M2"), f.allocate("M3")
	if _, err := pool.Exec(ctx, "UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1", f.exp); err != nil {
		t.Fatal(err)
	}
	f.test = f.newTest()
	f.a, f.b, f.c = f.join(f.test, f.allocA, f.arenaA), f.join(f.test, f.allocB, f.arenaB), f.join(f.test, f.allocC, f.arenaC)
	f.recording = f.start(f.test, "tests/source-a", "now()")
	id(t, pool, "INSERT INTO misko.trials (test_id, test_subject_id, number, repetition, attempt, started_at, recorded_by) VALUES ($1, $2, 1, 1, 1, now() - interval '20 hours', $3) RETURNING id",
		f.test, f.a, technician.UserID)
	f.calA = id(t, pool, calibrationSQL, f.arenaA, nil, nil, nil, 0, nil, "VALID", nil, technician.UserID)
	f.calB = id(t, pool, calibrationSQL, f.arenaB, nil, nil, nil, 640, nil, "VALID", nil, technician.UserID)
	return f
}

func (f *fixture) arena(name, parameters string) string {
	env := id(f.t, f.pool, "INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, 'MWM') RETURNING id", name)
	return id(f.t, f.pool, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'MWM', 1, 1, $2, $3) RETURNING id`, env, parameters, manager.UserID)
}

func (f *fixture) allocate(code string) string {
	subject := id(f.t, f.pool, "INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id", code)
	return id(f.t, f.pool, `INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
		VALUES ($1, $2, $3, now() - interval '3 days', $4) RETURNING id`, f.exp, f.group, subject, manager.UserID)
}

func (f *fixture) newTest() string {
	return id(f.t, f.pool, `INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
		VALUES ($1, $2, $3, 1, 'MWM', 1, 1, now() - interval '2 days', $4) RETURNING id`, f.exp, f.timepoint, f.version, manager.UserID)
}

func (f *fixture) join(test, allocation, arena string) string {
	return id(f.t, f.pool, `INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
		VALUES ($1, $2, $3, $4, 'MWM', 1, $5) RETURNING id`, f.exp, test, allocation, arena, manager.UserID)
}

// start puts a planned test in progress and attaches a verified video created
// at the given SQL time.
func (f *fixture) start(test, object, createdAt string) string {
	if _, err := f.pool.Exec(ctx, "UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() - interval '1 day' WHERE id = $1", test); err != nil {
		f.t.Fatal(err)
	}
	asset := id(f.t, f.pool, `INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ORIGINAL', 'misko-test-videos', $1, 'video/mp4', 4096, 1, 'VERIFIED', 42, now(), $2) RETURNING id`, object, technician.UserID)
	return id(f.t, f.pool, `INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, clip_start_us, clip_end_us, created_by, created_at)
		VALUES ($1, $2, $3, 1000000, 9000000, $4, `+createdAt+`) RETURNING id`, f.exp, test, asset, technician.UserID)
}

func (f *fixture) runsOf(testSubjectID string) []domain.Run {
	f.t.Helper()
	runs, err := f.s.Runs(ctx, viewer, f.test, testSubjectID)
	if err != nil {
		f.t.Fatal(err)
	}
	return runs
}

func (f *fixture) result(testSubjectID string) (status, runID string) {
	f.t.Helper()
	err := f.pool.QueryRow(ctx, "SELECT status, run_id FROM misko.subject_results WHERE test_subject_id = $1", testSubjectID).Scan(&status, &runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ""
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return status, runID
}

func (f *fixture) count(table, runID string) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM misko."+table+" WHERE run_id = $1", runID).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// assertRejected expects a database error: the named constraint, or any
// integrity or raised error when constraint is empty, never a mistyped statement.
func assertRejected(t *testing.T, name string, pool *pgxpool.Pool, constraint, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(ctx, sql, args...)
	var pgErr *pgconn.PgError
	switch {
	case !errors.As(err, &pgErr):
		t.Errorf("%s: direct SQL was not rejected by the database: %v", name, err)
	case constraint != "" && pgErr.ConstraintName != constraint:
		t.Errorf("%s: constraint %q (%s), want %q", name, pgErr.ConstraintName, pgErr.Message, constraint)
	case constraint == "" && !strings.HasPrefix(pgErr.Code, "23") && pgErr.Code != "P0001":
		t.Errorf("%s: SQLSTATE %s (%s)", name, pgErr.Code, pgErr.Message)
	}
}

func (f *fixture) worker(name string) (domain.Worker, string) {
	f.t.Helper()
	registered, err := f.s.RegisterWorker(ctx, manager, application.WorkerInput{Name: name, ModelVersion: "tracker 1.0", Capabilities: []domain.Capability{{ParadigmKey: "MWM", ParadigmVersion: 1}}})
	if err != nil {
		f.t.Fatal(err)
	}
	return registered.Worker, registered.Token
}

// submission names an analyzed video and a trajectory; the API computes the metrics.
func (f *fixture) submission(run domain.Run) domain.Submission {
	return domain.Submission{Attempt: run.Attempt, ModelVersion: "tracker 1.0",
		Artifacts: []domain.ArtifactInput{{Kind: domain.AnalyzedVideo, ObjectName: run.OutputPrefix() + "overlay.mp4"}, {Kind: domain.Trajectory, ObjectName: run.OutputPrefix() + "trajectory.json"}},
		Pair:      domain.PairInput{AnalyzedObjectName: run.OutputPrefix() + "overlay.mp4", SourceOffsetUs: 1_000_000, TimeMappingVersion: "identity-v1"},
	}
}

// trajectory walks along y = 25 cm from x = 5 cm, one centimeter per 0.1 s, for
// distance centimeters (reaching the platform of the fixture arenas at x = 10), then
// stays still for 1.5 s. lostEvery > 0 marks every lostEvery-th sample lost.
func (f *fixture) trajectory(run domain.Run, distance, lostEvery int) []byte {
	f.t.Helper()
	var ts []int64
	var xs, ys []*float64
	var tracked []bool
	var confidence []float64
	add := func(i int, x float64) {
		ts = append(ts, int64(i)*100_000)
		lost := lostEvery > 0 && i%lostEvery == lostEvery-1
		if lost {
			xs, ys, tracked, confidence = append(xs, nil), append(ys, nil), append(tracked, false), append(confidence, 0)
			return
		}
		y := 25.0
		xs, ys, tracked, confidence = append(xs, &x), append(ys, &y), append(tracked, true), append(confidence, 0.9)
	}
	for i := 0; i <= distance; i++ {
		add(i, 5+float64(i))
	}
	for i := distance + 1; i <= distance+15; i++ {
		add(i, 5+float64(distance))
	}
	data, err := json.Marshal(map[string]any{
		"schema": domain.TrajectorySchema, "runId": run.ID, "attempt": run.Attempt, "durationUs": 8_000_000,
		"samples": map[string]any{"tUs": ts, "xCm": xs, "yCm": ys, "tracked": tracked, "confidence": confidence},
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return data
}

// requestOutputs declares the overlay and the given trajectory for the run's attempt.
func (f *fixture) requestOutputs(w domain.Worker, run domain.Run, trajectory []byte) {
	f.t.Helper()
	for _, o := range []application.OutputRequest{
		{Attempt: run.Attempt, Kind: domain.AnalyzedVideo, FileName: "overlay.mp4", ContentType: "video/mp4", SizeBytes: 4096, CRC32C: 7},
		{Attempt: run.Attempt, Kind: domain.Trajectory, FileName: "trajectory.json", ContentType: "application/json", SizeBytes: int64(len(trajectory)), CRC32C: 8},
	} {
		if _, _, err := f.s.RequestOutput(ctx, w, run.ID, o); err != nil {
			f.t.Fatal(err)
		}
	}
}

// store uploads the overlay and the trajectory as declared.
func (f *fixture) store(run domain.Run, trajectory []byte) {
	f.objects.put(run.OutputPrefix()+"overlay.mp4", 4096, 7, "video/mp4")
	f.objects.putData(run.OutputPrefix()+"trajectory.json", trajectory, int64(len(trajectory)), 8, "application/json")
}

func TestSchedulingLeasesAndPublication(t *testing.T) {
	f := newFixture(t)
	a, tokenA := f.worker("tracker-a")
	b, _ := f.worker("tracker-b")

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.s.Tick(ctx); err != nil {
				t.Errorf("tick: %v", err)
			}
		}()
	}
	wg.Wait()
	runs, err := f.s.Runs(ctx, viewer, f.test, "")
	if err != nil || len(runs) != 2 {
		t.Fatalf("concurrent schedulers must create one job per calibrated animal: %+v %v", runs, err)
	}
	pinned := map[string]string{}
	for _, r := range runs {
		pinned[r.TestSubjectID] = r.CalibrationID
		if r.RecordingID != f.recording || r.Trigger != domain.Automatic || r.SourceGeneration != 42 || r.Parameters["tank_diameter_cm"] != 120 || r.Parameters["max_sample_gap_s"] != 0.25 {
			t.Fatalf("run inputs: %+v", r)
		}
	}
	if pinned[f.a] != f.calA || pinned[f.b] != f.calB {
		t.Fatalf("each animal pins its own arena's calibration: %v", pinned)
	}
	if report, _ := f.s.Tick(ctx); report.Enqueued != 0 || len(report.Skipped) != 0 {
		t.Fatalf("the same inputs were enqueued again: %+v", report)
	}
	assertRejected(t, "second automatic run for the same inputs", f.pool, "analysis_runs_automatic_key",
		`INSERT INTO misko.analysis_runs (experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
			calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, parameters, trigger, status, max_attempts, available_at)
		SELECT experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
			calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, parameters, trigger, 'QUEUED', max_attempts, available_at
		FROM misko.analysis_runs WHERE id = $1`, runs[0].ID)

	var mu sync.Mutex
	var claimed []application.Job
	var owners []domain.Worker
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := []domain.Worker{a, b}[i%2]
			job, err := f.s.Claim(ctx, w)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				claimed, owners = append(claimed, job), append(owners, w)
			case !errors.Is(err, application.ErrNoRun):
				t.Errorf("claim: %v", err)
			}
		}()
	}
	wg.Wait()
	if len(claimed) != 2 || claimed[0].Run.ID == claimed[1].Run.ID || claimed[0].Run.Attempt != 1 || len(claimed[0].Contract.Metrics) != 8 {
		t.Fatalf("each run is claimed exactly once: %+v", claimed)
	}
	for _, job := range claimed {
		arena, crop := f.arenaA, 0
		if job.Run.TestSubjectID == f.b {
			arena, crop = f.arenaB, 640
		}
		if job.Arena != arena || job.Calibration == nil || job.Calibration.ID != job.Run.CalibrationID || job.Calibration.CropX != crop || job.Calibration.CropWidth != 640 {
			t.Fatalf("the job carries the animal's arena and crop: %+v %+v", job, job.Calibration)
		}
	}
	first, holder := claimed[0].Run, owners[0]
	other := a
	if holder.ID == a.ID {
		other = b
	}
	if _, err := f.s.Heartbeat(ctx, other, first.ID, 1); !errors.Is(err, domain.ErrStaleAttempt) {
		t.Fatalf("heartbeat by another worker: %v", err)
	}

	// Both workers crash: the leases expire and the other worker takes attempt 2.
	f.clock.advance(2 * time.Minute)
	if report, err := f.s.Tick(ctx); err != nil || report.Requeued != 2 {
		t.Fatalf("expired leases: %+v %v", report, err)
	}
	var job application.Job
	for range 2 {
		next, err := f.s.Claim(ctx, other)
		if err != nil {
			t.Fatal(err)
		}
		if next.Run.ID == first.ID {
			job = next
			continue
		}
		if _, err := f.s.Fail(ctx, other, next.Run.ID, next.Run.Attempt, "stopped", false); err != nil {
			t.Fatal(err)
		}
	}
	run := job.Run
	if run.Attempt != 2 {
		t.Fatalf("reclaim: %+v", job)
	}
	if _, _, err := f.s.RequestOutput(ctx, holder, run.ID, application.OutputRequest{Attempt: 1, Kind: domain.AnalyzedVideo, FileName: "late.mp4", ContentType: "video/mp4", SizeBytes: 1, CRC32C: 1}); !errors.Is(err, domain.ErrStaleAttempt) {
		t.Fatalf("late attempt requests output: %v", err)
	}
	if _, err := f.s.Submit(ctx, holder, run.ID, f.submission(first)); !errors.Is(err, domain.ErrStaleAttempt) {
		t.Fatalf("late attempt submits: %v", err)
	}

	good := f.trajectory(run, 10, 0)
	size := int64(len(good))
	f.requestOutputs(other, run, good)
	f.objects.put(run.OutputPrefix()+"overlay.mp4", 4096, 7, "video/mp4")
	if _, err := f.s.Submit(ctx, other, run.ID, f.submission(run)); !errors.Is(err, application.ErrOutputNotVerified) {
		t.Fatalf("missing trajectory: %v", err)
	}
	for name, stored := range map[string]application.ObjectAttrs{
		"corrupt":            {Size: size, CRC32C: 9, ContentType: "application/json"},
		"truncated":          {Size: size - 1, CRC32C: 8, ContentType: "application/json"},
		"wrong content type": {Size: size, CRC32C: 8, ContentType: "application/octet-stream"},
	} {
		f.objects.putData(run.OutputPrefix()+"trajectory.json", good, stored.Size, stored.CRC32C, stored.ContentType)
		if _, err := f.s.Submit(ctx, other, run.ID, f.submission(run)); !errors.Is(err, application.ErrOutputNotVerified) {
			t.Fatalf("%s trajectory: %v", name, err)
		}
	}
	// A verified object whose content belongs to the crashed attempt is refused.
	f.objects.putData(run.OutputPrefix()+"trajectory.json", f.trajectory(first, 10, 0), size, 8, "application/json")
	if _, err := f.s.Submit(ctx, other, run.ID, f.submission(run)); !errors.Is(err, domain.ErrInvalidTrajectory) {
		t.Fatalf("trajectory of the previous attempt: %v", err)
	}
	for _, table := range []string{"metric_results", "analysis_events", "analysis_artifacts", "analysis_video_pairs"} {
		if n := f.count(table, run.ID); n != 0 {
			t.Fatalf("%s has %d rows after a rejected submission", table, n)
		}
	}
	if status, _ := f.result(run.TestSubjectID); status != "" {
		t.Fatalf("no result before a success: %s", status)
	}
	f.store(run, good)
	undeclared := f.submission(run)
	undeclared.Artifacts = append(undeclared.Artifacts, domain.ArtifactInput{Kind: domain.Thumbnail, ObjectName: run.OutputPrefix() + "thumb.png"})
	f.objects.put(run.OutputPrefix()+"thumb.png", 4096, 7, "image/png")
	if _, err := f.s.Submit(ctx, other, run.ID, undeclared); !errors.Is(err, application.ErrUnknownOutput) {
		t.Fatalf("output uploaded without being declared: %v", err)
	}
	done, err := f.s.Submit(ctx, other, run.ID, f.submission(run))
	if err != nil || done.Status != domain.Succeeded || done.ModelVersion != "tracker 1.0" {
		t.Fatalf("publish: %+v %v", done, err)
	}
	// The first success opens the animal's result for review.
	if status, resultRun := f.result(run.TestSubjectID); status != "PENDING" || resultRun != run.ID {
		t.Fatalf("result after the first success: %s %s", status, resultRun)
	}

	// Duplicate delivery of the accepted attempt publishes nothing new.
	again, err := f.s.Submit(ctx, other, run.ID, f.submission(run))
	if err != nil || again.Status != domain.Succeeded {
		t.Fatalf("duplicate delivery: %+v %v", again, err)
	}
	detail, err := f.s.RunDetail(ctx, viewer, run.ID)
	if err != nil || detail.Published == nil || len(detail.Published.Metrics) != 8 || len(detail.Published.Artifacts) != 2 ||
		detail.Published.Pair == nil || detail.Published.Pair.SourceGeneration != 42 || detail.Published.Pair.SourceOffsetUs != 1_000_000 {
		t.Fatalf("published result: %+v %v", detail.Published, err)
	}
	// The Go engine computed these from the stored samples: 25 counted 0.1 s
	// intervals, reaching the platform (radius 5 cm around x = 15) at 0.5 s and
	// staying on it, so the escape is at 0.5 s after 5 cm of swimming.
	metrics := map[string]domain.MetricValue{}
	for _, m := range detail.Published.Metrics {
		metrics[m.Key] = m
	}
	for key, want := range map[string]float64{"path_length_cm": 5, "duration_s": 2.5, "escape_latency_s": 0.5, "mean_swim_speed_cm_s": 10,
		"platform_crossings_count": 1, "target_quadrant_time_ratio": 1, "thigmotaxis_time_ratio": 0} {
		if m := metrics[key]; m.Value == nil || math.Abs(*m.Value-want) > 1e-9 {
			t.Errorf("%s: %+v want %v", key, m, want)
		}
	}
	wantEvents := []domain.Event{
		{Type: "in_platform", Kind: "INTERVAL", StartUs: 500_000, EndUs: 2_500_000},
		{Type: "platform_entry", Kind: "POINT", StartUs: 500_000, EndUs: 500_000},
	}
	if len(detail.Published.Events) != len(wantEvents) {
		t.Fatalf("events: %+v", detail.Published.Events)
	}
	for i, e := range detail.Published.Events {
		confidence := e.Confidence
		e.Confidence = 0
		if e != wantEvents[i] || math.Abs(confidence-0.9) > 1e-6 {
			t.Errorf("event %d: %+v confidence %v want %+v", i, e, confidence, wantEvents[i])
		}
	}
	if n := id(t, f.pool, "SELECT count(*)::text FROM misko.analysis_events WHERE run_id = $1 AND test_subject_id = $2", run.ID, run.TestSubjectID); n != "2" {
		t.Fatalf("events belong to the analysed animal: %s", n)
	}
	links, err := f.s.VideoPair(ctx, viewer, run.ID)
	if err != nil || !strings.Contains(links.OriginalURL, "tests/source-a?generation=42") || !strings.Contains(links.AnalyzedURL, "overlay.mp4?generation=") {
		t.Fatalf("video pair: %+v %v", links, err)
	}

	for _, tc := range []struct{ name, sql, constraint string }{
		{"edit metric", "UPDATE misko.metric_results SET value = 1 WHERE run_id = $1", ""},
		{"delete pair", "DELETE FROM misko.analysis_video_pairs WHERE run_id = $1", ""},
		{"event after success", "INSERT INTO misko.analysis_events (run_id, attempt, test_subject_id, event_type, kind, start_us, end_us, confidence) SELECT id, attempt, test_subject_id, 'in_platform', 'INTERVAL', 0, 1, 1 FROM misko.analysis_runs WHERE id = $1", "analysis_outputs_fenced"},
		{"reopen run", "UPDATE misko.analysis_runs SET status = 'QUEUED', worker_id = NULL, finished_at = NULL, lease_expires_at = NULL WHERE id = $1", "analysis_runs_transition"},
		{"move run to other source", "UPDATE misko.analysis_runs SET source_generation = 43 WHERE id = $1", "analysis_runs_transition"},
		{"delete result", "DELETE FROM misko.subject_results WHERE run_id = $1", "subject_results_immutable"},
	} {
		assertRejected(t, tc.name, f.pool, tc.constraint, tc.sql, run.ID)
	}

	// A run cannot be marked SUCCEEDED without publishing its pair in the same transaction.
	pending, err := f.s.Reanalyze(ctx, technician, run.TestSubjectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Claim(ctx, other); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, "succeed without pair", f.pool, "analysis_runs_published",
		"UPDATE misko.analysis_runs SET status = 'SUCCEEDED', model_version = 'x', finished_at = now(), lease_expires_at = NULL WHERE id = $1", pending.ID)
	assertRejected(t, "skip the lease", f.pool, "analysis_runs_transition",
		"UPDATE misko.analysis_runs SET attempt = attempt + 1 WHERE id = $1", pending.ID)
	assertRejected(t, "a result pointing to an unfinished run", f.pool, "subject_results_succeeded",
		"UPDATE misko.subject_results SET run_id = $1 WHERE test_subject_id = $2", pending.ID, run.TestSubjectID)

	if _, err := f.s.DisableWorker(ctx, manager, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AuthenticateWorker(ctx, tokenA); !errors.Is(err, application.ErrWorkerUnauthenticated) {
		t.Fatalf("disabled worker token: %v", err)
	}
}

// The scheduler only reads animals it can plan, so 520 older animals waiting
// for a calibration never fill its batch, and an animal whose stored inputs
// cannot be read is skipped without stopping the others.
func TestStarvedCandidatesAndBadRowsNeverBlockTheTick(t *testing.T) {
	f := newFixture(t)
	f.worker("tracker")
	var script strings.Builder
	script.WriteString("DO $$ DECLARE t uuid; a uuid; BEGIN FOR i IN 1..520 LOOP\n")
	fmt.Fprintf(&script, `INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
		VALUES ('%[1]s', '%[2]s', '%[3]s', 1, 'MWM', 1, 1, now(), '%[4]s') RETURNING id INTO t;
	INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
		VALUES ('%[1]s', t, '%[5]s', '%[6]s', 'MWM', 1, '%[4]s');
	UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() WHERE id = t;
	INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ORIGINAL', 'misko-test-videos', 'old/' || i, 'video/mp4', 4096, 1, 'VERIFIED', 7, now(), '%[4]s') RETURNING id INTO a;
	INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, created_by, created_at) VALUES ('%[1]s', t, a, '%[4]s', now() - interval '30 days');
	`, f.exp, f.timepoint, f.version, manager.UserID, f.allocC, f.arenaC)
	script.WriteString("END LOOP; END $$")
	if _, err := f.pool.Exec(ctx, script.String()); err != nil {
		t.Fatal(err)
	}
	// An arena whose parameters are not numbers, calibrated, in an older session.
	broken := f.arena("Broken box", `{"tank_diameter_cm": "wide", "platform_diameter_cm": 10}`)
	id(t, f.pool, calibrationSQL, broken, nil, nil, nil, 0, nil, "VALID", nil, technician.UserID)
	brokenTest := f.newTest()
	brokenSubject := f.join(brokenTest, f.allocate("M4"), broken)
	f.start(brokenTest, "tests/broken", "now() - interval '40 days'")

	report, err := f.s.Tick(ctx)
	if err != nil || report.Enqueued != 2 || len(report.Skipped) != 1 || !strings.Contains(report.Skipped[0].Error(), brokenSubject) {
		t.Fatalf("tick: enqueued %d skipped %v err %v", report.Enqueued, report.Skipped, err)
	}
	if len(f.runsOf(f.a)) != 1 || len(f.runsOf(f.b)) != 1 {
		t.Fatalf("the calibrated animals were starved")
	}
	if _, err := f.s.Reanalyze(ctx, technician, brokenSubject); !errors.Is(err, application.ErrNotReady) {
		t.Fatalf("manual run of a broken row: %v", err)
	}
}

func TestReanalysisReviewAndBlinding(t *testing.T) {
	f := newFixture(t)
	w, _ := f.worker("tracker")
	for name, tc := range map[string]struct {
		subject string
		want    error
	}{
		"uncalibrated arena": {f.c, application.ErrNotReady},
		"unknown animal":     {"01a00000-0000-7000-8000-0000000000ff", application.ErrTestSubjectNotFound},
		"malformed id":       {"M1", application.ErrTestSubjectNotFound},
	} {
		if _, err := f.s.Reanalyze(ctx, technician, tc.subject); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// Worker failures: a retryable crash queues attempt 2, then the attempts run out.
	manual, err := f.s.Reanalyze(ctx, technician, f.a)
	if err != nil || manual.Trigger != domain.Manual || manual.CreatedBy != technician.UserID || manual.TestSubjectID != f.a || manual.CalibrationID != f.calA {
		t.Fatalf("manual run: %+v %v", manual, err)
	}
	job, _ := f.s.Claim(ctx, w)
	if retry, err := f.s.Fail(ctx, w, job.Run.ID, 1, "decoder crashed", true); err != nil || retry.Status != domain.Queued {
		t.Fatalf("retryable failure: %+v %v", retry, err)
	}
	job, _ = f.s.Claim(ctx, w)
	if failed, err := f.s.Fail(ctx, w, job.Run.ID, 2, "decoder crashed", true); err != nil || failed.Status != domain.Failed || job.Run.Attempt != 2 {
		t.Fatalf("attempts exhausted: %+v %v", failed, err)
	}

	// A trajectory that loses half of its samples fails QC: the run fails, nothing is published and no result opens.
	if _, err := f.s.Reanalyze(ctx, technician, f.a); err != nil {
		t.Fatal(err)
	}
	job, _ = f.s.Claim(ctx, w)
	lossy := f.trajectory(job.Run, 10, 2)
	f.requestOutputs(w, job.Run, lossy)
	f.store(job.Run, lossy)
	failed, err := f.s.Submit(ctx, w, job.Run.ID, f.submission(job.Run))
	if err != nil || failed.Status != domain.Failed || !strings.HasPrefix(failed.FailureReason, "QC_FAILED: max_lost_frame_ratio") ||
		f.count("metric_results", job.Run.ID) != 0 || f.count("analysis_video_pairs", job.Run.ID) != 0 {
		t.Fatalf("QC failure: %+v %v", failed, err)
	}
	if status, _ := f.result(f.a); status != "" {
		t.Fatalf("a failed run opened a result: %s", status)
	}

	succeed := func(distance int) domain.Run {
		t.Helper()
		if _, err := f.s.Reanalyze(ctx, technician, f.a); err != nil {
			t.Fatal(err)
		}
		job, err := f.s.Claim(ctx, w)
		if err != nil {
			t.Fatal(err)
		}
		run := job.Run
		shifted := f.submission(run)
		shifted.Pair.SourceOffsetUs = 0
		if _, err := f.s.Submit(ctx, w, run.ID, shifted); !errors.Is(err, domain.ErrInvalidPair) {
			t.Errorf("pair without the clip offset: %v", err)
		}
		trajectory := f.trajectory(run, distance, 0)
		f.requestOutputs(w, run, trajectory)
		f.store(run, trajectory)
		done, err := f.s.Submit(ctx, w, run.ID, f.submission(run))
		if err != nil {
			t.Fatal(err)
		}
		return done
	}
	first := succeed(10)
	second := succeed(20)
	if status, run := f.result(f.a); status != "PENDING" || run != first.ID {
		t.Fatalf("a newer run never replaces the result under review: %s %s", status, run)
	}

	// The reviewer sees the animal, its arena, the run under review with its
	// metrics, and that a newer run exists.
	page, err := f.s.Results(ctx, viewer, application.ReviewQuery{ExperimentID: f.exp})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("pending results: %+v %v", page, err)
	}
	item := page.Items[0]
	if item.SubjectCode != "M1" || item.EnvironmentName != "Box A" || item.TimepointName != "Day 7" || item.Run.ID != first.ID || !item.NewerRunAvailable() ||
		item.LatestRunID != second.ID || len(item.Metrics) != 8 || item.Group == nil || item.Group.Code != "G1" || item.Blinded {
		t.Fatalf("review item: %+v", item)
	}
	if _, err := f.s.Accept(ctx, viewer, f.a, ""); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("viewer accepts: %v", err)
	}
	accepted, err := f.s.Accept(ctx, technician, f.a, "clean")
	if err != nil || accepted.Result.Status != domain.ResultAccepted || accepted.Result.ReviewedBy != technician.UserID || accepted.Result.Note != "clean" {
		t.Fatalf("accept: %+v %v", accepted, err)
	}
	if page, _ := f.s.Results(ctx, viewer, application.ReviewQuery{ExperimentID: f.exp}); page.Total != 0 {
		t.Fatalf("accepted results leave the pending list: %+v", page)
	}
	chosen, err := f.s.ChooseRun(ctx, technician, f.a, second.ID)
	if err != nil || chosen.Result.Status != domain.ResultPending || chosen.Run.ID != second.ID || chosen.NewerRunAvailable() || chosen.Result.ReviewedBy != "" {
		t.Fatalf("choose newer run: %+v %v", chosen, err)
	}
	queued, err := f.s.Reanalyze(ctx, technician, f.a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ChooseRun(ctx, technician, f.a, queued.ID); !errors.Is(err, domain.ErrRunNotChosen) {
		t.Fatalf("choosing an unfinished run: %v", err)
	}
	job, _ = f.s.Claim(ctx, w)
	if _, err := f.s.Fail(ctx, w, job.Run.ID, 1, "stopped", false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Reject(ctx, technician, f.a, "", ""); !errors.Is(err, domain.ErrInvalidRejection) {
		t.Fatalf("reject without reason: %v", err)
	}
	rejected, err := f.s.Reject(ctx, technician, f.a, "TRACKING_LOST", "tail tracked")
	if err != nil || rejected.Result.Status != domain.ResultRejected || rejected.Result.Reason != "TRACKING_LOST" {
		t.Fatalf("reject: %+v %v", rejected, err)
	}
	if _, err := f.s.Accept(ctx, technician, f.b, ""); !errors.Is(err, application.ErrResultNotFound) {
		t.Fatalf("an animal without a result: %v", err)
	}
	detail, err := f.s.Result(ctx, viewer, f.a)
	if err != nil || len(detail.Runs) != 2 || detail.Item.Result.Status != domain.ResultRejected {
		t.Fatalf("result detail lists the succeeded runs: %+v %v", detail, err)
	}

	// Blinding hides the group from every review read.
	if _, err := f.pool.Exec(ctx, "UPDATE misko.experiments SET blinded = true WHERE id = $1", f.exp); err != nil {
		t.Fatal(err)
	}
	all, err := f.s.Results(ctx, viewer, application.ReviewQuery{Status: "ALL"})
	if err != nil || len(all.Items) != 1 || all.Items[0].Group != nil || !all.Items[0].Blinded {
		t.Fatalf("blinded list: %+v %v", all, err)
	}
	bulk, err := f.s.AcceptMany(ctx, technician, []string{f.a}, "")
	if err != nil || len(bulk) != 1 || bulk[0].Group != nil || bulk[0].Result.Status != domain.ResultAccepted {
		t.Fatalf("blinded bulk accept: %+v %v", bulk, err)
	}

	// An excluded animal is never analysed again.
	if _, err := f.pool.Exec(ctx, "UPDATE misko.allocations SET excluded_at = now(), exclusion_reason = 'DIED' WHERE id = $1", f.allocB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Reanalyze(ctx, technician, f.b); !errors.Is(err, application.ErrNotReady) {
		t.Fatalf("excluded animal: %v", err)
	}
	// The tick still schedules A, which only had manual runs, but never B.
	if report, err := f.s.Tick(ctx); err != nil || report.Enqueued != 1 || len(f.runsOf(f.b)) != 0 {
		t.Fatalf("tick after exclusion: %+v %v", report, err)
	}

	// Completing the experiment freezes the reviewed results.
	for _, sql := range []string{
		"UPDATE misko.tests SET status = 'COMPLETED', completed_at = now() WHERE experiment_id = $1",
		"UPDATE misko.experiments SET status = 'COMPLETED', completed_at = now() WHERE id = $1",
	} {
		if _, err := f.pool.Exec(ctx, sql, f.exp); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.Reject(ctx, technician, f.a, "OTHER", ""); !errors.Is(err, application.ErrExperimentClosed) {
		t.Fatalf("review after completion: %v", err)
	}
}

// A default calibration of an arena lets its animals be analysed, correcting
// it never reanalyses what already ran, and an override of one animal does.
func TestArenaCalibrationSchedulesOnceAndOverridesReanalyse(t *testing.T) {
	f := newFixture(t)
	tracker, _ := f.worker("tracker-a")
	if report, err := f.s.Tick(ctx); err != nil || report.Enqueued != 2 {
		t.Fatalf("the two calibrated arenas are ready: %+v %v", report, err)
	}
	calC := id(t, f.pool, calibrationSQL, f.arenaC, nil, nil, nil, 1280, nil, "VALID", nil, technician.UserID)
	if report, err := f.s.Tick(ctx); err != nil || report.Enqueued != 1 || f.runsOf(f.c)[0].CalibrationID != calC {
		t.Fatalf("the new default calibrates the third arena: %+v %v", report, err)
	}
	corrected := id(t, f.pool, calibrationSQL, f.arenaA, nil, nil, nil, 0, f.calA, "VALID", nil, technician.UserID)
	if report, err := f.s.Tick(ctx); err != nil || report.Enqueued != 0 {
		t.Fatalf("correcting a default must not reanalyse: %+v %v", report, err)
	}
	// An animal with a queued run cannot be analysed again until that run ends.
	if _, err := f.s.Reanalyze(ctx, technician, f.a); !errors.Is(err, application.ErrRunPending) {
		t.Fatalf("reanalysis while a run is queued: %v", err)
	}
	for range 3 {
		job, err := f.s.Claim(ctx, tracker)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.Fail(ctx, tracker, job.Run.ID, 1, "stopped", false); err != nil {
			t.Fatal(err)
		}
	}
	// Reanalysis stays an explicit act and pins the corrected default.
	manual, err := f.s.Reanalyze(ctx, technician, f.a)
	if err != nil || manual.CalibrationID != corrected {
		t.Fatalf("manual reanalysis: %+v %v", manual, err)
	}

	// The camera moved for animal B: an override of B for this video schedules B again.
	override := id(t, f.pool, calibrationSQL, f.arenaB, f.b, f.recording, 0, 700, nil, "VALID", nil, technician.UserID)
	if report, err := f.s.Tick(ctx); err != nil || report.Enqueued != 1 || f.runsOf(f.b)[0].CalibrationID != override {
		t.Fatalf("an override reanalyses its animal: %+v %v", report, err)
	}
	// A rejected correction of the override leaves B waiting.
	id(t, f.pool, calibrationSQL, f.arenaB, f.b, f.recording, 0, 700, override, "REJECTED", "EXCESSIVE_CHECK_ERROR", technician.UserID)
	if _, err := f.s.Reanalyze(ctx, technician, f.b); !errors.Is(err, application.ErrNotReady) {
		t.Fatalf("an animal whose override was rejected: %v", err)
	}
	assertRejected(t, "an override of another arena", f.pool, "calibrations_scope", calibrationSQL, f.arenaA, f.b, f.recording, 0, 0, nil, "VALID", nil, technician.UserID)
	assertRejected(t, "a run pinning the calibration of another arena", f.pool, "analysis_runs_calibration_scope",
		`INSERT INTO misko.analysis_runs (experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
			calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, parameters, trigger, status, max_attempts, available_at)
		SELECT experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
			$2::uuid, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, parameters, 'MANUAL', 'QUEUED', max_attempts, available_at
		FROM misko.analysis_runs WHERE id = $1`, manual.ID, f.calB)
}

func TestReanalysisIsRefusedWhileARunIsPending(t *testing.T) {
	f := newFixture(t)
	w, _ := f.worker("tracker")
	first, err := f.s.Reanalyze(ctx, technician, f.a)
	if err != nil {
		t.Fatal(err)
	}
	// Queued: a repeated request is refused and adds no run.
	if _, err := f.s.Reanalyze(ctx, technician, f.a); !errors.Is(err, application.ErrRunPending) {
		t.Fatalf("second request while queued: %v", err)
	}
	// Running is pending too.
	job, err := f.s.Claim(ctx, w)
	if err != nil || job.Run.ID != first.ID {
		t.Fatalf("claim: %+v %v", job, err)
	}
	if _, err := f.s.Reanalyze(ctx, technician, f.a); !errors.Is(err, application.ErrRunPending) {
		t.Fatalf("request while running: %v", err)
	}
	// Another animal of the same video is not blocked by this one.
	if _, err := f.s.Reanalyze(ctx, technician, f.b); err != nil {
		t.Fatalf("a pending run of one animal blocked another: %v", err)
	}
	// Once the run has ended a new request is accepted again.
	if _, err := f.s.Fail(ctx, w, first.ID, 1, "stopped", false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Reanalyze(ctx, technician, f.a); err != nil {
		t.Fatalf("request after the run ended: %v", err)
	}

	// Concurrent requests create at most one run.
	f2 := newFixture(t)
	f2.worker("tracker")
	var wg sync.WaitGroup
	var accepted, refused atomic.Int32
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch _, err := f2.s.Reanalyze(ctx, technician, f2.a); {
			case err == nil:
				accepted.Add(1)
			case errors.Is(err, application.ErrRunPending):
				refused.Add(1)
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 || refused.Load() != 7 {
		t.Fatalf("accepted %d refused %d, want 1 and 7", accepted.Load(), refused.Load())
	}
}
