package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	analysiscatalog "github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/catalog"
	analysispostgres "github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/postgres"
	analysistoken "github.com/HappyHackingSpace/Misko/backend/internal/analysis/adapters/token"
	analysisapp "github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	analysisdomain "github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	calibrationcatalog "github.com/HappyHackingSpace/Misko/backend/internal/calibration/adapters/catalog"
	calibrationpostgres "github.com/HappyHackingSpace/Misko/backend/internal/calibration/adapters/postgres"
	calibrationapp "github.com/HappyHackingSpace/Misko/backend/internal/calibration/application"
	calibrationdomain "github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	environmentscatalog "github.com/HappyHackingSpace/Misko/backend/internal/environments/adapters/catalog"
	environmentspostgres "github.com/HappyHackingSpace/Misko/backend/internal/environments/adapters/postgres"
	environmentsapp "github.com/HappyHackingSpace/Misko/backend/internal/environments/application"
	mediapostgres "github.com/HappyHackingSpace/Misko/backend/internal/media/adapters/postgres"
	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	mediadomain "github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	subjectanalysiscatalog "github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/adapters/catalog"
	subjectanalysispostgres "github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/adapters/postgres"
	subjectanalysisapp "github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/application"
	trialscatalog "github.com/HappyHackingSpace/Misko/backend/internal/trials/adapters/catalog"
	trialspostgres "github.com/HappyHackingSpace/Misko/backend/internal/trials/adapters/postgres"
	trialsapp "github.com/HappyHackingSpace/Misko/backend/internal/trials/application"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// analysisLease is how long a claimed job stays with a worker without a heartbeat.
	analysisLease       = 5 * time.Minute
	analysisMaxAttempts = 3
	// analysisMaxTrajectoryBytes caps the trajectory read to compute metrics;
	// about four hours of 30 fps samples.
	analysisMaxTrajectoryBytes = 64 << 20
)

// modules are the use cases that take part in analysis, wired through their
// ports: the analysis service, the experiment side and paradigm trials, with
// the media, calibration and environments modules they read from.
type modules struct {
	media           *mediaapp.Service
	calibration     *calibrationapp.Service
	environments    *environmentsapp.Service
	analysis        *analysisapp.Service
	subjectAnalysis *subjectanalysisapp.Service
	trials          *trialsapp.Service
}

// newModules wires the analysis service and its requesters. A nil objects
// store leaves uploads, claims and signed reads unconfigured, which the
// scheduler does not need.
func newModules(pool *pgxpool.Pool, objects mediaapp.ObjectStore, settings mediaapp.Settings) modules {
	m := modules{
		media:        mediaapp.New(mediapostgres.NewStore(pool), objects, settings, time.Now),
		calibration:  calibrationapp.New(calibrationpostgres.NewStore(pool), calibrationcatalog.New(), time.Now),
		environments: environmentsapp.New(environmentspostgres.NewStore(pool), environmentscatalog.New()),
	}
	// The listener reaches the experiment side, which is built on the
	// analysis service; it is set before any job can succeed.
	listener := &succeededListener{}
	var analysisObjectStore analysisapp.ObjectStore
	if objects != nil {
		analysisObjectStore = analysisObjects{objects}
	}
	m.analysis = analysisapp.New(analysisapp.Dependencies{
		Store: analysispostgres.NewStore(pool), Objects: analysisObjectStore, Sources: analysisSources{m.media},
		Calibrations: analysisCalibrations{m.calibration}, Catalog: analysiscatalog.New(), Tokens: analysistoken.New(), Listener: listener,
	}, analysisapp.Settings{
		Lease: analysisLease, MaxAttempts: analysisMaxAttempts, UploadTTL: settings.UploadTTL, ReadTTL: settings.ReadTTL, MaxOutputBytes: settings.MaxBytes,
		MaxTrajectoryBytes: analysisMaxTrajectoryBytes,
	}, time.Now)
	m.subjectAnalysis = subjectanalysisapp.New(subjectanalysispostgres.NewStore(pool), subjectAnalysisPort{m.analysis}, subjectanalysiscatalog.New(), time.Now)
	listener.subjects = m.subjectAnalysis
	m.trials = trialsapp.New(trialsapp.Dependencies{
		Store: trialspostgres.NewStore(pool), Analysis: trialsAnalysisPort{m.analysis}, Videos: trialsVideos{m.media},
		Calibrations: trialsCalibrations{m.calibration}, Environments: trialsEnvironments{m.environments}, Catalog: trialscatalog.New(),
	}, time.Now)
	return m
}

// TickReport is what one scheduler tick did across the modules.
type TickReport struct {
	Analysis        analysisapp.TickReport
	SubjectAnalysis subjectanalysisapp.TickReport
	Trials          trialsapp.TickReport
}

// tick expires leases, submits ready animals of tests and ready trials, and
// opens results the analysis service could not report.
func (m modules) tick(ctx context.Context) (TickReport, error) {
	var report TickReport
	var err error
	if report.Analysis, err = m.analysis.Tick(ctx); err != nil {
		return report, fmt.Errorf("analysis tick: %w", err)
	}
	if report.SubjectAnalysis, err = m.subjectAnalysis.Tick(ctx); err != nil {
		return report, fmt.Errorf("test subject analysis tick: %w", err)
	}
	if report.Trials, err = m.trials.Tick(ctx); err != nil {
		return report, fmt.Errorf("trial analysis tick: %w", err)
	}
	return report, nil
}

