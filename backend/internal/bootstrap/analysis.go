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

func newAnalysis(pool *pgxpool.Pool, objects mediaapp.ObjectStore, settings mediaapp.Settings) *analysisapp.Service {
	var store analysisapp.ObjectStore
	if objects != nil {
		store = analysisObjects{objects}
	}
	return analysisapp.New(analysispostgres.NewStore(pool), store, analysiscatalog.New(), analysistoken.New(), analysisapp.Settings{
		Lease: analysisLease, MaxAttempts: analysisMaxAttempts, UploadTTL: settings.UploadTTL, ReadTTL: settings.ReadTTL, MaxOutputBytes: settings.MaxBytes,
	}, time.Now)
}

// RunScheduler ticks the analysis scheduler until ctx ends: it returns expired
// leases to the queue and enqueues ready recordings. Several schedulers may run
// because enqueueing is idempotent and expired leases are taken with SKIP LOCKED.
// The scheduler signs no URLs, so it needs no storage configuration.
func RunScheduler(ctx context.Context, pool *pgxpool.Pool, interval time.Duration, logger *slog.Logger) {
	service := newAnalysis(pool, nil, mediaapp.Settings{})
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		report, err := service.Tick(ctx)
		if err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "analysis scheduler tick failed", "error", err)
		} else if report != (analysisapp.TickReport{}) {
			logger.InfoContext(ctx, "analysis scheduler tick", "enqueued", report.Enqueued, "requeued", report.Requeued, "failed", report.Failed)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
