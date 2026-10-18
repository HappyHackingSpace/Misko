//go:build integration

package postgres_test

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ctx = context.Background()

const author = "01a00000-0000-7000-8000-000000000001"

func id(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var out string
	if err := pool.QueryRow(ctx, sql, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// queued is a job of a requester on a verified video; the service only
// needs the video asset, nothing of the requester.
func queued(t *testing.T, pool *pgxpool.Pool, kind domain.RequesterKind, requester, key string, at time.Time) domain.Job {
	t.Helper()
	asset := id(t, pool, `INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ORIGINAL', 'misko-test', 'videos/' || gen_random_uuid(), 'video/mp4', 10, 1, 'VERIFIED', 9, now(), $1) RETURNING id`, author)
	end := int64(6_000_000)
	return domain.Job{
		Input: domain.Input{Requester: domain.Requester{Kind: kind, ID: requester}, IdempotencyKey: key, ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1,
			MetricEngineVersion: 1, ResultSchemaVersion: 1, Parameters: map[string]float64{"min_bout_s": 0.5},
			Source: domain.Source{AssetID: asset, Generation: 9, CRC32C: 1, ClipEndUs: &end},
			Calibration: &domain.Calibration{ID: "01a00000-0000-7000-8000-0000000000ca", FrameWidth: 640, FrameHeight: 480,
				Crop: domain.Rect{X: 10, Y: 10, Width: 300, Height: 300}, Plane: "ARENA_FLOOR", Transform: [9]float64{0.1, 0, 0, 0, 0.1, 0, 0, 0, 1},
				Zones: []domain.Zone{
					{Key: "novel_object", Pixels: domain.Shape{Circle: &domain.Circle{X: 100, Y: 100, Radius: 20}}, Centimeters: domain.Shape{Circle: &domain.Circle{X: 10, Y: 10, Radius: 2}}},
					{Key: "familiar_object", Pixels: domain.Shape{Polygon: []domain.Vertex{{X: 1, Y: 1}, {X: 5, Y: 1}, {X: 5, Y: 5}}},
						Centimeters: domain.Shape{Polygon: []domain.Vertex{{X: 0.1, Y: 0.1}, {X: 0.5, Y: 0.1}, {X: 0.5, Y: 0.5}}}},
				}},
			Trigger: domain.Automatic},
		Status: domain.Queued, MaxAttempts: 2, AvailableAt: at,
	}
}

func worker(t *testing.T, pool *pgxpool.Pool, store *postgres.Store) domain.Worker {
	t.Helper()
	hash := make([]byte, 32)
	_, _ = rand.Read(hash)
	w, err := store.CreateWorker(ctx, domain.Worker{Name: "tracker-" + rand.Text()[:6], ModelVersion: "m 1", CreatedBy: author,
		Capabilities: []domain.Capability{{ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1}}}, hash)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func constraint(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

// The jobs table is the queue: an idempotency key creates one job, the
// snapshot round-trips, claims follow capabilities and availability, and
// only the current attempt of a running job writes outputs.
func TestJobsQueueSnapshotsAndFencing(t *testing.T) {
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	trial := "01a00000-0000-7000-8000-0000000000f1"

	first, err := store.CreateJob(ctx, queued(t, pool, domain.RequesterTrial, trial, "trial:x", now.Add(-time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.CreateJob(ctx, queued(t, pool, domain.RequesterTrial, trial, "trial:x", now))
	if err != nil || again.ID != first.ID {
		t.Fatalf("an idempotency key creates one job: %s %s %v", first.ID, again.ID, err)
	}
	c := first.Calibration
	if c == nil || c.ID != "01a00000-0000-7000-8000-0000000000ca" || c.Crop.Width != 300 || c.Transform[0] != 0.1 || len(c.Zones) != 2 ||
		c.Zones[0].Centimeters.Circle.Radius != 2 || len(c.Zones[1].Pixels.Polygon) != 3 || first.Parameters["min_bout_s"] != 0.5 {
		t.Fatalf("snapshot: %+v", first)
	}
	free := queued(t, pool, domain.RequesterTestSubject, "01a00000-0000-7000-8000-0000000000f2", "", now.Add(time.Hour))
	free.Calibration = nil
	later, err := store.CreateJob(ctx, free)
	if err != nil || later.Calibration != nil {
		t.Fatalf("job without calibration: %+v %v", later, err)
	}
	if jobs, err := store.JobsByRequester(ctx, domain.RequesterTrial, []string{trial, "not-a-uuid"}); err != nil || len(jobs) != 1 || jobs[0].ID != first.ID {
		t.Fatalf("by requester: %+v %v", jobs, err)
	}
	if jobs, err := store.Jobs(ctx, []string{first.ID, later.ID}); err != nil || len(jobs) != 2 || jobs[0].ID != later.ID {
		t.Fatalf("jobs newest first: %+v %v", jobs, err)
	}

	w := worker(t, pool, store)
	if _, err := store.ClaimNext(ctx, []domain.Capability{{ParadigmKey: "MWM", ParadigmVersion: 1}}, now); !errors.Is(err, application.ErrNoJob) {
		t.Fatalf("claim without capability: %v", err)
	}
	next, err := store.ClaimNext(ctx, w.Capabilities, now)
	if err != nil || next.ID != first.ID {
		t.Fatalf("claim the available job: %+v %v", next, err)
	}
	running, _ := next.Claim(w.ID, now, time.Minute)
	if running, err = store.UpdateJob(ctx, running); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimNext(ctx, w.Capabilities, now); !errors.Is(err, application.ErrNoJob) {
		t.Fatalf("the later job is not yet available: %v", err)
	}

	// Outputs need the current running attempt; the snapshot never changes.
	upload := application.OutputUpload{JobID: running.ID, Attempt: 2, ObjectName: running.OutputPrefix() + "o.mp4", Kind: domain.AnalyzedVideo,
		ContentType: "video/mp4", SizeBytes: 10, CRC32C: 1}
	if err := store.CreateOutputUpload(ctx, upload); !errors.Is(err, domain.ErrStaleAttempt) {
		t.Fatalf("an old attempt's output: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE misko.analysis_jobs SET parameters = '{}' WHERE id = $1`, running.ID); constraint(err) != "analysis_jobs_transition" {
		t.Fatalf("snapshot changed: %v", err)
	}

	// Success needs the pair in the same transaction.
	done, _ := running.Succeed(w.ID, 1, "m 1", now)
	if err := store.Transaction(ctx, func(tx application.Store) error { _, err := tx.UpdateJob(ctx, done); return err }); constraint(err) != "analysis_jobs_published" {
		t.Fatalf("success without a pair: %v", err)
	}
	upload.Attempt = 1
	video := application.VerifiedOutput{OutputUpload: upload, Bucket: "misko-test", Generation: 4}
	value := 0.4
	result := domain.Result{Pair: domain.PairInput{AnalyzedObjectName: upload.ObjectName, TimeMappingVersion: "identity-v1"},
		Metrics: []domain.MetricValue{{Key: "discrimination_index", Unit: "ratio", Value: &value}},
		Events:  []domain.Event{{Type: "exploration", Label: "novel", Kind: "INTERVAL", StartUs: 0, EndUs: 10, Confidence: 0.9}}}
	err = store.Transaction(ctx, func(tx application.Store) error {
		if err := tx.CreateOutputUpload(ctx, upload); err != nil {
			return err
		}
		if err := tx.Publish(ctx, running, result, []application.VerifiedOutput{video}, domain.Quality{Samples: 4, Tracked: 3, LostRatio: 0.25, MeanConfidence: 0.9}); err != nil {
			return err
		}
		_, err := tx.UpdateJob(ctx, done)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := store.Published(ctx, []string{running.ID, later.ID})
	p := published[running.ID]
	if err != nil || len(p.Metrics) != 1 || len(p.Events) != 1 || p.Events[0].Label != "novel" || p.Pair == nil || p.Pair.AnalyzedGeneration != 4 ||
		p.QC == nil || !p.QC.Passed || p.QC.Tracked != 3 || len(published) != 1 {
		t.Fatalf("published: %+v %v", published, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM misko.analysis_jobs WHERE id = $1`, running.ID); constraint(err) != "analysis_jobs_transition" {
		t.Fatalf("finished job deleted: %v", err)
	}
}

// Expired leases are taken once even when two schedulers tick together.
func TestExpiredLeasesAreLockedOnce(t *testing.T) {
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	job, err := store.CreateJob(ctx, queued(t, pool, domain.RequesterTrial, "01a00000-0000-7000-8000-0000000000f3", "", now.Add(-time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	w := worker(t, pool, store)
	claimed, _ := job.Claim(w.ID, now.Add(-time.Hour), time.Minute)
	if _, err := store.UpdateJob(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	err = store.Transaction(ctx, func(tx application.Store) error {
		locked, err := tx.LockExpiredJobs(ctx, now)
		if err != nil || len(locked) != 1 {
			t.Fatalf("first scheduler: %v %v", locked, err)
		}
		other, err := postgres.NewStore(pool).LockExpiredJobs(ctx, now)
		if err != nil || len(other) != 0 {
			t.Fatalf("second scheduler sees a locked lease: %v %v", other, err)
		}
		next, _ := locked[0].Expire(now)
		_, err = tx.UpdateJob(ctx, next)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if again, err := store.Job(ctx, job.ID); err != nil || again.Status != domain.Queued || again.Attempt != 1 {
		t.Fatalf("requeued: %+v %v", again, err)
	}
}