// RunScheduler ticks until ctx ends: it returns expired leases to the queue,
// submits one automatic job per ready animal of a test and per ready trial
// video, and opens the results of succeeded jobs. Several schedulers may run
// because submissions are idempotent and expired leases are taken with SKIP
// LOCKED. The scheduler signs no URLs, so it needs no storage configuration.
func RunScheduler(ctx context.Context, pool *pgxpool.Pool, interval time.Duration, logger *slog.Logger) {
	m := newModules(pool, nil, mediaapp.Settings{})
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		report, err := m.tick(ctx)
		logTick(ctx, logger, report, err)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// logTick logs a tick's outcome. Candidates a tick skipped are logged one by
// one; they never stop the others from being submitted.
func logTick(ctx context.Context, logger *slog.Logger, r TickReport, err error) {
	if err != nil {
		if ctx.Err() == nil {
			logger.ErrorContext(ctx, "analysis scheduler tick failed", "error", err)
		}
		return
	}
	for _, skipped := range append(r.SubjectAnalysis.Skipped, r.Trials.Skipped...) {
		logger.WarnContext(ctx, "analysis scheduler skipped a candidate", "error", skipped)
	}
	if r.Analysis.Requeued+r.Analysis.Failed+r.SubjectAnalysis.Submitted+r.SubjectAnalysis.Opened+r.Trials.Submitted > 0 {
		logger.InfoContext(ctx, "analysis scheduler tick", "requeued", r.Analysis.Requeued, "failed", r.Analysis.Failed,
			"submitted", r.SubjectAnalysis.Submitted, "opened", r.SubjectAnalysis.Opened, "trialsSubmitted", r.Trials.Submitted,
			"skipped", len(r.SubjectAnalysis.Skipped)+len(r.Trials.Skipped))
	}
}

// analysisObjects gives the analysis service the video object store.
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

// analysisSources reads source videos through the media module's asset port.
type analysisSources struct{ media *mediaapp.Service }

func (a analysisSources) Source(ctx context.Context, id string) (analysisdomain.SourceVideo, error) {
	asset, err := a.media.Asset(ctx, id)
	if errors.Is(err, mediaapp.ErrAssetNotFound) {
		return analysisdomain.SourceVideo{}, analysisapp.ErrSourceNotFound
	}
	if err != nil {
		return analysisdomain.SourceVideo{}, err
	}
	return analysisdomain.SourceVideo{ID: asset.ID, ObjectName: asset.ObjectName, Verified: asset.Status == mediadomain.Verified,
		Generation: asset.Generation, CRC32C: asset.CRC32C}, nil
}

// analysisCalibrations reads the calibration a job snapshots through the
// calibration module's port. A rejected calibration cannot be pinned.
type analysisCalibrations struct{ calibration *calibrationapp.Service }

func (a analysisCalibrations) Calibration(ctx context.Context, id string) (analysisdomain.Calibration, error) {
	c, err := a.calibration.Snapshot(ctx, id)
	if errors.Is(err, calibrationapp.ErrCalibrationNotFound) {
		return analysisdomain.Calibration{}, analysisapp.ErrCalibrationNotFound
	}
	if err != nil {
		return analysisdomain.Calibration{}, err
	}
	if c.Status != calibrationdomain.Valid {
		return analysisdomain.Calibration{}, fmt.Errorf("%w: calibration %s was rejected", analysisdomain.ErrCalibrationRequired, id)
	}
	out := analysisdomain.Calibration{ID: c.ID, FrameWidth: c.FrameWidth, FrameHeight: c.FrameHeight,
		Crop: analysisdomain.Rect{X: c.Crop.X, Y: c.Crop.Y, Width: c.Crop.Width, Height: c.Crop.Height}, Plane: string(c.Plane), Transform: c.Transform}
	for _, z := range c.Zones {
		out.Zones = append(out.Zones, analysisdomain.Zone{Key: z.Key, Pixels: shape(z.Pixels), Centimeters: shape(z.Centimeters)})
	}
	return out, nil
}

func shape(s calibrationdomain.Shape) analysisdomain.Shape {
	var out analysisdomain.Shape
	if s.Circle != nil {
		out.Circle = &analysisdomain.Circle{X: s.Circle.X, Y: s.Circle.Y, Radius: s.Circle.Radius}
	}
	for _, v := range s.Polygon {
		out.Polygon = append(out.Polygon, analysisdomain.Vertex{X: v.X, Y: v.Y})
	}
	return out
}

// succeededListener tells the experiment side about succeeded jobs of
// animals of tests; trial jobs need no notice.
type succeededListener struct {
	subjects *subjectanalysisapp.Service
}

func (l *succeededListener) JobSucceeded(ctx context.Context, job analysisdomain.Job) error {
	if job.Requester.Kind != analysisdomain.RequesterTestSubject || l.subjects == nil {
		return nil
	}
	return l.subjects.JobSucceeded(ctx, job.ID, job.Requester.ID)
}

// refused reports whether the analysis service refused a request's inputs,
// which a requester answers as "not ready".
func refused(err error) bool {
	for _, target := range []error{analysisdomain.ErrInvalidRequest, analysisdomain.ErrNotAnalysable, analysisdomain.ErrSourceNotVerified,
		analysisdomain.ErrCalibrationRequired, analysisdomain.ErrMissingZones, analysisdomain.ErrInvalidCalibration,
		analysisapp.ErrSourceNotFound, analysisapp.ErrCalibrationNotFound} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
