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

// plannedTests creates two experiments, each with one planned test, using SQL only.
func plannedTests(t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	var tests []string
	for i, code := range []string{"E1", "E2"} {
		exp := id(t, pool, "INSERT INTO misko.experiments (code, title) VALUES ($1, 'Study') RETURNING id", code)
		subject := id(t, pool, "INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id", "S"+code)
		enrollment := id(t, pool, "INSERT INTO misko.enrollments (experiment_id, subject_id, enrolled_at) VALUES ($1, $2, now() - interval '2 days') RETURNING id", exp, subject)
		env := id(t, pool, "INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, 'OPEN_FIELD') RETURNING id", "Arena "+code)
		rev := id(t, pool, "INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, created_by) VALUES ($1, 'OPEN_FIELD', 1, 1, '{}', $2) RETURNING id", env, technician.UserID)
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
		tests = append(tests, id(t, pool, `INSERT INTO misko.tests (experiment_id, enrollment_id, subject_id, protocol_version_id, step_position, paradigm_key, paradigm_version, environment_revision_id, planned_trials, scheduled_at, created_by)
			VALUES ($1, $2, $3, $4, 1, 'OPEN_FIELD', 1, $5, 1, now() - interval '1 day', $6) RETURNING id`, exp, enrollment, subject, version, rev, technician.UserID))
		_ = i
	}
	return tests[0], tests[1]
}

func request(size int64) domain.UploadRequest {
	return domain.UploadRequest{FileName: "trial.mp4", ContentType: "video/mp4", SizeBytes: size, CRC32C: domain.FormatCRC32C(0xe2932d1b), ClipStartUs: 250_000}
}

func TestUploadsAreVerifiedAndPinned(t *testing.T) {
	pool := pgtest.Database(t)
	store := &objects{stored: map[string]domain.ObjectAttrs{}}
	s := application.New(postgres.NewStore(pool), store, settings, time.Now)
	test, foreign := plannedTests(t, pool)

	upload, err := s.StartUpload(ctx, technician, test, request(4096))
	if err != nil {
		t.Fatal(err)
	}
	rec := upload.Recording
	if rec.Asset.Status != domain.Pending || rec.Asset.Bucket != "misko-test-videos" || rec.Asset.ObjectName != "tests/"+test+"/originals/"+rec.Asset.ID ||
		rec.Clip.StartUs != 250_000 || rec.TestID != test || !strings.HasSuffix(upload.Request.URL, rec.Asset.ObjectName) {
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
	list, err := s.Recordings(ctx, viewer, test)
	if err != nil || len(list) != 2 || list[0].ID != rec.ID || list[1].Asset.Status != domain.Rejected {
		t.Fatalf("recordings: %+v %v", list, err)
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

	if _, err := pool.Exec(ctx, "UPDATE misko.tests SET status = 'CANCELLED', cancelled_at = now(), cancel_reason = 'duplicate' WHERE id = $1", foreign); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartUpload(ctx, technician, foreign, request(10)); !errors.Is(err, application.ErrTestCancelled) {
		t.Fatalf("upload to a cancelled test: %v", err)
	}
}

func second[T any](_ T, err error) error { return err }
