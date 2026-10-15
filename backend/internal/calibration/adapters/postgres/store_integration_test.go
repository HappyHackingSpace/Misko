//go:build integration

package postgres_test

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/adapters/catalog"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"testing"
	"time"
)

var (
	ctx        = context.Background()
	technician = access.Actor{UserID: "01a00000-0000-7000-8000-000000000002", Role: access.Technician}
	viewer     = access.Actor{UserID: "01a00000-0000-7000-8000-000000000003", Role: access.Viewer}
	// truth maps a 1920x1080 camera to the 50 x 40 cm arena of the fixture.
	truth = domain.Homography{0.03, 0.002, -5, -0.001, 0.045, -3, 0.00001, 0.00002, 1}
)

func point(px, py float64) domain.Correspondence {
	x, y, err := truth.Apply(px, py)
	if err != nil {
		panic(err)
	}
	return domain.Correspondence{PixelX: px, PixelY: py, WorldX: x, WorldY: y}
}

func input(supersedes string) domain.Input {
	return domain.Input{
		CameraID: "cam-top-1", FrameWidth: 1920, FrameHeight: 1080, Crop: domain.Rect{Width: 1920, Height: 1080}, Plane: domain.ArenaFloor,
		Fit:          []domain.Correspondence{point(200, 150), point(1700, 120), point(1750, 1000), point(250, 980)},
		Check:        []domain.Correspondence{point(900, 500), point(400, 700), point(1400, 300)},
		SupersedesID: supersedes,
	}
}

