// Package application contains the analysis service's use cases.
//
// Other modules use the in-process port (Submit, Get, Jobs, ByRequester,
// Results): they authorize their own users and pass an opaque requester.
// Workers authenticate with their own token and act only on jobs they
// claimed. Users manage workers with lab:configure and read jobs, their
// results and their videos with *:read.
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
)

var (
	ErrWorkerUnauthenticated = errors.New("worker authentication required")
	ErrWorkerNotFound        = errors.New("worker not found")
	ErrWorkerNameTaken       = errors.New("worker name is already in use")
	ErrUnknownCapability     = errors.New("capability names a paradigm version outside the catalog or one that cannot be computed from a trajectory")
	ErrJobNotFound           = errors.New("analysis job not found")
	ErrSourceNotFound        = errors.New("source video not found")
	ErrCalibrationNotFound   = errors.New("calibration not found")
	ErrNoJob                 = errors.New("no queued job matches the worker's capabilities")
	ErrUnknownOutput         = errors.New("artifacts must name outputs requested for this attempt")
	ErrOutputNotVerified     = errors.New("an output object is missing or differs from its declared size, checksum or content type")
	ErrJobNotSucceeded       = errors.New("the job has no published result")
	ErrObjectNotFound        = errors.New("stored object not found")
	ErrInvalidOutput         = errors.New("outputs need a known kind, a video or data content type, a size within the limit and a base64 CRC32C")
	ErrStorageNotConfigured  = errors.New("video storage is not configured")
)

type SignedRequest struct {
	Method    string
	URL       string
	Headers   map[string]string
	ExpiresAt time.Time
}

type ObjectAttrs struct {
	Generation  int64
	Size        int64
	CRC32C      uint32
	ContentType string
}

// ObjectStore is private object storage for source and output files.
type ObjectStore interface {
	Bucket() string
	UploadURL(ctx context.Context, object, contentType string, expires time.Time) (SignedRequest, error)
	ReadURL(ctx context.Context, object string, generation int64, expires time.Time) (string, error)
	// Attrs returns ErrObjectNotFound when the object does not exist.
	Attrs(ctx context.Context, object string) (ObjectAttrs, error)
	// Read returns one object generation of at most limit bytes, or
	// ErrObjectNotFound.
	Read(ctx context.Context, object string, generation, limit int64) ([]byte, error)
}

// Sources reads source videos from the video store.
type Sources interface {
	// Source returns ErrSourceNotFound for an unknown asset.
	Source(ctx context.Context, assetID string) (domain.SourceVideo, error)
}

// Calibrations reads the calibration a job snapshots.
type Calibrations interface {
	// Calibration returns ErrCalibrationNotFound for an unknown calibration
	// and domain.ErrCalibrationRequired for one that was rejected.
	Calibration(ctx context.Context, id string) (domain.Calibration, error)
}

// Listener hears about jobs that succeeded, after the result is stored. It
// may be told twice about the same job and must be idempotent.
type Listener interface {
	JobSucceeded(ctx context.Context, job domain.Job) error
}

// Catalog reads paradigm contracts and runs the metric engine.
type Catalog interface {
	// Contract returns ErrUnknownCapability for versions outside the catalog.
	Contract(key string, version int) (domain.Contract, error)
	// Evaluate computes metrics and events from a trajectory with pinned
	// parameters and the snapshot's zones; samples the engine rejects are
	// domain.ErrInvalidTrajectory.
	Evaluate(key string, version int, parameters map[string]float64, t domain.Track, zones []domain.Zone, durationUs int64) (domain.Computed, error)
}

// Tokens issues worker tokens and hashes them for storage and lookup.
type Tokens interface {
	New() (token string, hash []byte, err error)
	Hash(token string) []byte
}

type OutputUpload struct {
	JobID       string
	Attempt     int
	ObjectName  string
	Kind        string
	ContentType string
	SizeBytes   int64
	CRC32C      uint32
}

