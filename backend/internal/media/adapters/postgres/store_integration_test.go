//go:build integration

package postgres_test

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	ctx        = context.Background()
	technician = access.Actor{UserID: "01a00000-0000-7000-8000-000000000002", Role: access.Technician}
	other      = access.Actor{UserID: "01a00000-0000-7000-8000-000000000005", Role: access.Technician}
	viewer     = access.Actor{UserID: "01a00000-0000-7000-8000-000000000003", Role: access.Viewer}
	settings   = application.Settings{MaxBytes: 1 << 30, UploadTTL: time.Hour, ReadTTL: 15 * time.Minute}
)

// objects is an in-memory object store; it is not evidence of GCS behavior.
type objects struct {
	mu      sync.Mutex
	stored  map[string]domain.ObjectAttrs
	nextGen int64
}

func (o *objects) Bucket() string { return "misko-test-videos" }

func (o *objects) UploadURL(_ context.Context, object, contentType string, expires time.Time) (application.SignedRequest, error) {
	return application.SignedRequest{Method: "POST", URL: "https://storage.test/" + object, Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: expires}, nil
}

func (o *objects) SizedUploadURL(ctx context.Context, object, contentType string, size int64, expires time.Time) (application.SignedRequest, error) {
	signed, err := o.UploadURL(ctx, object, contentType, expires)
	signed.Headers["x-goog-content-length-range"] = "0," + strconv.FormatInt(size, 10)
	return signed, err
}

func (o *objects) ReadURL(_ context.Context, object string, generation int64, expires time.Time) (string, error) {
	return "https://storage.test/" + object + "?generation=" + strconv.FormatInt(generation, 10), nil
}

func (o *objects) Attrs(_ context.Context, object string) (domain.ObjectAttrs, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	attrs, ok := o.stored[object]
	if !ok {
		return domain.ObjectAttrs{}, application.ErrObjectNotFound
	}
	return attrs, nil
}

func (o *objects) Read(context.Context, string, int64, int64) ([]byte, error) {
	return nil, application.ErrObjectNotFound
}

func (o *objects) put(object string, size int64, crc uint32) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.nextGen++
	o.stored[object] = domain.ObjectAttrs{Generation: 1_757_592_000_000_000 + o.nextGen, Size: size, CRC32C: crc, ContentType: "video/mp4"}
}