func id(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var out string
	if err := pool.QueryRow(ctx, sql, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// study is a running experiment with a test in progress filming two animals in
// arenas A and B (one camera), and a second test with one animal in arena A,
// created with SQL only because the contexts that own these tables are ported
// separately.
type study struct {
	pool                  *pgxpool.Pool
	experiment            string
	test, otherTest       string
	first, second, other  string // test subjects
	arenaA, arenaB        string // environment revisions
	environmentA, objects string
}

func runningStudy(t *testing.T, pool *pgxpool.Pool) *study {
	t.Helper()
	s := &study{pool: pool}
	s.experiment = id(t, pool, "INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id")
	group := id(t, pool, "INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id", s.experiment)
	timepoint := id(t, pool, "INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Day 1', 1, 1) RETURNING id", s.experiment)
	protocol := id(t, pool, "INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id", s.experiment)
	var version string
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by) VALUES ($1, $2, 1, 1, $3) RETURNING id",
			s.experiment, protocol, technician.UserID).Scan(&version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session) VALUES ($1, 1, 'OPEN_FIELD', 1, 'STANDARD', 1, '{}')", version)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1", s.experiment); err != nil {
		t.Fatal(err)
	}
	arena := func(name string) (string, string) {
		env := id(t, pool, "INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, 'OPEN_FIELD') RETURNING id", name)
		return env, id(t, pool, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
			VALUES ($1, 'OPEN_FIELD', 1, 1, '{"arena_width_cm": 50, "arena_height_cm": 40, "center_fraction": 0.5}', $2) RETURNING id`, env, technician.UserID)
	}
	s.environmentA, s.arenaA = arena("Arena A")
	_, s.arenaB = arena("Arena B")
	plan := func() string {
		return id(t, pool, `INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
			VALUES ($1, $2, $3, 1, 'OPEN_FIELD', 1, 1, now(), $4) RETURNING id`, s.experiment, timepoint, version, technician.UserID)
	}
	join := func(test, code, arena string) string {
		subject := id(t, pool, "INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id", code)
		allocation := id(t, pool, `INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
			VALUES ($1, $2, $3, now() - interval '1 day', $4) RETURNING id`, s.experiment, group, subject, technician.UserID)
		return id(t, pool, `INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
			VALUES ($1, $2, $3, $4, 'OPEN_FIELD', 1, $5) RETURNING id`, s.experiment, test, allocation, arena, technician.UserID)
	}
	s.test, s.otherTest = plan(), plan()
	s.first, s.second, s.other = join(s.test, "M1", s.arenaA), join(s.test, "M2", s.arenaB), join(s.otherTest, "M3", s.arenaA)
	if _, err := pool.Exec(ctx, "UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() WHERE id = ANY($1)", []string{s.test, s.otherTest}); err != nil {
		t.Fatal(err)
	}
	return s
}

// video attaches a new current video to a test, superseding the previous one.
func (s *study) video(t *testing.T, test string, verified bool) string {
	t.Helper()
	s.objects += "x"
	status, generation, verifiedAt := "PENDING", any(nil), any(nil)
	if verified {
		status, generation, verifiedAt = "VERIFIED", int64(42), time.Now()
	}
	asset := id(t, s.pool, `INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ORIGINAL', 'misko-test-videos', $1, 'video/mp4', 1024, 1, $2, $3, $4, $5) RETURNING id`, "videos/"+s.objects, status, generation, verifiedAt, technician.UserID)
	return id(t, s.pool, `INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, supersedes_id, created_by)
		VALUES ($1, $2, $3, (SELECT r.id FROM misko.test_recordings r WHERE r.test_id = $2 AND NOT EXISTS (SELECT 1 FROM misko.test_recordings n WHERE n.supersedes_id = r.id)), $4) RETURNING id`, s.experiment, test, asset, technician.UserID)
}

func TestOverridesGateAnalysis(t *testing.T) {
	pool := pgtest.Database(t)
	s := application.New(postgres.NewStore(pool), catalog.New(), time.Now)
	st := runningStudy(t, pool)
	status := func(test, subject string) application.SubjectStatus {
		t.Helper()
		view, err := s.SubjectStatus(ctx, viewer, test, subject)
		if err != nil {
			t.Fatal(err)
		}
		return view
	}

	if view := status(st.test, st.first); view.Status != application.WaitingForVideo || view.RecordingID != "" {
		t.Fatalf("before any video: %+v", view)
	}
	if _, err := s.Calibrate(ctx, technician, st.test, st.first, input("")); !errors.Is(err, application.ErrNoVideo) {
		t.Fatalf("override without a video: %v", err)
	}
	rec := st.video(t, st.test, true)
	if view := status(st.test, st.first); view.Status != application.Waiting || view.Current != nil || view.RecordingID != rec {
		t.Fatalf("before calibration: %+v", view)
	}
	in := input("")
	in.ReferenceFrameUs = 1_500_000
	first, err := s.Calibrate(ctx, technician, st.test, st.first, in)
	if err != nil || first.Status != domain.Valid || first.ToleranceCm != 2 || first.TestSubjectID != st.first || first.RecordingID != rec ||
		first.EnvironmentRevisionID != st.arenaA || first.ReferenceFrameUs != 1_500_000 || first.FitRMSErrorCm > 1e-6 {
		t.Fatalf("first calibration: %+v %v", first, err)
	}
	if view := status(st.test, st.first); view.Status != application.Calibrated || view.Current.ID != first.ID || view.Source != domain.SourceOverride {
		t.Fatalf("calibrated: %+v", view)
	}
	if view := status(st.test, st.second); view.Status != application.Waiting || view.Source != "" {
		t.Fatalf("the other animal of the video is not calibrated by it: %+v", view)
	}
	for name, err := range map[string]error{
		"status through another test":        second(s.SubjectStatus(ctx, viewer, st.otherTest, st.first)),
		"overrides through another test":     second(s.Overrides(ctx, viewer, st.otherTest, st.first)),
		"malformed test subject":             second(s.SubjectStatus(ctx, viewer, st.test, "subject")),
		"override of an animal of elsewhere": second(s.Calibrate(ctx, technician, st.test, st.other, input(""))),
	} {
		if !errors.Is(err, application.ErrTestSubjectNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}

	for name, tc := range map[string]struct {
		in   domain.Input
		want error
	}{
		"second first calibration": {input(""), domain.ErrStaleCorrection},
		"point off the arena": {func() domain.Input {
			in := input(first.ID)
			in.Check[0].WorldX = 80
			return in
		}(), domain.ErrPointOutsideEnvironment},
		"correction from another camera": {func() domain.Input { in := input(first.ID); in.FrameHeight = 720; in.Crop.Height = 720; return in }(), domain.ErrCameraMismatch},
	} {
		if _, err := s.Calibrate(ctx, technician, st.test, st.first, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}

	off := input(first.ID)
	off.Check[2].WorldY += 3
	rejected, err := s.Calibrate(ctx, technician, st.test, st.first, off)
	if err != nil || rejected.Status != domain.Rejected || rejected.RejectionReason != domain.RejectedCheckError || rejected.SupersedesID != first.ID {
		t.Fatalf("excessive CHECK error: %+v %v", rejected, err)
	}
	if view := status(st.test, st.first); view.Status != application.Waiting || view.Source != domain.SourceOverride {
		t.Fatalf("a rejected correction leaves the animal waiting: %+v", view)
	}

	// Concurrent corrections of the same calibration: exactly one extends the chain.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var created []domain.Calibration
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := s.Calibrate(ctx, technician, st.test, st.first, input(rejected.ID))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				created = append(created, c)
			case !errors.Is(err, domain.ErrStaleCorrection):
				t.Errorf("concurrent correction: %v", err)
			}
		}()
	}
	wg.Wait()
	if len(created) != 1 {
		t.Fatalf("concurrent corrections created %d calibrations", len(created))
	}
	if view := status(st.test, st.first); view.Status != application.Calibrated || view.Current.ID != created[0].ID {
		t.Fatalf("after correction: %+v", view)
	}
	chain, err := s.Overrides(ctx, viewer, st.test, st.first)
	if err != nil || len(chain) != 3 || chain[0].ID != first.ID || chain[0].Transform != first.Transform || chain[1].Status != domain.Rejected {
		t.Fatalf("older calibrations stay readable and unchanged: %+v %v", chain, err)
	}

	// While a replacement video is still uploading, the verified video and its
	// override stay in effect; once verified, the replacement is current and
	// falls back to the arena default (none here) until the animal is
	// calibrated on it.
	st.video(t, st.test, false)
	if view := status(st.test, st.first); view.Status != application.Calibrated || view.Current.ID != created[0].ID {
		t.Fatalf("pending replacement: %+v", view)
	}
	replacement := st.video(t, st.test, true)
	if view := status(st.test, st.first); view.Status != application.Waiting || view.Source != "" || view.RecordingID != replacement {
		t.Fatalf("verified replacement: %+v", view)
	}
	// The chain continues across videos, and the new video may use another camera.
	moved := input(created[0].ID)
	moved.CameraID, moved.ReferenceFrameUs = "cam-top-2", 10
	again, err := s.Calibrate(ctx, technician, st.test, st.first, moved)
	if err != nil || again.RecordingID != replacement || again.SupersedesID != created[0].ID {
		t.Fatalf("override on the replacement: %+v %v", again, err)
	}
	if view := status(st.test, st.first); view.Status != application.Calibrated || view.Current.ID != again.ID {
		t.Fatalf("calibrated on the replacement: %+v", view)
	}

	for name, sql := range map[string]string{
		"edit":   "UPDATE misko.calibrations SET tolerance_cm = 10 WHERE id = $1",
		"delete": "DELETE FROM misko.calibrations WHERE id = $1",
	} {
		if _, err := pool.Exec(ctx, sql, first.ID); err == nil {
			t.Errorf("%s: direct SQL succeeded", name)
		}
	}
}

