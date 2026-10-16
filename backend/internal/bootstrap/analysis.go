package bootstrap

import (
	"context"
	"errors"
	analysiscatalog "github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/catalog"
	analysispostgres "github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/postgres"
	analysistoken "github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/token"
	analysisapp "github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"time"
)

const (
	// analysisLease is how long a claimed run stays with a worker without a heartbeat.
	analysisLease       = 5 * time.Minute
	analysisMaxAttempts = 3
	// analysisMaxTrajectoryBytes caps the trajectory read to compute metrics;
	// about four hours of 30 fps samples.
	analysisMaxTrajectoryBytes = 64 << 20
)

// analysisObjects gives the analysis use cases the video object store.
type analysisObjects struct {
	objects mediaapp.ObjectStore
}

func (a analysisObjects) Bucket() string { return a.objects.Bucket() }

func (a analysisObjects) UploadURL(ctx context.Context, object, contentType string, expires time.Time) (analysisapp.SignedRequest, error) {
	r, err := a.objects.UploadURL(ctx, object, contentType, expires)
	return analysisapp.SignedRequest(r), err
}

func (a analysisObjects) ReadURL(ctx context.Context, object string, generation int64, expires time.Time) (string, error) {
	return a.objects.ReadURL(ctx, object, generation, expires)
}

func (a analysisObjects) Attrs(ctx context.Context, object string) (analysisapp.ObjectAttrs, error) {
	attrs, err := a.objects.Attrs(ctx, object)
	if errors.Is(err, mediaapp.ErrObjectNotFound) {
		return analysisapp.ObjectAttrs{}, analysisapp.ErrObjectNotFound
	}
	return analysisapp.ObjectAttrs(attrs), err
}

func (a analysisObjects) Read(ctx context.Context, object string, generation, limit int64) ([]byte, error) {
	data, err := a.objects.Read(ctx, object, generation, limit)
	if errors.Is(err, mediaapp.ErrObjectNotFound) {
		return nil, analysisapp.ErrObjectNotFound
	}
	return data, err
}

func newAnalysis(pool *pgxpool.Pool, objects mediaapp.ObjectStore, settings mediaapp.Settings) *analysisapp.Service {
	var store analysisapp.ObjectStore
	if objects != nil {
		store = analysisObjects{objects}
	}
	return analysisapp.New(analysispostgres.NewStore(pool), store, analysiscatalog.New(), analysistoken.New(), analysisapp.Settings{
		Lease: analysisLease, MaxAttempts: analysisMaxAttempts, UploadTTL: settings.UploadTTL, ReadTTL: settings.ReadTTL, MaxOutputBytes: settings.MaxBytes,
		MaxTrajectoryBytes: analysisMaxTrajectoryBytes,
	}, time.Now)
}

// RunScheduler ticks the analysis scheduler until ctx ends: it returns expired
// leases to the queue and enqueues one run per ready animal of a test. Several
// schedulers may run because enqueueing is idempotent and expired leases are
// taken with SKIP LOCKED. The scheduler signs no URLs, so it needs no storage
// configuration.
func RunScheduler(ctx context.Context, pool *pgxpool.Pool, interval time.Duration, logger *slog.Logger) {
	service := newAnalysis(pool, nil, mediaapp.Settings{})
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		report, err := service.Tick(ctx)
		logTick(ctx, logger, report, err)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// logTick logs a tick's outcome. Candidates the tick skipped are logged one by
// one; they never stop the others from being enqueued.
func logTick(ctx context.Context, logger *slog.Logger, report analysisapp.TickReport, err error) {
	if err != nil {
		if ctx.Err() == nil {
			logger.ErrorContext(ctx, "analysis scheduler tick failed", "error", err)
		}
		return
	}
	for _, skipped := range report.Skipped {
		logger.WarnContext(ctx, "analysis scheduler skipped a candidate", "error", skipped)
	}
	if report.Enqueued+report.Requeued+report.Failed > 0 {
		logger.InfoContext(ctx, "analysis scheduler tick", "enqueued", report.Enqueued, "requeued", report.Requeued, "failed", report.Failed,
			"skipped", len(report.Skipped))
	}
}
