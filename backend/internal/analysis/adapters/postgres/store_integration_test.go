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

type fixture struct {
	t                                        *testing.T
	pool                                     *pgxpool.Pool
	clock                                    *clock
	objects                                  *objects
	s                                        *application.Service
	test, otherTest, trial, calibrated, bare string
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

// newFixture builds, with SQL only, an OPEN_FIELD test in progress with one trial,
// a calibrated verified recording (clip 1 s to 9 s) and a verified recording
// without calibration, plus a second test.
func newFixture(t *testing.T) *fixture {
	pool := pgtest.Database(t)
	f := &fixture{t: t, pool: pool, clock: &clock{now: time.Now().UTC().Truncate(time.Second)}, objects: &objects{stored: map[string]application.ObjectAttrs{}, contents: map[string][]byte{}}}
	f.s = application.New(postgres.NewStore(pool), f.objects, catalog.New(), token.New(),
		application.Settings{Lease: time.Minute, MaxAttempts: 2, UploadTTL: time.Hour, ReadTTL: time.Minute, MaxOutputBytes: 1 << 30, MaxTrajectoryBytes: 1 << 20}, f.clock.Now)
	exp := id(t, pool, "INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id")
	subject := id(t, pool, "INSERT INTO misko.subjects (code, species, sex) VALUES ('S1', 'MOUSE', 'MALE') RETURNING id")
	enrollment := id(t, pool, "INSERT INTO misko.enrollments (experiment_id, subject_id, enrolled_at) VALUES ($1, $2, now() - interval '3 days') RETURNING id", exp, subject)
	env := id(t, pool, "INSERT INTO misko.environments (name, paradigm_key) VALUES ('Arena', 'OPEN_FIELD') RETURNING id")
	rev := id(t, pool, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, created_by)
		VALUES ($1, 'OPEN_FIELD', 1, 1, '{"arena_width_cm": 50, "arena_height_cm": 50, "center_fraction": 0.5}', $2) RETURNING id`, env, manager.UserID)
	protocol := id(t, pool, "INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id", exp)
	var version string
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		version = id(t, tx, "INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by) VALUES ($1, $2, 1, 1, $3) RETURNING id", exp, protocol, manager.UserID)
		_, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, environment_revision_id, trial_type, trials, inter_trial_interval_s, session)
			VALUES ($1, 1, 'OPEN_FIELD', 1, $2, 'STANDARD', 1, 0, '{"max_sample_gap_s": 0.25}')`, version, rev)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	newTest := func() string {
		return id(t, pool, `INSERT INTO misko.tests (experiment_id, enrollment_id, subject_id, protocol_version_id, step_position, paradigm_key, paradigm_version, environment_revision_id, planned_trials, scheduled_at, created_by)
			VALUES ($1, $2, $3, $4, 1, 'OPEN_FIELD', 1, $5, 1, now() - interval '2 days', $6) RETURNING id`, exp, enrollment, subject, version, rev, manager.UserID)
	}
	f.test, f.otherTest = newTest(), newTest()
	for _, test := range []string{f.test, f.otherTest} {
		if _, err := pool.Exec(ctx, "UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() - interval '1 day' WHERE id = $1", test); err != nil {
			t.Fatal(err)
		}
	}
	f.trial = id(t, pool, "INSERT INTO misko.trials (test_id, number, repetition, attempt, started_at, recorded_by) VALUES ($1, 1, 1, 1, now() - interval '20 hours', $2) RETURNING id", f.test, technician.UserID)
	id(t, pool, "INSERT INTO misko.trials (test_id, number, repetition, attempt, started_at, recorded_by) VALUES ($1, 1, 1, 1, now() - interval '20 hours', $2) RETURNING id", f.otherTest, technician.UserID)
	recording := func(object string) string {
		asset := id(t, pool, `INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
			VALUES ('ORIGINAL', 'misko-test-videos', $1, 'video/mp4', 4096, 1, 'VERIFIED', 42, now(), $2) RETURNING id`, object, technician.UserID)
		return id(t, pool, "INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, clip_start_us, clip_end_us, created_by) VALUES ($1, $2, $3, 1000000, 9000000, $4) RETURNING id",
			exp, f.test, asset, technician.UserID)
	}
	f.calibrated, f.bare = recording("tests/source-a"), recording("tests/source-b")
	id(t, pool, `INSERT INTO misko.calibrations (recording_id, environment_revision_id, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height, reference_frame_us,
		measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, created_by)
		VALUES ($1, (SELECT t.environment_revision_id FROM misko.test_recordings r JOIN misko.tests t ON t.id = r.test_id WHERE r.id = $1), 'cam', 1920, 1080, 0, 0, 1920, 1080, 0, 'ARENA_FLOOR', '[{},{},{},{}]', '[{},{},{}]', '{1,0,0,0,1,0,0,0,1}', 0, 0, 0, 2, 'test', 'VALID', $2) RETURNING id`,
		f.calibrated, technician.UserID)
	return f
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
	registered, err := f.s.RegisterWorker(ctx, manager, application.WorkerInput{Name: name, ModelVersion: "tracker 1.0", Capabilities: []domain.Capability{{ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1}}})
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
// distance centimeters (entering the 12.5 to 37.5 cm center at x = 13), then
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
	runs, err := f.s.Runs(ctx, viewer, f.test)
	if err != nil || len(runs) != 1 || runs[0].RecordingID != f.calibrated || runs[0].Trigger != domain.Automatic || runs[0].CalibrationID == "" ||
		runs[0].SourceGeneration != 42 || runs[0].Parameters["arena_width_cm"] != 50 || runs[0].Parameters["max_sample_gap_s"] != 0.25 {
		t.Fatalf("concurrent schedulers must create one job for the calibrated recording: %+v %v", runs, err)
	}
	if report, _ := f.s.Tick(ctx); report.Enqueued != 0 {
		t.Fatalf("the same inputs were enqueued again: %+v", report)
	}
	assertRejected(t, "second automatic run for the same inputs", f.pool, "analysis_runs_automatic_key",
		`INSERT INTO misko.analysis_runs (experiment_id, test_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
			calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, environment_revision_id, protocol_version_id,
			parameters, trigger, status, max_attempts, available_at)
		SELECT experiment_id, test_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
			calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, environment_revision_id, protocol_version_id,
			parameters, trigger, 'QUEUED', max_attempts, available_at FROM misko.analysis_runs WHERE id = $1`, runs[0].ID)

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
	if len(claimed) != 1 || claimed[0].Run.Attempt != 1 || len(claimed[0].Contract.Metrics) != 9 {
		t.Fatalf("exactly one worker claims the run: %+v", claimed)
	}
	first, holder := claimed[0].Run, owners[0]
	other := a
	if holder.ID == a.ID {
		other = b
	}
	if _, err := f.s.Heartbeat(ctx, other, first.ID, 1); !errors.Is(err, domain.ErrStaleAttempt) {
		t.Fatalf("heartbeat by another worker: %v", err)
	}

	// The holder crashes: its lease expires and the other worker takes attempt 2.
	f.clock.advance(2 * time.Minute)
	if report, err := f.s.Tick(ctx); err != nil || report.Requeued != 1 {
		t.Fatalf("expired lease: %+v %v", report, err)
	}
	job, err := f.s.Claim(ctx, other)
	if err != nil || job.Run.ID != first.ID || job.Run.Attempt != 2 {
		t.Fatalf("reclaim: %+v %v", job, err)
	}
	run := job.Run
	if _, _, err := f.s.RequestOutput(ctx, holder, run.ID, application.OutputRequest{Attempt: 1, Kind: domain.AnalyzedVideo, FileName: "late.mp4", ContentType: "video/mp4", SizeBytes: 1, CRC32C: 1}); !errors.Is(err, domain.ErrStaleAttempt) {
		t.Fatalf("late attempt requests output: %v", err)
	}
	if _, err := f.s.Submit(ctx, holder, run.ID, f.submission(first)); !errors.Is(err, domain.ErrStaleAttempt) {
		t.Fatalf("late attempt submits: %v", err)
	}
	if calibration := job.Calibration; calibration == nil || calibration.ID != run.CalibrationID || calibration.FrameWidth != 1920 || calibration.Transform != [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1} {
		t.Fatalf("the job must carry the pinned calibration: %+v", job.Calibration)
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

	// Duplicate delivery of the accepted attempt publishes nothing new.
	again, err := f.s.Submit(ctx, other, run.ID, f.submission(run))
	if err != nil || again.Status != domain.Succeeded {
		t.Fatalf("duplicate delivery: %+v %v", again, err)
	}
	detail, err := f.s.RunDetail(ctx, viewer, run.ID)
	if err != nil || detail.Published == nil || len(detail.Published.Metrics) != 9 || len(detail.Published.Artifacts) != 2 ||
		detail.Published.Pair == nil || detail.Published.Pair.SourceGeneration != 42 || detail.Published.Pair.SourceOffsetUs != 1_000_000 {
		t.Fatalf("published result: %+v %v", detail.Published, err)
	}
	// The Go engine computed these from the stored samples: 25 counted 0.1 s
	// intervals, entering the center at 0.8 s and standing still from 1.0 s.
	metrics := map[string]domain.MetricValue{}
	for _, m := range detail.Published.Metrics {
		metrics[m.Key] = m
	}
	for key, want := range map[string]float64{"distance_cm": 10, "duration_s": 2.5, "center_time_s": 1.7, "center_entries_count": 1, "immobility_s": 1.5} {
		if m := metrics[key]; m.Value == nil || math.Abs(*m.Value-want) > 1e-9 {
			t.Errorf("%s: %+v want %v", key, m, want)
		}
	}
	wantEvents := []domain.Event{
		{Type: "center_entry", Kind: "POINT", StartUs: 800_000, EndUs: 800_000},
		{Type: "in_center", Kind: "INTERVAL", StartUs: 800_000, EndUs: 2_500_000},
		{Type: "immobile", Kind: "INTERVAL", StartUs: 1_000_000, EndUs: 2_500_000},
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
	links, err := f.s.VideoPair(ctx, viewer, run.ID)
	if err != nil || !strings.Contains(links.OriginalURL, "tests/source-a?generation=42") || !strings.Contains(links.AnalyzedURL, "overlay.mp4?generation=") {
		t.Fatalf("video pair: %+v %v", links, err)
	}

	for _, tc := range []struct{ name, sql, constraint string }{
		{"edit metric", "UPDATE misko.metric_results SET value = 1 WHERE run_id = $1", ""},
		{"delete pair", "DELETE FROM misko.analysis_video_pairs WHERE run_id = $1", ""},
		{"event after success", "INSERT INTO misko.analysis_events (run_id, attempt, test_id, event_type, kind, start_us, end_us, confidence) SELECT id, attempt, test_id, 'immobile', 'INTERVAL', 0, 1, 1 FROM misko.analysis_runs WHERE id = $1", "analysis_outputs_fenced"},
		{"reopen run", "UPDATE misko.analysis_runs SET status = 'QUEUED', worker_id = NULL, finished_at = NULL, lease_expires_at = NULL WHERE id = $1", "analysis_runs_transition"},
		{"move run to other source", "UPDATE misko.analysis_runs SET source_generation = 43 WHERE id = $1", "analysis_runs_transition"},
	} {
		assertRejected(t, tc.name, f.pool, tc.constraint, tc.sql, run.ID)
	}

	// A run cannot be marked SUCCEEDED without publishing its pair in the same transaction.
	pending, err := f.s.Reanalyze(ctx, technician, f.test, f.calibrated)
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

	if _, err := f.s.DisableWorker(ctx, manager, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AuthenticateWorker(ctx, tokenA); !errors.Is(err, application.ErrWorkerUnauthenticated) {
		t.Fatalf("disabled worker token: %v", err)
	}
}

func TestReanalysisFailuresAndValidation(t *testing.T) {
	f := newFixture(t)
	w, _ := f.worker("tracker")
	if _, err := f.s.Reanalyze(ctx, technician, f.test, f.bare); !errors.Is(err, application.ErrNotReady) {
		t.Fatalf("reanalyze without calibration: %v", err)
	}
	if _, err := f.s.Reanalyze(ctx, technician, f.otherTest, f.calibrated); !errors.Is(err, application.ErrRecordingNotFound) {
		t.Fatalf("recording through another test: %v", err)
	}

	// Worker failures: a retryable crash queues attempt 2, then the attempts run out.
	manual, err := f.s.Reanalyze(ctx, technician, f.test, f.calibrated)
	if err != nil || manual.Trigger != domain.Manual || manual.CreatedBy != technician.UserID {
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

	// A trajectory that loses half of its samples fails QC: the run fails and nothing is published.
	if _, err := f.s.Reanalyze(ctx, technician, f.test, f.calibrated); err != nil {
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
	var stored string
	if err := f.pool.QueryRow(ctx, "SELECT status || ' ' || failure_reason FROM misko.analysis_runs WHERE id = $1", job.Run.ID).Scan(&stored); err != nil || !strings.HasPrefix(stored, "FAILED QC_FAILED") {
		t.Fatalf("stored QC failure: %q %v", stored, err)
	}

	succeed := func(distance int) domain.Run {
		t.Helper()
		if _, err := f.s.Reanalyze(ctx, technician, f.test, f.calibrated); err != nil {
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
	distance := func(runID string) float64 {
		detail, err := f.s.RunDetail(ctx, viewer, runID)
		if err != nil || detail.Published == nil {
			t.Fatalf("detail: %+v %v", detail, err)
		}
		for _, m := range detail.Published.Metrics {
			if m.Key == "distance_cm" {
				return *m.Value
			}
		}
		return -1
	}
	if distance(first.ID) != 10 || distance(second.ID) != 20 {
		t.Fatalf("reanalysis must keep earlier results: %v %v", distance(first.ID), distance(second.ID))
	}
	a, _ := f.s.VideoPair(ctx, viewer, first.ID)
	b, _ := f.s.VideoPair(ctx, viewer, second.ID)
	if a.Pair.AnalyzedAssetID == b.Pair.AnalyzedAssetID || a.Pair.SourceAssetID != b.Pair.SourceAssetID {
		t.Fatalf("each run pairs its own output with the same original: %+v %+v", a.Pair, b.Pair)
	}
	runs, _ := f.s.Runs(ctx, viewer, f.test)
	if len(runs) != 4 {
		t.Fatalf("runs: %d", len(runs))
	}
}

// A default calibration of the environment lets recordings without one of their
// own be analyzed, and correcting it never reanalyzes what already ran.
func TestEnvironmentCalibrationSchedulesOnceAndNeverReanalyzes(t *testing.T) {
	f := newFixture(t)
	tracker, _ := f.worker("tracker-a")
	if report, err := f.s.Tick(ctx); err != nil || report.Enqueued != 1 {
		t.Fatalf("only the recording with its own calibration is ready: %+v %v", report, err)
	}
	revision := id(t, f.pool, "SELECT environment_revision_id FROM misko.tests WHERE id = $1", f.test)
	insert := `INSERT INTO misko.calibrations (recording_id, environment_revision_id, supersedes_id, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height,
		measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, created_by)
		VALUES (NULL, $1, $2, 'cam', 1920, 1080, 0, 0, 1920, 1080, 'ARENA_FLOOR', '[{},{},{},{}]', '[{},{},{}]', '{1,0,0,0,1,0,0,0,1}', 0, 0, 0, 2, 'test', $3, $4) RETURNING id`
	def := id(t, f.pool, insert, revision, nil, "VALID", technician.UserID)
	if report, err := f.s.Tick(ctx); err != nil || report.Enqueued != 1 {
		t.Fatalf("the default calibrates the other recording: %+v %v", report, err)
	}
	runs, err := f.s.Runs(ctx, viewer, f.test)
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs: %+v %v", runs, err)
	}
	for _, run := range runs {
		switch {
		case run.RecordingID == f.bare && run.CalibrationID != def:
			t.Fatalf("the bare recording pins the environment calibration: %+v", run)
		case run.RecordingID == f.calibrated && (run.CalibrationID == "" || run.CalibrationID == def):
			t.Fatalf("a recording with its own calibration keeps it: %+v", run)
		}
	}

	corrected := id(t, f.pool, insert, revision, def, "VALID", technician.UserID)
	if report, err := f.s.Tick(ctx); err != nil || report.Enqueued != 0 {
		t.Fatalf("correcting the default must not reanalyze: %+v %v", report, err)
	}
	// A recording with a queued run cannot be analyzed again until that run ends.
	if _, err := f.s.Reanalyze(ctx, technician, f.test, f.bare); !errors.Is(err, application.ErrRunPending) {
		t.Fatalf("reanalysis while a run is queued: %v", err)
	}
	for range runs {
		job, err := f.s.Claim(ctx, tracker)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.Fail(ctx, tracker, job.Run.ID, 1, "stopped", false); err != nil {
			t.Fatal(err)
		}
	}
	// Reanalysis stays an explicit act and pins the corrected default.
	manual, err := f.s.Reanalyze(ctx, technician, f.test, f.bare)
	if err != nil || manual.CalibrationID != corrected {
		t.Fatalf("manual reanalysis: %+v %v", manual, err)
	}
	ownOfCalibrated := id(t, f.pool, "SELECT id FROM misko.calibrations WHERE recording_id = $1", f.calibrated)
	assertRejected(t, "a run pinning the calibration of another recording", f.pool, "analysis_runs_calibration_scope",
		`INSERT INTO misko.analysis_runs (experiment_id, test_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, clip_end_us,
			calibration_id, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, environment_revision_id, protocol_version_id,
			parameters, trigger, status, max_attempts, available_at)
		SELECT experiment_id, test_id, recording_id, source_asset_id, source_generation + 1, source_crc32c, clip_start_us, clip_end_us,
			$2::uuid, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, environment_revision_id, protocol_version_id,
			parameters, 'MANUAL', 'QUEUED', max_attempts, available_at FROM misko.analysis_runs WHERE id = $1`, manual.ID, ownOfCalibrated)
}

func TestReanalysisIsRefusedWhileARunIsPending(t *testing.T) {
	f := newFixture(t)
	w, _ := f.worker("tracker")
	first, err := f.s.Reanalyze(ctx, technician, f.test, f.calibrated)
	if err != nil {
		t.Fatal(err)
	}
	// Queued: a repeated request is refused and adds no run.
	if _, err := f.s.Reanalyze(ctx, technician, f.test, f.calibrated); !errors.Is(err, application.ErrRunPending) {
		t.Fatalf("second request while queued: %v", err)
	}
	// Running is pending too.
	job, err := f.s.Claim(ctx, w)
	if err != nil || job.Run.ID != first.ID {
		t.Fatalf("claim: %+v %v", job, err)
	}
	if _, err := f.s.Reanalyze(ctx, technician, f.test, f.calibrated); !errors.Is(err, application.ErrRunPending) {
		t.Fatalf("request while running: %v", err)
	}
	// Another recording is not blocked by this one.
	if _, err := f.s.Reanalyze(ctx, technician, f.test, f.bare); errors.Is(err, application.ErrRunPending) {
		t.Fatalf("a pending run of one recording blocked another: %v", err)
	}
	// Once the run has ended a new request is accepted again.
	if _, err := f.s.Fail(ctx, w, first.ID, 1, "stopped", false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Reanalyze(ctx, technician, f.test, f.calibrated); err != nil {
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
			switch _, err := f2.s.Reanalyze(ctx, technician, f2.test, f2.calibrated); {
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