// VerifiedOutput is an output whose stored object matched its declaration.
type VerifiedOutput struct {
	OutputUpload
	Bucket     string
	Generation int64
}

// QC is the tracking quality of the trajectory a job was measured from and
// whether the paradigm's quality rules passed.
type QC struct {
	domain.Quality
	Passed bool
}

// Published is the stored result of a job. Metrics, events, outputs and the
// pair exist once it succeeded; QC also when quality rules failed it.
type Published struct {
	Metrics   []domain.MetricValue
	Events    []domain.Event
	Artifacts []VerifiedOutput
	Pair      *Pair
	QC        *QC
}

// Pair maps the analysed video onto the source generation it was made from.
type Pair struct {
	AnalyzedObjectName             string
	AnalyzedGeneration             int64
	SourceOffsetUs, OutputOffsetUs int64
	TimeMappingVersion             string
}

type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	CreateWorker(ctx context.Context, w domain.Worker, tokenHash []byte) (domain.Worker, error)
	Workers(ctx context.Context) ([]domain.Worker, error)
	DisableWorker(ctx context.Context, id string, at time.Time) (domain.Worker, error)
	// WorkerByToken returns ErrWorkerUnauthenticated for unknown or disabled workers.
	WorkerByToken(ctx context.Context, tokenHash []byte) (domain.Worker, error)
	ActiveCapabilities(ctx context.Context) ([]domain.Capability, error)
	// CreateJob stores a queued job. When a job with the same idempotency key
	// exists, that job is returned instead.
	CreateJob(ctx context.Context, j domain.Job) (domain.Job, error)
	LockExpiredJobs(ctx context.Context, now time.Time) ([]domain.Job, error)
	// ClaimNext locks the oldest available queued job for one of the
	// capabilities, skipping jobs locked by others, or returns ErrNoJob.
	ClaimNext(ctx context.Context, capabilities []domain.Capability, now time.Time) (domain.Job, error)
	UpdateJob(ctx context.Context, j domain.Job) (domain.Job, error)
	// Job returns ErrJobNotFound for an unknown id.
	Job(ctx context.Context, id string) (domain.Job, error)
	LockJob(ctx context.Context, id string) (domain.Job, error)
	// Jobs returns the known jobs among ids, newest first.
	Jobs(ctx context.Context, ids []string) ([]domain.Job, error)
	// JobsByRequester returns the jobs of the requesters, newest first.
	JobsByRequester(ctx context.Context, kind domain.RequesterKind, ids []string) ([]domain.Job, error)
	CreateOutputUpload(ctx context.Context, u OutputUpload) error
	OutputUploads(ctx context.Context, jobID string, attempt int) ([]OutputUpload, error)
	// Publish stores the result, outputs, pair and QC in the caller's transaction.
	Publish(ctx context.Context, j domain.Job, result domain.Result, outputs []VerifiedOutput, quality domain.Quality) error
	// RecordQC stores the quality of an attempt that QC failed.
	RecordQC(ctx context.Context, j domain.Job, qc QC) error
	// Published returns the stored results of the jobs by id.
	Published(ctx context.Context, jobIDs []string) (map[string]Published, error)
}

type Settings struct {
	Lease              time.Duration
	MaxAttempts        int
	UploadTTL, ReadTTL time.Duration
	MaxOutputBytes     int64
	// MaxTrajectoryBytes caps the trajectory the API reads into memory.
	MaxTrajectoryBytes int64
}

// Dependencies are the ports the service uses. A nil Objects store disables
// claiming, outputs and signed reads; a nil Listener hears nothing.
type Dependencies struct {
	Store        Store
	Objects      ObjectStore
	Sources      Sources
	Calibrations Calibrations
	Catalog      Catalog
	Tokens       Tokens
	Listener     Listener
}

