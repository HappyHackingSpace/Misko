// Package application contains analysis use cases. Workers authenticate with
// their own token and act only on runs they claimed; users read runs and
// results with *:read, start reanalysis and review results with test:run and
// manage workers with lab:configure. A run analyses one animal of a test (a
// test subject) in the test's current video.
package application

import (
	"context"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"strings"
	"time"
)

var (
	ErrWorkerUnauthenticated = errors.New("worker authentication required")
	ErrWorkerNotFound        = errors.New("worker not found")
	ErrWorkerNameTaken       = errors.New("worker name is already in use")
	ErrUnknownCapability     = errors.New("capability names a paradigm version outside the catalog or one that cannot be computed from a trajectory")
	ErrRunNotFound           = errors.New("analysis run not found")
	ErrTestSubjectNotFound   = errors.New("test subject not found")
	ErrNotReady              = errors.New("the animal needs a verified current video of a test that is not cancelled, a running or completed experiment, no exclusion, a valid calibration of its arena when the paradigm requires one and an active worker capability")
	ErrRunPending            = errors.New("an analysis of this animal is already queued or running")
	ErrNoRun                 = errors.New("no queued run matches the worker's capabilities")
	ErrUnknownOutput         = errors.New("artifacts must name outputs requested for this attempt")
	ErrOutputNotVerified     = errors.New("an output object is missing or differs from its declared size, checksum or content type")
	ErrRunNotSucceeded       = errors.New("the run has no published result")
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

// Catalog reads paradigm contracts and runs the metric engine.
type Catalog interface {
	// Contract returns ErrUnknownCapability for versions outside the catalog.
	Contract(key string, version int) (domain.Contract, error)
	// Evaluate computes metrics and events from a trajectory with pinned
	// parameters; samples the engine rejects are domain.ErrInvalidTrajectory.
	Evaluate(key string, version int, parameters map[string]float64, t domain.Track, durationUs int64) (domain.Computed, error)
}

// Tokens issues worker tokens and hashes them for storage and lookup.
type Tokens interface {
	New() (token string, hash []byte, err error)
	Hash(token string) []byte
}

// Candidate is one animal of a test that may be analysed in the test's
// current video. RecordingID is empty while the test has no video and
// CalibrationID is empty while the animal's arena has no valid effective
// calibration (its own override for this video, else the arena default).
type Candidate struct {
	ExperimentID, TestID, TestSubjectID, RecordingID, SourceAssetID string
	SourceGeneration                                                int64
	SourceCRC32C                                                    uint32
	ClipStartUs                                                     int64
	ClipEndUs                                                       *int64
	CalibrationID                                                   string
	ParadigmKey                                                     string
	ParadigmVersion                                                 int
	Parameters                                                      map[string]float64
	VideoVerified                                                   bool
	TestCancelled                                                   bool
	ExperimentStatus                                                string
	Excluded                                                        bool
	// Invalid is set when the candidate's stored inputs cannot be read, such
	// as arena parameters that are not numbers; it is never planned.
	Invalid error
}

// eligible reports whether the animal's video and study state allow an analysis.
func (c Candidate) eligible() bool {
	return c.RecordingID != "" && c.VideoVerified && !c.TestCancelled && !c.Excluded &&
		(c.ExperimentStatus == "RUNNING" || c.ExperimentStatus == "COMPLETED")
}

type OutputUpload struct {
	RunID       string
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

// Published is the accepted result of a run.
type Published struct {
	Metrics   []domain.MetricValue
	Events    []domain.Event
	Artifacts []VerifiedOutput
	Pair      *Pair
}

type Pair struct {
	SourceAssetID, AnalyzedAssetID string
	SourceGeneration               int64
	AnalyzedObjectName             string
	AnalyzedGeneration             int64
	SourceObjectName               string
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
	// ReadyTestSubjects lists animals that can be analysed now with one of the
	// capabilities, with a valid calibration when their paradigm version is
	// among calibrated, and no automatic run for the current video generation
	// and calibration. Every returned candidate is eligible.
	ReadyTestSubjects(ctx context.Context, capabilities, calibrated []domain.Capability) ([]Candidate, error)
	// Candidate returns ErrTestSubjectNotFound for an unknown test subject.
	Candidate(ctx context.Context, testSubjectID string) (Candidate, error)
	// CreateAutomaticRun reports false when an equal automatic run already exists.
	CreateAutomaticRun(ctx context.Context, r domain.Run) (bool, error)
	// LockTestSubject serializes run creation for one animal of a test until
	// the transaction ends; PendingRun then reports a queued or running run of it.
	LockTestSubject(ctx context.Context, testSubjectID string) error
	PendingRun(ctx context.Context, testSubjectID string) (bool, error)
	CreateRun(ctx context.Context, r domain.Run) (domain.Run, error)
	LockExpiredRuns(ctx context.Context, now time.Time) ([]domain.Run, error)
	// ClaimNext locks the oldest available queued run for one of the
	// capabilities, skipping runs locked by others, or returns ErrNoRun.
	ClaimNext(ctx context.Context, capabilities []domain.Capability, now time.Time) (domain.Run, error)
	UpdateRun(ctx context.Context, r domain.Run) (domain.Run, error)
	Run(ctx context.Context, id string) (domain.Run, error)
	LockRun(ctx context.Context, id string) (domain.Run, error)
	// Runs lists the runs of a test, of one of its animals when testSubjectID is set.
	Runs(ctx context.Context, testID, testSubjectID string) ([]domain.Run, error)
	SubjectRuns(ctx context.Context, testSubjectID string) ([]domain.Run, error)
	Calibration(ctx context.Context, id string) (domain.CalibrationRef, error)
	// Arena returns the environment revision a test subject is filmed in.
	Arena(ctx context.Context, testSubjectID string) (string, error)
	SourceObject(ctx context.Context, assetID string) (string, error)
	CreateOutputUpload(ctx context.Context, u OutputUpload) error
	OutputUploads(ctx context.Context, runID string, attempt int) ([]OutputUpload, error)
	// Publish stores the result, analyzed video asset, pair and artifacts in the caller's transaction.
	Publish(ctx context.Context, run domain.Run, result domain.Result, outputs []VerifiedOutput) error
	Published(ctx context.Context, runID string) (Published, error)
	// OpenResult creates the PENDING result of the run's animal unless it has one.
	OpenResult(ctx context.Context, r domain.SubjectResult) error
	// LockResult locks an animal's result and returns its experiment's status,
	// or ErrResultNotFound.
	LockResult(ctx context.Context, testSubjectID string) (domain.SubjectResult, string, error)
	UpdateResult(ctx context.Context, r domain.SubjectResult) (domain.SubjectResult, error)
	ReviewItems(ctx context.Context, f ReviewFilter) ([]ReviewItem, int, error)
	RunMetrics(ctx context.Context, runIDs []string) (map[string][]domain.MetricValue, error)
}

type Settings struct {
	Lease              time.Duration
	MaxAttempts        int
	UploadTTL, ReadTTL time.Duration
	MaxOutputBytes     int64
	// MaxTrajectoryBytes caps the trajectory the API reads into memory.
	MaxTrajectoryBytes int64
}

type Service struct {
	store    Store
	objects  ObjectStore
	catalog  Catalog
	tokens   Tokens
	settings Settings
	now      func() time.Time
}

// New builds the service. A nil objects store disables claiming and outputs.
func New(store Store, objects ObjectStore, catalog Catalog, tokens Tokens, settings Settings, now func() time.Time) *Service {
	return &Service{store: store, objects: objects, catalog: catalog, tokens: tokens, settings: settings, now: now}
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
		return Registered{}, err
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

// TickReport counts what a scheduler tick did. Skipped holds candidates and
// capabilities that could not be planned or enqueued; a tick logs them and
// carries on, so one bad row never blocks the others.
type TickReport struct {
	Enqueued, Requeued, Failed int
	Skipped                    []error
}

// Tick returns expired leases to the queue and enqueues one automatic run per
// ready animal. Concurrent ticks create each automatic run once.
func (s *Service) Tick(ctx context.Context) (TickReport, error) {
	var report TickReport
	now := s.clock()
	err := s.store.Transaction(ctx, func(tx Store) error {
		expired, err := tx.LockExpiredRuns(ctx, now)
		if err != nil {
			return err
		}
		for _, r := range expired {
			next, changed := r.Expire(now)
			if !changed {
				continue
			}
			if _, err := tx.UpdateRun(ctx, next); err != nil {
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
	active, err := s.store.ActiveCapabilities(ctx)
	if err != nil {
		return TickReport{}, err
	}
	var capabilities, calibrated []domain.Capability
	for _, c := range active {
		contract, err := s.catalog.Contract(c.ParadigmKey, c.ParadigmVersion)
		if err != nil {
			report.Skipped = append(report.Skipped, fmt.Errorf("capability %s v%d: %w", c.ParadigmKey, c.ParadigmVersion, err))
			continue
		}
		capabilities = append(capabilities, c)
		if contract.CalibrationRequired {
			calibrated = append(calibrated, c)
		}
	}
	if len(capabilities) == 0 {
		return report, nil
	}
	candidates, err := s.store.ReadyTestSubjects(ctx, capabilities, calibrated)
	if err != nil {
		return TickReport{}, err
	}
	for _, c := range candidates {
		run, ok, err := s.plan(c, capabilities, domain.Automatic, "", now)
		if err == nil && !ok {
			err = ErrNotReady
		}
		if err == nil {
			var created bool
			if created, err = s.store.CreateAutomaticRun(ctx, run); created {
				report.Enqueued++
			}
		}
		if err != nil {
			report.Skipped = append(report.Skipped, fmt.Errorf("test subject %s: %w", c.TestSubjectID, err))
		}
	}
	return report, nil
}

// plan decides whether a candidate is ready and pins its inputs.
func (s *Service) plan(c Candidate, capabilities []domain.Capability, trigger domain.Trigger, createdBy string, now time.Time) (domain.Run, bool, error) {
	if c.Invalid != nil {
		return domain.Run{}, false, c.Invalid
	}
	if !c.eligible() {
		return domain.Run{}, false, nil
	}
	contract, err := s.catalog.Contract(c.ParadigmKey, c.ParadigmVersion)
	if err != nil {
		return domain.Run{}, false, err
	}
	if contract.CalibrationRequired && c.CalibrationID == "" {
		return domain.Run{}, false, nil
	}
	if !contract.CalibrationRequired {
		c.CalibrationID = ""
	}
	capable := false
	for _, cap := range capabilities {
		capable = capable || (cap.ParadigmKey == c.ParadigmKey && cap.ParadigmVersion == c.ParadigmVersion)
	}
	if !capable {
		return domain.Run{}, false, nil
	}
	source := domain.Source{
		ExperimentID: c.ExperimentID, TestID: c.TestID, TestSubjectID: c.TestSubjectID, RecordingID: c.RecordingID, SourceAssetID: c.SourceAssetID,
		SourceGeneration: c.SourceGeneration, SourceCRC32C: c.SourceCRC32C, ClipStartUs: c.ClipStartUs, ClipEndUs: c.ClipEndUs,
		CalibrationID: c.CalibrationID, ParadigmKey: c.ParadigmKey, ParadigmVersion: c.ParadigmVersion, Parameters: c.Parameters,
	}
	return domain.NewRun(source, trigger, createdBy, now, s.settings.MaxAttempts, contract.MetricEngineVersion, contract.ResultSchemaVersion), true, nil
}

// Reanalyze queues a new manual run of one animal with the current inputs:
// the test's current video and the arena's current effective calibration.
// Earlier runs, their results and the reviewed result stay unchanged.
func (s *Service) Reanalyze(ctx context.Context, actor access.Actor, testSubjectID string) (domain.Run, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Run{}, err
	}
	c, err := s.store.Candidate(ctx, testSubjectID)
	if err != nil {
		return domain.Run{}, err
	}
	capabilities, err := s.store.ActiveCapabilities(ctx)
	if err != nil {
		return domain.Run{}, err
	}
	run, ok, err := s.plan(c, capabilities, domain.Manual, actor.UserID, s.clock())
	if c.Invalid != nil || errors.Is(err, ErrUnknownCapability) || (err == nil && !ok) {
		return domain.Run{}, ErrNotReady
	}
	if err != nil {
		return domain.Run{}, err
	}
	var created domain.Run
	err = s.store.Transaction(ctx, func(tx Store) error {
		// One analysis per animal at a time: a repeated request must not flood the workers.
		if err := tx.LockTestSubject(ctx, c.TestSubjectID); err != nil {
			return err
		}
		pending, err := tx.PendingRun(ctx, c.TestSubjectID)
		if err != nil {
			return err
		}
		if pending {
			return ErrRunPending
		}
		created, err = tx.CreateRun(ctx, run)
		return err
	})
	return created, err
}

// Job is a claimed run with its contract, the arena (environment revision)
// its animal is filmed in and, when the paradigm needs one, the pinned
// calibration whose crop is that arena's region of the frame and whose
// transform maps its pixels to arena centimeters.
type Job struct {
	Run         domain.Run
	Contract    domain.Contract
	Arena       string
	Calibration *domain.CalibrationRef
}

// Claim leases the oldest queued run the worker can analyze.
func (s *Service) Claim(ctx context.Context, w domain.Worker) (Job, error) {
	if s.objects == nil {
		return Job{}, ErrStorageNotConfigured
	}
	now := s.clock()
	var job Job
	err := s.store.Transaction(ctx, func(tx Store) error {
		run, err := tx.ClaimNext(ctx, w.Capabilities, now)
		if err != nil {
			return err
		}
		claimed, err := run.Claim(w.ID, now, s.settings.Lease)
		if err != nil {
			return err
		}
		if job.Run, err = tx.UpdateRun(ctx, claimed); err != nil {
			return err
		}
		if job.Contract, err = s.catalog.Contract(claimed.ParadigmKey, claimed.ParadigmVersion); err != nil {
			return err
		}
		if job.Arena, err = tx.Arena(ctx, claimed.TestSubjectID); err != nil {
			return err
		}
		if claimed.CalibrationID != "" {
			calibration, err := tx.Calibration(ctx, claimed.CalibrationID)
			if err != nil {
				return err
			}
			job.Calibration = &calibration
		}
		return nil
	})
	if err != nil {
		return Job{}, err
	}
	return job, nil
}

func (s *Service) Heartbeat(ctx context.Context, w domain.Worker, runID string, attempt int) (domain.Run, error) {
	now := s.clock()
	var out domain.Run
	err := s.store.Transaction(ctx, func(tx Store) error {
		run, err := tx.LockRun(ctx, runID)
		if err != nil {
			return err
		}
		beat, err := run.Heartbeat(w.ID, attempt, now, s.settings.Lease)
		if err != nil {
			return err
		}
		out, err = tx.UpdateRun(ctx, beat)
		return err
	})
	return out, err
}

// SourceURL signs a read of the run's pinned source generation for its worker.
func (s *Service) SourceURL(ctx context.Context, w domain.Worker, runID string, attempt int) (string, time.Time, error) {
	if s.objects == nil {
		return "", time.Time{}, ErrStorageNotConfigured
	}
	run, err := s.store.Run(ctx, runID)
	if err != nil {
		return "", time.Time{}, err
	}
	if err := run.CheckLease(w.ID, attempt); err != nil {
		return "", time.Time{}, err
	}
	object, err := s.store.SourceObject(ctx, run.SourceAssetID)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := s.now().Add(s.settings.ReadTTL)
	url, err := s.objects.ReadURL(ctx, object, run.SourceGeneration, expires)
	return url, expires, err
}

type OutputRequest struct {
	Attempt     int
	Kind        string
	FileName    string
	ContentType string
	SizeBytes   int64
	CRC32C      uint32
}

// RequestOutput records an output the attempt will upload and signs its upload.
func (s *Service) RequestOutput(ctx context.Context, w domain.Worker, runID string, req OutputRequest) (OutputUpload, SignedRequest, error) {
	if s.objects == nil {
		return OutputUpload{}, SignedRequest{}, ErrStorageNotConfigured
	}
	run, err := s.store.Run(ctx, runID)
	if err != nil {
		return OutputUpload{}, SignedRequest{}, err
	}
	if err := run.CheckLease(w.ID, req.Attempt); err != nil {
		return OutputUpload{}, SignedRequest{}, err
	}
	name := strings.TrimSpace(req.FileName)
	video := req.Kind == domain.AnalyzedVideo && (req.ContentType == "video/mp4" || req.ContentType == "video/webm")
	data := (req.Kind == domain.Trajectory && req.ContentType == domain.TrajectoryContentType) ||
		(req.Kind == domain.Thumbnail && (req.ContentType == "image/jpeg" || req.ContentType == "image/png"))
	if (!video && !data) || req.SizeBytes < 1 || req.SizeBytes > s.settings.MaxOutputBytes ||
		name == "" || len(name) > 120 || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return OutputUpload{}, SignedRequest{}, ErrInvalidOutput
	}
	u := OutputUpload{RunID: run.ID, Attempt: run.Attempt, ObjectName: run.OutputPrefix() + name, Kind: req.Kind, ContentType: req.ContentType, SizeBytes: req.SizeBytes, CRC32C: req.CRC32C}
	if err := s.store.CreateOutputUpload(ctx, u); err != nil {
		return OutputUpload{}, SignedRequest{}, err
	}
	signed, err := s.objects.UploadURL(ctx, u.ObjectName, u.ContentType, s.now().Add(s.settings.UploadTTL))
	return u, signed, err
}

// Submit verifies every output object in storage, then publishes the result,
// the analyzed video and its pair in one transaction. A repeated delivery of
// the accepted attempt returns the run without publishing again.
func (s *Service) Submit(ctx context.Context, w domain.Worker, runID string, sub domain.Submission) (domain.Run, error) {
	if s.objects == nil {
		return domain.Run{}, ErrStorageNotConfigured
	}
	sub.WorkerID = w.ID
	run, err := s.store.Run(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	if run.Status == domain.Succeeded && run.WorkerID == w.ID && run.Attempt == sub.Attempt {
		return run, nil
	}
	if err := run.CheckLease(w.ID, sub.Attempt); err != nil {
		return domain.Run{}, err
	}
	result, err := domain.Validate(run, sub)
	if err != nil {
		return domain.Run{}, err
	}
	uploads, err := s.store.OutputUploads(ctx, run.ID, sub.Attempt)
	if err != nil {
		return domain.Run{}, err
	}
	outputs := make([]VerifiedOutput, 0, len(result.Artifacts))
	for _, a := range result.Artifacts {
		i := indexUpload(uploads, a)
		if i < 0 {
			return domain.Run{}, ErrUnknownOutput
		}
		attrs, err := s.objects.Attrs(ctx, a.ObjectName)
		if errors.Is(err, ErrObjectNotFound) {
			return domain.Run{}, ErrOutputNotVerified
		}
		if err != nil {
			return domain.Run{}, err
		}
		u := uploads[i]
		if attrs.Generation <= 0 || attrs.Size != u.SizeBytes || attrs.CRC32C != u.CRC32C || attrs.ContentType != u.ContentType {
			return domain.Run{}, ErrOutputNotVerified
		}
		outputs = append(outputs, VerifiedOutput{OutputUpload: u, Bucket: s.objects.Bucket(), Generation: attrs.Generation})
	}
	track, err := s.readTrajectory(ctx, run, result, outputs)
	if err != nil {
		return domain.Run{}, err
	}
	contract, err := s.catalog.Contract(run.ParadigmKey, run.ParadigmVersion)
	if err != nil {
		return domain.Run{}, err
	}
	if reason := domain.CheckQC(contract.QC, track); reason != "" {
		return s.Fail(ctx, w, runID, sub.Attempt, reason, false)
	}
	computed, err := s.catalog.Evaluate(run.ParadigmKey, run.ParadigmVersion, run.Parameters, track, result.DurationUs)
	if err != nil {
		return domain.Run{}, err
	}
	result.Metrics, result.Events = computed.Metrics, track.Events(computed)
	now := s.clock()
	var out domain.Run
	err = s.store.Transaction(ctx, func(tx Store) error {
		locked, err := tx.LockRun(ctx, runID)
		if err != nil {
			return err
		}
		if locked.Status == domain.Succeeded && locked.WorkerID == w.ID && locked.Attempt == sub.Attempt {
			out = locked
			return nil
		}
		done, err := locked.Succeed(w.ID, sub.Attempt, result.ModelVersion, now)
		if err != nil {
			return err
		}
		if err := tx.Publish(ctx, done, result, outputs); err != nil {
			return err
		}
		if out, err = tx.UpdateRun(ctx, done); err != nil {
			return err
		}
		// The first successful run of an animal opens its result for review.
		return tx.OpenResult(ctx, domain.OpenResult(out))
	})
	if err != nil {
		return domain.Run{}, err
	}
	return out, nil
}

// readTrajectory reads the verified trajectory generation. The published
// metrics are computed from exactly these bytes.
func (s *Service) readTrajectory(ctx context.Context, run domain.Run, result domain.Result, outputs []VerifiedOutput) (domain.Track, error) {
	for _, o := range outputs {
		if o.ObjectName != result.TrajectoryObject {
			continue
		}
		if o.ContentType != domain.TrajectoryContentType {
			return domain.Track{}, domain.ErrMissingTrajectory
		}
		if o.SizeBytes > s.settings.MaxTrajectoryBytes {
			return domain.Track{}, fmt.Errorf("%w: %d bytes exceed %d", domain.ErrInvalidTrajectory, o.SizeBytes, s.settings.MaxTrajectoryBytes)
		}
		data, err := s.objects.Read(ctx, o.ObjectName, o.Generation, s.settings.MaxTrajectoryBytes)
		if errors.Is(err, ErrObjectNotFound) {
			return domain.Track{}, ErrOutputNotVerified
		}
		if err != nil {
			return domain.Track{}, err
		}
		return domain.ParseTrajectory(data, run, result.DurationUs)
	}
	return domain.Track{}, domain.ErrMissingTrajectory
}

func indexUpload(uploads []OutputUpload, a domain.ArtifactInput) int {
	for i, u := range uploads {
		if u.ObjectName == a.ObjectName && u.Kind == a.Kind {
			return i
		}
	}
	return -1
}

// Fail records a worker failure for its current attempt.
func (s *Service) Fail(ctx context.Context, w domain.Worker, runID string, attempt int, reason string, retryable bool) (domain.Run, error) {
	now := s.clock()
	var out domain.Run
	err := s.store.Transaction(ctx, func(tx Store) error {
		run, err := tx.LockRun(ctx, runID)
		if err != nil {
			return err
		}
		failed, err := run.Fail(w.ID, attempt, reason, retryable, now)
		if err != nil {
			return err
		}
		out, err = tx.UpdateRun(ctx, failed)
		return err
	})
	return out, err
}

// Runs lists the runs of a test, newest first, optionally of one of its animals.
func (s *Service) Runs(ctx context.Context, actor access.Actor, testID, testSubjectID string) ([]domain.Run, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	return s.store.Runs(ctx, testID, testSubjectID)
}

type Detail struct {
	Run       domain.Run
	Published *Published
}

func (s *Service) RunDetail(ctx context.Context, actor access.Actor, runID string) (Detail, error) {
	if err := actor.Require(access.Read); err != nil {
		return Detail{}, err
	}
	run, err := s.store.Run(ctx, runID)
	if err != nil {
		return Detail{}, err
	}
	if run.Status != domain.Succeeded {
		return Detail{Run: run}, nil
	}
	published, err := s.store.Published(ctx, run.ID)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Run: run, Published: &published}, nil
}

type VideoPairLinks struct {
	Pair                     Pair
	OriginalURL, AnalyzedURL string
	ExpiresAt                time.Time
}

// VideoPair signs reads of the run's pinned original and its own analyzed video.
func (s *Service) VideoPair(ctx context.Context, actor access.Actor, runID string) (VideoPairLinks, error) {
	if err := actor.Require(access.Read); err != nil {
		return VideoPairLinks{}, err
	}
	if s.objects == nil {
		return VideoPairLinks{}, ErrStorageNotConfigured
	}
	run, err := s.store.Run(ctx, runID)
	if err != nil {
		return VideoPairLinks{}, err
	}
	if run.Status != domain.Succeeded {
		return VideoPairLinks{}, ErrRunNotSucceeded
	}
	published, err := s.store.Published(ctx, run.ID)
	if err != nil {
		return VideoPairLinks{}, err
	}
	if published.Pair == nil {
		return VideoPairLinks{}, ErrRunNotSucceeded
	}
	p := *published.Pair
	expires := s.now().Add(s.settings.ReadTTL)
	original, err := s.objects.ReadURL(ctx, p.SourceObjectName, p.SourceGeneration, expires)
	if err != nil {
		return VideoPairLinks{}, err
	}
	analyzed, err := s.objects.ReadURL(ctx, p.AnalyzedObjectName, p.AnalyzedGeneration, expires)
	if err != nil {
		return VideoPairLinks{}, err
	}
	return VideoPairLinks{Pair: p, OriginalURL: original, AnalyzedURL: analyzed, ExpiresAt: expires}, nil
}
