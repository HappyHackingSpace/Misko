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

// recordings creates an OPEN_FIELD test with a verified and a pending
// recording, and a second test, using SQL only.
func recordings(t *testing.T, pool *pgxpool.Pool) (test, verified, pending, otherTest string) {
	t.Helper()
	exp := id(t, pool, "INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id")
	subject := id(t, pool, "INSERT INTO misko.subjects (code, species, sex) VALUES ('S1', 'MOUSE', 'MALE') RETURNING id")
	enrollment := id(t, pool, "INSERT INTO misko.enrollments (experiment_id, subject_id, enrolled_at) VALUES ($1, $2, now() - interval '2 days') RETURNING id", exp, subject)
	env := id(t, pool, "INSERT INTO misko.environments (name, paradigm_key) VALUES ('Arena', 'OPEN_FIELD') RETURNING id")
	rev := id(t, pool, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, created_by)
		VALUES ($1, 'OPEN_FIELD', 1, 1, '{"arena_width_cm": 50, "arena_height_cm": 40, "center_fraction": 0.5}', $2) RETURNING id`, env, technician.UserID)
	protocol := id(t, pool, "INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id", exp)
	var version string
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by) VALUES ($1, $2, 1, 1, $3) RETURNING id", exp, protocol, technician.UserID).Scan(&version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, environment_revision_id, trial_type, trials, inter_trial_interval_s, session) VALUES ($1, 1, 'OPEN_FIELD', 1, $2, 'STANDARD', 1, 0, '{}')", version, rev)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	newTest := func() string {
		return id(t, pool, `INSERT INTO misko.tests (experiment_id, enrollment_id, subject_id, protocol_version_id, step_position, paradigm_key, paradigm_version, environment_revision_id, planned_trials, scheduled_at, created_by)
			VALUES ($1, $2, $3, $4, 1, 'OPEN_FIELD', 1, $5, 1, now() - interval '1 day', $6) RETURNING id`, exp, enrollment, subject, version, rev, technician.UserID)
	}
	recording := func(testID, object string, verified bool) string {
		status, generation, verifiedAt := "PENDING", any(nil), any(nil)
		if verified {
			status, generation, verifiedAt = "VERIFIED", int64(42), time.Now()
		}
		asset := id(t, pool, `INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
			VALUES ('ORIGINAL', 'misko-test-videos', $1, 'video/mp4', 1024, 1, $2, $3, $4, $5) RETURNING id`, object, status, generation, verifiedAt, technician.UserID)
		return id(t, pool, "INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, created_by) VALUES ($1, $2, $3, $4) RETURNING id", exp, testID, asset, technician.UserID)
	}
	test, otherTest = newTest(), newTest()
	return test, recording(test, "a", true), recording(test, "b", false), otherTest
}

func TestCalibrationChainsGateAnalysis(t *testing.T) {
	pool := pgtest.Database(t)
	s := application.New(postgres.NewStore(pool), catalog.New(), time.Now)
	test, rec, pending, otherTest := recordings(t, pool)

	if view, err := s.Status(ctx, viewer, test, rec); err != nil || view.Status != application.Waiting || view.Current != nil {
		t.Fatalf("before calibration: %+v %v", view, err)
	}
	first, err := s.Calibrate(ctx, technician, test, rec, input(""))
	if err != nil || first.Status != domain.Valid || first.ToleranceCm != 2 || first.RecordingID != rec || first.FitRMSErrorCm > 1e-6 {
		t.Fatalf("first calibration: %+v %v", first, err)
	}
	if view, err := s.Status(ctx, viewer, test, rec); err != nil || view.Status != application.Calibrated || view.Current.ID != first.ID {
		t.Fatalf("calibrated: %+v %v", view, err)
	}
	if _, err := s.Status(ctx, viewer, otherTest, rec); !errors.Is(err, application.ErrRecordingNotFound) {
		t.Fatalf("status through another test: %v", err)
	}
	if _, err := s.Calibrations(ctx, viewer, otherTest, rec); !errors.Is(err, application.ErrRecordingNotFound) {
		t.Fatalf("calibrations through another test: %v", err)
	}

	for name, tc := range map[string]struct {
		test, recording string
		in              domain.Input
		want            error
	}{
		"second first calibration":  {test, rec, input(""), domain.ErrStaleCorrection},
		"unverified video":          {test, pending, input(""), application.ErrVideoNotVerified},
		"recording of another test": {otherTest, rec, input(""), application.ErrRecordingNotFound},
		"point off the arena": {test, rec, func() domain.Input {
			in := input(first.ID)
			in.Check[0].WorldX = 80
			return in
		}(), domain.ErrPointOutsideEnvironment},
		"correction from another camera": {test, rec, func() domain.Input { in := input(first.ID); in.FrameHeight = 720; in.Crop.Height = 720; return in }(), domain.ErrCameraMismatch},
	} {
		if _, err := s.Calibrate(ctx, technician, tc.test, tc.recording, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}

	off := input(first.ID)
	off.Check[2].WorldY += 3
	rejected, err := s.Calibrate(ctx, technician, test, rec, off)
	if err != nil || rejected.Status != domain.Rejected || rejected.RejectionReason != domain.RejectedCheckError || rejected.SupersedesID != first.ID {
		t.Fatalf("excessive CHECK error: %+v %v", rejected, err)
	}
	if view, _ := s.Status(ctx, viewer, test, rec); view.Status != application.Waiting {
		t.Fatalf("a rejected correction leaves the video waiting: %+v", view)
	}

	// Concurrent corrections of the same calibration: exactly one extends the chain.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var created []domain.Calibration
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := s.Calibrate(ctx, technician, test, rec, input(rejected.ID))
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
	if view, _ := s.Status(ctx, viewer, test, rec); view.Status != application.Calibrated || view.Current.ID != created[0].ID {
		t.Fatalf("after correction: %+v", view)
	}
	chain, err := s.Calibrations(ctx, viewer, test, rec)
	if err != nil || len(chain) != 3 || chain[0].ID != first.ID || chain[0].Transform != first.Transform || chain[1].Status != domain.Rejected {
		t.Fatalf("older calibrations stay readable and unchanged: %+v %v", chain, err)
	}

	for name, sql := range map[string]string{
		"edit":   "UPDATE misko.calibrations SET tolerance_cm = 10 WHERE id = $1",
		"delete": "DELETE FROM misko.calibrations WHERE id = $1",
	} {
		if _, err := pool.Exec(ctx, sql, first.ID); err == nil {
			t.Errorf("%s: direct SQL succeeded", name)
		}
	}
	insert := `INSERT INTO misko.calibrations (recording_id, environment_revision_id, supersedes_id, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height,
		reference_frame_us, measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm,
		tolerance_cm, algorithm_version, status, created_by)
		SELECT $1, environment_revision_id, $2, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height, reference_frame_us, measurement_plane,
		fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, created_by
		FROM misko.calibrations WHERE id = $3`
	_, err = pool.Exec(ctx, insert, rec, nil, first.ID)
	if _, constraint := pgtx.Violation(err); constraint != "calibrations_first_key" {
		t.Errorf("second root through SQL: %v", err)
	}
	// The latest calibration is not superseded yet, so only the scope foreign key can refuse this.
	_, err = pool.Exec(ctx, insert, pending, created[0].ID, first.ID)
	if _, constraint := pgtx.Violation(err); constraint != "calibrations_supersedes_fkey" {
		t.Errorf("correction of another recording's calibration: %v", err)
	}
}

func TestEnvironmentCalibrationIsInheritedAndOverridden(t *testing.T) {
	pool := pgtest.Database(t)
	s := application.New(postgres.NewStore(pool), catalog.New(), time.Now)
	test, rec, pending, _ := recordings(t, pool)
	env := id(t, pool, "SELECT e.environment_id FROM misko.tests t JOIN misko.environment_revisions e ON e.id = t.environment_revision_id WHERE t.id = $1", test)
	revision := id(t, pool, "SELECT environment_revision_id FROM misko.tests WHERE id = $1", test)
	lab := access.Actor{UserID: "01a00000-0000-7000-8000-000000000001", Role: access.LabManager}

	if view, err := s.EnvironmentStatus(ctx, viewer, env, 1); err != nil || view.Status != application.Waiting || view.Source != "" {
		t.Fatalf("uncalibrated environment: %+v %v", view, err)
	}
	if _, err := s.CalibrateEnvironment(ctx, technician, env, 1, input("")); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("a technician cannot calibrate the rig: %v", err)
	}
	if _, err := s.CalibrateEnvironment(ctx, lab, env, 2, input("")); !errors.Is(err, application.ErrRevisionNotFound) {
		t.Fatalf("unknown revision: %v", err)
	}
	def, err := s.CalibrateEnvironment(ctx, lab, env, 1, input(""))
	if err != nil || def.Status != domain.Valid || def.RecordingID != "" || def.EnvironmentRevisionID != revision || def.Scope() != domain.ScopeEnvironment {
		t.Fatalf("environment calibration: %+v %v", def, err)
	}
	if _, err := s.CalibrateEnvironment(ctx, lab, env, 1, input("")); !errors.Is(err, domain.ErrStaleCorrection) {
		t.Fatalf("second first environment calibration: %v", err)
	}

	// Every verified recording of the revision is calibrated without a calibration of its own.
	view, err := s.Status(ctx, viewer, test, rec)
	if err != nil || view.Status != application.Calibrated || view.Current.ID != def.ID || view.Source != domain.SourceEnvironment || view.Drift != application.DriftUnchecked {
		t.Fatalf("inherited: %+v %v", view, err)
	}
	if view, _ := s.Status(ctx, viewer, test, pending); view.Status != application.Waiting {
		t.Fatalf("an unverified video still waits: %+v", view)
	}
	if own, err := s.Calibrations(ctx, viewer, test, rec); err != nil || len(own) != 0 {
		t.Fatalf("the recording's own list stays empty: %+v %v", own, err)
	}
	listed, err := s.EnvironmentCalibrations(ctx, viewer, env, 1)
	if err != nil || len(listed) != 1 || listed[0].ID != def.ID {
		t.Fatalf("environment chain: %+v %v", listed, err)
	}

	// The scheduler view agrees with the domain rule.
	effective := func(recording string) (source, calibration string) {
		t.Helper()
		var src, cal *string
		if err := pool.QueryRow(ctx, "SELECT source, calibration_id::text FROM misko.effective_calibrations WHERE recording_id = $1", recording).Scan(&src, &cal); err != nil {
			t.Fatal(err)
		}
		return valueOf(src), valueOf(cal)
	}
	if src, cal := effective(rec); src != "ENVIRONMENT" || cal != def.ID {
		t.Fatalf("effective_calibrations: %q %q", src, cal)
	}

	// A correction of the default is one chain and does not touch the recording chain.
	corrected, err := s.CalibrateEnvironment(ctx, lab, env, 1, input(def.ID))
	if err != nil || corrected.SupersedesID != def.ID {
		t.Fatalf("environment correction: %+v %v", corrected, err)
	}
	if src, cal := effective(rec); src != "ENVIRONMENT" || cal != corrected.ID {
		t.Fatalf("after correction: %q %q", src, cal)
	}

	// A manual calibration overrides the default for that recording only.
	override, err := s.Calibrate(ctx, technician, test, rec, input(""))
	if err != nil || override.RecordingID != rec || override.EnvironmentRevisionID != revision {
		t.Fatalf("override: %+v %v", override, err)
	}
	if view, _ := s.Status(ctx, viewer, test, rec); view.Source != domain.SourceRecording || view.Current.ID != override.ID {
		t.Fatalf("override wins: %+v", view)
	}
	if src, cal := effective(rec); src != "RECORDING" || cal != override.ID {
		t.Fatalf("effective_calibrations with override: %q %q", src, cal)
	}

	// A rejected override waits and never falls back to the default.
	bad := input(override.ID)
	bad.Check[2].WorldY += 3
	rejected, err := s.Calibrate(ctx, technician, test, rec, bad)
	if err != nil || rejected.Status != domain.Rejected {
		t.Fatalf("rejected override: %+v %v", rejected, err)
	}
	if view, _ := s.Status(ctx, viewer, test, rec); view.Status != application.Waiting || view.Source != domain.SourceRecording {
		t.Fatalf("rejected override waits: %+v", view)
	}
	if src, cal := effective(rec); src != "RECORDING" || cal != "" {
		t.Fatalf("effective_calibrations with rejected override: %q %q", src, cal)
	}

	insert := `INSERT INTO misko.calibrations (recording_id, environment_revision_id, supersedes_id, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height,
		reference_frame_us, measurement_plane, fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm,
		tolerance_cm, algorithm_version, status, created_by)
		SELECT $1, $2, $3, camera_id, frame_width, frame_height, crop_x, crop_y, crop_width, crop_height, reference_frame_us, measurement_plane,
		fit_points, check_points, transform, fit_rms_error_cm, check_rms_error_cm, check_max_error_cm, tolerance_cm, algorithm_version, status, created_by
		FROM misko.calibrations WHERE id = $4`
	otherRevision := id(t, pool, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, created_by)
		VALUES ($1, 'OPEN_FIELD', 2, 1, '{"arena_width_cm": 50, "arena_height_cm": 40, "center_fraction": 0.5}', $2) RETURNING id`, env, technician.UserID)
	for name, tc := range map[string]struct {
		recording  any
		revision   string
		supersedes any
		constraint string
	}{
		"a recording calibration of another revision":          {rec, otherRevision, nil, "calibrations_scope"},
		"an environment correction of a recording calibration": {nil, revision, rejected.ID, "calibrations_supersedes_fkey"},
		"a recording correction of an environment calibration": {rec, revision, corrected.ID, "calibrations_supersedes_fkey"},
	} {
		_, err := pool.Exec(ctx, insert, tc.recording, tc.revision, tc.supersedes, override.ID)
		if _, constraint := pgtx.Violation(err); constraint != tc.constraint {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func valueOf(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