func id(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var out string
	if err := pool.QueryRow(ctx, sql, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// study is a running experiment with one test in progress filming one
// animal and one planned test, created with SQL only because the contexts that
// own these tables are ported separately.
type study struct {
	experiment, test, planned string
}

func runningStudy(t *testing.T, pool *pgxpool.Pool, code string) study {
	t.Helper()
	exp := id(t, pool, "INSERT INTO misko.experiments (code, title) VALUES ($1, 'Study') RETURNING id", code)
	group := id(t, pool, "INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id", exp)
	timepoint := id(t, pool, "INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Day 1', 1, 1) RETURNING id", exp)
	protocol := id(t, pool, "INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id", exp)
	var version string
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by) VALUES ($1, $2, 1, 1, $3) RETURNING id",
			exp, protocol, technician.UserID).Scan(&version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session) VALUES ($1, 1, 'MWM', 1, 'ACQUISITION', 1, '{}')", version)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1", exp); err != nil {
		t.Fatal(err)
	}
	subject := id(t, pool, "INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id", "S-"+code)
	allocation := id(t, pool, "INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by) VALUES ($1, $2, $3, now() - interval '1 day', $4) RETURNING id",
		exp, group, subject, technician.UserID)
	env := id(t, pool, "INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, 'MWM') RETURNING id", "Arena "+code)
	arena := id(t, pool, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
		VALUES ($1, 'MWM', 1, 1, '{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30}', $2) RETURNING id`, env, technician.UserID)
	plan := `INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
		VALUES ($1, $2, $3, 1, 'MWM', 1, 1, now(), $4) RETURNING id`
	test, planned := id(t, pool, plan, exp, timepoint, version, technician.UserID), id(t, pool, plan, exp, timepoint, version, technician.UserID)
	if _, err := pool.Exec(ctx, `INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
		VALUES ($1, $2, $3, $4, 'MWM', 1, $5)`, exp, test, allocation, arena, technician.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() WHERE id = $1", test); err != nil {
		t.Fatal(err)
	}
	return study{experiment: exp, test: test, planned: planned}
}

func request(size int64) domain.UploadRequest {
	return domain.UploadRequest{FileName: "trial.mp4", ContentType: "video/mp4", SizeBytes: size, CRC32C: domain.FormatCRC32C(0xe2932d1b), ClipStartUs: 250_000}
}

func TestUploadsAreVerifiedAndPinned(t *testing.T) {
	pool := pgtest.Database(t)
	store := &objects{stored: map[string]domain.ObjectAttrs{}}
	s := application.New(postgres.NewStore(pool), store, settings, time.Now)
	first, elsewhere := runningStudy(t, pool, "E1"), runningStudy(t, pool, "E2")
	test, foreign := first.test, elsewhere.test

	upload, err := s.StartUpload(ctx, technician, test, request(4096))
	if err != nil {
		t.Fatal(err)
	}
	rec := upload.Recording
	if rec.Asset.Status != domain.Pending || rec.Asset.Bucket != "misko-test-videos" || rec.Asset.ObjectName != "tests/"+test+"/originals/"+rec.Asset.ID ||
		rec.Clip.StartUs != 250_000 || rec.TestID != test || rec.Current || rec.SupersedesID != "" || !strings.HasSuffix(upload.Request.URL, rec.Asset.ObjectName) ||
		upload.Request.Headers["x-goog-content-length-range"] != "0,4096" {
		t.Fatalf("started: %+v %+v", rec, upload.Request)
	}
	if _, err := s.FinalizeUpload(ctx, technician, test, rec.ID); !errors.Is(err, application.ErrUploadIncomplete) {
		t.Fatalf("finalize before the upload finished: %v", err)
	}
	if _, err := s.FinalizeUpload(ctx, other, test, rec.ID); !errors.Is(err, application.ErrNotUploader) {
		t.Fatalf("finalize by another technician: %v", err)
	}
	if _, err := s.ReadURL(ctx, viewer, test, rec.ID); !errors.Is(err, application.ErrNotVerified) {
		t.Fatalf("read a pending video: %v", err)
	}
	store.put(rec.Asset.ObjectName, 4096, 0xe2932d1b)

	var wg sync.WaitGroup
	results := make(chan domain.Recording, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.FinalizeUpload(ctx, technician, test, rec.ID)
			if err != nil {
				t.Errorf("concurrent finalize: %v", err)
			}
			results <- r
		}()
	}
	wg.Wait()
	close(results)
	var verified domain.Recording
	for r := range results {
		if r.Asset.Status != domain.Verified || r.Asset.VerifiedAt == nil {
			t.Fatalf("finalized: %+v", r.Asset)
		}
		if verified.ID != "" && (!r.Asset.VerifiedAt.Equal(*verified.Asset.VerifiedAt) || r.Asset.Generation != verified.Asset.Generation) {
			t.Fatalf("finalizations disagree: %+v %+v", r.Asset, verified.Asset)
		}
		verified = r
	}

	// Overwriting the object later does not change the verified reference.
	store.put(rec.Asset.ObjectName, 4096, 0xe2932d1b)
	again, err := s.FinalizeUpload(ctx, technician, test, rec.ID)
	if err != nil || again.Asset.Generation != verified.Asset.Generation {
		t.Fatalf("repeated finalization: %+v %v", again.Asset, err)
	}
	link, err := s.ReadURL(ctx, viewer, test, rec.ID)
	if err != nil || !strings.HasSuffix(link.URL, "?generation="+strconv.FormatInt(verified.Asset.Generation, 10)) {
		t.Fatalf("read URL: %+v %v", link, err)
	}
	for name, err := range map[string]error{
		"read through another test":     second(s.ReadURL(ctx, viewer, foreign, rec.ID)),
		"finalize through another test": second(s.FinalizeUpload(ctx, technician, foreign, rec.ID)),
		"malformed recording":           second(s.ReadURL(ctx, viewer, test, "recording")),
	} {
		if !errors.Is(err, application.ErrRecordingNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}

	corrupt, err := s.StartUpload(ctx, technician, test, request(2048))
	if err != nil {
		t.Fatal(err)
	}
	store.put(corrupt.Recording.Asset.ObjectName, 2048, 7)
	rejected, err := s.FinalizeUpload(ctx, technician, test, corrupt.Recording.ID)
	if err != nil || rejected.Asset.Status != domain.Rejected || rejected.Asset.RejectionReason != domain.RejectedChecksumMismatch {
		t.Fatalf("corrupt upload: %+v %v", rejected.Asset, err)
	}
	store.put(corrupt.Recording.Asset.ObjectName, 2048, 0xe2932d1b)
	if still, err := s.FinalizeUpload(ctx, technician, test, corrupt.Recording.ID); err != nil || still.Asset.Status != domain.Rejected {
		t.Fatalf("a rejected upload stays rejected: %+v %v", still.Asset, err)
	}
	if _, err := s.ReadURL(ctx, viewer, test, corrupt.Recording.ID); !errors.Is(err, application.ErrNotVerified) {
		t.Fatalf("read a rejected video: %v", err)
	}
	// The corrupt upload extends the chain but is rejected, so the verified
	// video stays current; both stay readable in the list.
	list, err := s.Recordings(ctx, viewer, test)
	if err != nil || len(list) != 2 || list[0].ID != rec.ID || !list[0].Current || list[1].Asset.Status != domain.Rejected ||
		list[1].Current || list[1].SupersedesID != rec.ID {
		t.Fatalf("recordings: %+v %v", list, err)
	}
	if current, err := s.Current(ctx, viewer, test); err != nil || current.ID != rec.ID {
		t.Fatalf("current recording: %+v %v", current, err)
	}
	if _, err := s.Current(ctx, viewer, first.planned); !errors.Is(err, application.ErrRecordingNotFound) {
		t.Fatalf("current recording of a test without video: %v", err)
	}
	if _, err := s.Recordings(ctx, viewer, "01a00000-0000-7000-8000-00000000dead"); !errors.Is(err, application.ErrTestNotFound) {
		t.Fatalf("recordings of an unknown test: %v", err)
	}

	// Stored references cannot be edited, moved or deleted with direct SQL.
	for name, sql := range map[string]string{
		"change generation": "UPDATE misko.video_assets SET generation = generation + 1 WHERE id = $1",
		"move object":       "UPDATE misko.video_assets SET object_name = 'elsewhere' WHERE id = $1",
		"delete asset":      "DELETE FROM misko.video_assets WHERE id = $1",
		"delete recording":  "DELETE FROM misko.test_recordings WHERE video_asset_id = $1",
		"move recording":    "UPDATE misko.test_recordings SET clip_start_us = 0 WHERE video_asset_id = $1",
	} {
		if _, err := pool.Exec(ctx, sql, verified.Asset.ID); err == nil {
			t.Errorf("%s: direct SQL succeeded", name)
		}
	}
	_, err = pool.Exec(ctx, `INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, created_by)
		VALUES ('ORIGINAL', 'misko-test-videos', $1, 'video/mp4', 1, 1, $2)`, verified.Asset.ObjectName, technician.UserID)
	if _, constraint := pgtx.Violation(err); constraint != "video_assets_object_key" {
		t.Errorf("reused object key: %v", err)
	}
	loose := id(t, pool, `INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, created_by)
		VALUES ('ORIGINAL', 'misko-test-videos', 'loose/object', 'video/mp4', 1, 1, $1) RETURNING id`, technician.UserID)
	_, err = pool.Exec(ctx, `INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, created_by)
		SELECT r.experiment_id, $1, $2, r.created_by FROM misko.test_recordings r WHERE r.id = $3`, foreign, loose, rec.ID)
	if _, constraint := pgtx.Violation(err); constraint != "test_recordings_test_fkey" {
		t.Errorf("a recording joined a test of another experiment: %v", err)
	}
	// A second chain head or a fork of the chain is refused by the database.
	for name, supersedes := range map[string]any{"second first video": nil, "fork of a superseded video": rec.ID} {
		_, err = pool.Exec(ctx, `INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, supersedes_id, created_by) VALUES ($1, $2, $3, $4, $5)`,
			first.experiment, test, loose, supersedes, technician.UserID)
		if _, constraint := pgtx.Violation(err); constraint != "test_recordings_first_key" && constraint != "test_recordings_supersedes_key" {
			t.Errorf("%s: %v", name, err)
		}
	}

	for name, tc := range map[string]struct {
		test string
		want error
	}{
		"planned test":   {first.planned, domain.ErrTestNotStarted},
		"unknown test":   {"01a00000-0000-7000-8000-00000000dead", application.ErrTestNotFound},
		"malformed test": {"test", application.ErrTestNotFound},
	} {
		if _, err := s.StartUpload(ctx, technician, tc.test, request(10)); !errors.Is(err, tc.want) {
			t.Errorf("upload to a %s: %v", name, err)
		}
	}
	// The database rule behind the status check maps to the same error.
	pending, _, err := domain.NewUpload(request(10), settings.MaxBytes, technician.UserID)
	if err != nil {
		t.Fatal(err)
	}
	pending.Bucket = "misko-test-videos"
	err = postgres.NewStore(pool).Transaction(ctx, func(tx application.Store) error {
		_, err := tx.CreateRecording(ctx, application.TestRef{ID: first.planned, ExperimentID: first.experiment, Status: "IN_PROGRESS"}, pending, domain.Clip{}, "")
		return err
	})
	if !errors.Is(err, domain.ErrTestNotStarted) {
		t.Fatalf("a video for a planned test past the status check: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE misko.tests SET status = 'CANCELLED', cancelled_at = now(), cancel_reason = 'duplicate' WHERE id = $1", foreign); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartUpload(ctx, technician, foreign, request(10)); !errors.Is(err, domain.ErrTestNotStarted) {
		t.Fatalf("upload to a cancelled test: %v", err)
	}
	// A completed test still takes a replacement video.
	if _, err := pool.Exec(ctx, "UPDATE misko.tests SET status = 'COMPLETED', completed_at = now() WHERE id = $1", test); err != nil {
		t.Fatal(err)
	}
	if replaced, err := s.StartUpload(ctx, technician, test, request(10)); err != nil || replaced.Recording.SupersedesID != corrupt.Recording.ID {
		t.Fatalf("replacement on a completed test: %+v %v", replaced.Recording, err)
	}
}

// Concurrent uploads to one test form one linear chain: each supersedes the
// one that was current when it took the test's lock.
func TestConcurrentUploadsFormOneChain(t *testing.T) {
	pool := pgtest.Database(t)
	s := application.New(postgres.NewStore(pool), &objects{stored: map[string]domain.ObjectAttrs{}}, settings, time.Now)
	test := runningStudy(t, pool, "E1").test
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.StartUpload(ctx, technician, test, request(10)); err != nil {
				t.Errorf("concurrent upload: %v", err)
			}
		}()
	}
	wg.Wait()
	list, err := s.Recordings(ctx, viewer, test)
	if err != nil || len(list) != 6 {
		t.Fatalf("recordings: %d %v", len(list), err)
	}
	superseded, current := map[string]bool{}, 0
	for _, r := range list {
		if r.SupersedesID != "" {
			superseded[r.SupersedesID] = true
		}
		if r.Current {
			current++
		}
	}
	// Every upload joins one linear chain; none is verified, so none is current.
	if len(superseded) != 5 || current != 0 {
		t.Fatalf("chain: %d superseded, %d current", len(superseded), current)
	}
}

func second[T any](_ T, err error) error { return err }