func TestEnvironmentDefaultsServeEveryArenaOfAVideo(t *testing.T) {
	pool := pgtest.Database(t)
	s := application.New(postgres.NewStore(pool), catalog.New(), time.Now)
	st := runningStudy(t, pool)
	rec := st.video(t, st.test, true)
	lab := access.Actor{UserID: "01a00000-0000-7000-8000-000000000001", Role: access.LabManager}

	if view, err := s.EnvironmentStatus(ctx, viewer, st.environmentA, 1); err != nil || view.Status != application.Waiting || view.Source != "" {
		t.Fatalf("uncalibrated environment: %+v %v", view, err)
	}
	if _, err := s.CalibrateEnvironment(ctx, technician, st.environmentA, 1, input("")); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("a technician cannot calibrate the rig: %v", err)
	}
	if _, err := s.CalibrateEnvironment(ctx, lab, st.environmentA, 2, input("")); !errors.Is(err, application.ErrRevisionNotFound) {
		t.Fatalf("unknown revision: %v", err)
	}
	defaultA, err := s.CalibrateEnvironment(ctx, lab, st.environmentA, 1, input(""))
	if err != nil || defaultA.Status != domain.Valid || defaultA.RecordingID != "" || defaultA.TestSubjectID != "" || defaultA.EnvironmentRevisionID != st.arenaA ||
		defaultA.Scope() != domain.ScopeEnvironment {
		t.Fatalf("environment calibration: %+v %v", defaultA, err)
	}
	if _, err := s.CalibrateEnvironment(ctx, lab, st.environmentA, 1, input("")); !errors.Is(err, domain.ErrStaleCorrection) {
		t.Fatalf("second first environment calibration: %v", err)
	}
	if view, err := s.EnvironmentStatus(ctx, viewer, st.environmentA, 1); err != nil || view.Status != application.Calibrated || view.Current.ID != defaultA.ID {
		t.Fatalf("calibrated environment: %+v %v", view, err)
	}

	// Each animal of the shared video uses its own arena's default.
	statuses := func() map[string]application.SubjectStatus {
		t.Helper()
		list, err := s.TestStatus(ctx, viewer, st.test)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]application.SubjectStatus{}
		for _, v := range list {
			out[v.TestSubjectID] = v
		}
		return out
	}
	got := statuses()
	if a, b := got[st.first], got[st.second]; len(got) != 2 || a.Status != application.Calibrated || a.Current.ID != defaultA.ID || a.Source != domain.SourceEnvironment ||
		a.Drift != application.DriftUnchecked || a.RecordingID != rec || a.EnvironmentRevisionID != st.arenaA || b.Status != application.Waiting || b.EnvironmentRevisionID != st.arenaB {
		t.Fatalf("one arena calibrated: %+v", got)
	}
	if _, err := s.TestStatus(ctx, viewer, "01a00000-0000-7000-8000-00000000dead"); !errors.Is(err, application.ErrTestNotFound) {
		t.Fatalf("unknown test: %v", err)
	}
	if own, err := s.Overrides(ctx, viewer, st.test, st.first); err != nil || len(own) != 0 {
		t.Fatalf("the animal's own list stays empty: %+v %v", own, err)
	}

	// A correction of the default is one chain and is used at once.
	corrected, err := s.CalibrateEnvironment(ctx, lab, st.environmentA, 1, input(defaultA.ID))
	if err != nil || corrected.SupersedesID != defaultA.ID {
		t.Fatalf("environment correction: %+v %v", corrected, err)
	}
	if view := statuses()[st.first]; view.Current == nil || view.Current.ID != corrected.ID {
		t.Fatalf("after correction: %+v", view)
	}

	// An override replaces the default for that animal only; a rejected one
	// waits and never falls back to the default.
	in := input("")
	in.ReferenceFrameUs = 0
	override, err := s.Calibrate(ctx, technician, st.test, st.first, in)
	if err != nil || override.EnvironmentRevisionID != st.arenaA {
		t.Fatalf("override: %+v %v", override, err)
	}
	if view := statuses()[st.first]; view.Source != domain.SourceOverride || view.Current.ID != override.ID {
		t.Fatalf("override wins: %+v", view)
	}
	bad := input(override.ID)
	bad.Check[2].WorldY += 3
	rejected, err := s.Calibrate(ctx, technician, st.test, st.first, bad)
	if err != nil || rejected.Status != domain.Rejected {
		t.Fatalf("rejected override: %+v %v", rejected, err)
	}
	if view := statuses()[st.first]; view.Status != application.Waiting || view.Source != domain.SourceOverride {
		t.Fatalf("rejected override waits: %+v", view)
	}
	// The other test filmed in arena A is untouched by the override.
	st.video(t, st.otherTest, true)
	if view, err := s.SubjectStatus(ctx, viewer, st.otherTest, st.other); err != nil || view.Status != application.Calibrated || view.Current.ID != corrected.ID {
		t.Fatalf("another test in the same arena: %+v %v", view, err)
	}

	// The database refuses what the service never sends.
	insert := `INSERT INTO misko.calibrations (environment_revision_id, test_subject_id, recording_id, supersedes_id, camera_id, frame_width, frame_height,
		crop_x, crop_y, crop_width, crop_height, reference_frame_us, measurement_plane, fit_points, check_points, transform, fit_rms_error_cm,
		check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, created_by)
		SELECT $1, $2, $3, $4, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height, $5, measurement_plane,
		fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, created_by
		FROM misko.calibrations WHERE id = $6`
	for name, tc := range map[string]struct {
		revision          string
		subject, recorded any
		supersedes, frame any
		constraint        string
	}{
		"an override in another arena":             {st.arenaB, st.first, rec, rejected.ID, 0, "calibrations_scope"},
		"an environment correction of an override": {st.arenaA, nil, nil, rejected.ID, nil, "calibrations_supersedes_fkey"},
		"an override correcting the default":       {st.arenaA, st.first, rec, corrected.ID, 0, "calibrations_supersedes_fkey"},
		"a second default chain head":              {st.arenaA, nil, nil, nil, nil, "calibrations_first_key"},
	} {
		_, err := pool.Exec(ctx, insert, tc.revision, tc.subject, tc.recorded, tc.supersedes, tc.frame, override.ID)
		if _, constraint := pgtx.Violation(err); constraint != tc.constraint {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func second[T any](_ T, err error) error { return err }