type Service struct {
	store        Store
	objects      ObjectStore
	sources      Sources
	calibrations Calibrations
	catalog      Catalog
	tokens       Tokens
	listener     Listener
	settings     Settings
	now          func() time.Time
}

func New(deps Dependencies, settings Settings, now func() time.Time) *Service {
	return &Service{store: deps.Store, objects: deps.Objects, sources: deps.Sources, calibrations: deps.Calibrations, catalog: deps.Catalog,
		tokens: deps.Tokens, listener: deps.Listener, settings: settings, now: now}
}

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

type WorkerInput struct {
	Name, ModelVersion string
	Capabilities       []domain.Capability
}

type Registered struct {
	Worker domain.Worker
	// Token is shown once; only its hash is stored.
	Token string
}

func (s *Service) RegisterWorker(ctx context.Context, actor access.Actor, in WorkerInput) (Registered, error) {
	if err := actor.Require(access.LabConfigure); err != nil {
		return Registered{}, err
	}
	w, err := domain.NewWorker(in.Name, in.ModelVersion, in.Capabilities)
	if err != nil {
		return Registered{}, err
	}
	for _, c := range w.Capabilities {
		// Catalog presence is not analysis support: a worker may only claim
		// versions whose metrics the engine can compute from its trajectory.
		if contract, err := s.catalog.Contract(c.ParadigmKey, c.ParadigmVersion); err != nil || !contract.TrajectoryAnalyzable {
			return Registered{}, fmt.Errorf("%w: %s v%d", ErrUnknownCapability, c.ParadigmKey, c.ParadigmVersion)
		}
	}
	token, hash, err := s.tokens.New()
	if err != nil {
		return Registered{}, fmt.Errorf("issue worker token: %w", err)
	}
	w.CreatedBy = actor.UserID
	created, err := s.store.CreateWorker(ctx, w, hash)
	if err != nil {
		return Registered{}, err
	}
	return Registered{Worker: created, Token: token}, nil
}

func (s *Service) Workers(ctx context.Context, actor access.Actor) ([]domain.Worker, error) {
	if err := actor.Require(access.LabConfigure); err != nil {
		return nil, err
	}
	return s.store.Workers(ctx)
}

// DisableWorker revokes a worker token. Its running leases expire normally.
func (s *Service) DisableWorker(ctx context.Context, actor access.Actor, id string) (domain.Worker, error) {
	if err := actor.Require(access.LabConfigure); err != nil {
		return domain.Worker{}, err
	}
	return s.store.DisableWorker(ctx, id, s.clock())
}

func (s *Service) Capabilities(ctx context.Context, actor access.Actor) ([]domain.Capability, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	return s.store.ActiveCapabilities(ctx)
}

func (s *Service) AuthenticateWorker(ctx context.Context, token string) (domain.Worker, error) {
	if strings.TrimSpace(token) == "" {
		return domain.Worker{}, ErrWorkerUnauthenticated
	}
	return s.store.WorkerByToken(ctx, s.tokens.Hash(token))
}

// TickReport counts what a scheduler tick did with expired leases.
type TickReport struct {
	Requeued, Failed int
}

// Tick returns jobs whose lease expired to the queue, or fails them after
// their last attempt. Queued jobs wait for a capable worker to claim them;
// several schedulers may tick because expired leases are taken with SKIP
// LOCKED.
func (s *Service) Tick(ctx context.Context) (TickReport, error) {
	var report TickReport
	now := s.clock()
	err := s.store.Transaction(ctx, func(tx Store) error {
		expired, err := tx.LockExpiredJobs(ctx, now)
		if err != nil {
			return err
		}
		for _, j := range expired {
			next, changed := j.Expire(now)
			if !changed {
				continue
			}
			if _, err := tx.UpdateJob(ctx, next); err != nil {
				return err
			}
			if next.Status == domain.Failed {
				report.Failed++
			} else {
				report.Requeued++
			}
		}
		return nil
	})
	if err != nil {
		return TickReport{}, err
	}
	return report, nil
}
