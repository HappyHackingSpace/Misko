// Package domain holds analysis jobs, their leases and the validation of
// worker results. A job analyses one video with one paradigm version, its
// parameters and, when the paradigm needs one, a calibration. Its input is an
// immutable snapshot: the source video generation and clip, the parameters and
// a copy of the calibration (frame, crop, transform and zones). The requester
// (a paradigm trial or an animal of a test) is an opaque reference; the
// service never reads the requester's data. Reanalysis is a new job.
package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotClaimable        = errors.New("the job is not queued or not yet available")
	ErrStaleAttempt        = errors.New("the attempt is not the current lease of this worker")
	ErrInvalidReason       = errors.New("failure reason must contain 1 to 2000 characters")
	ErrInvalidModelVersion = errors.New("model version must contain 1 to 120 printable characters")
	ErrInvalidRequest      = errors.New("a job needs a TRIAL or TEST_SUBJECT requester, a paradigm version, finite parameters, a source video with a valid clip, a known trigger and an idempotency key of at most 200 characters")
	ErrNotAnalysable       = errors.New("this paradigm version cannot be analysed from a trajectory")
	ErrSourceNotVerified   = errors.New("the source video has not been verified")
	ErrCalibrationRequired = errors.New("this paradigm needs a valid calibration")
	ErrMissingZones        = errors.New("the calibration has no shape for a zone the paradigm requires")
	ErrInvalidCalibration  = errors.New("the calibration snapshot needs a frame, a crop inside it and a finite transform")
)

type Status string

const (
	Queued    Status = "QUEUED"
	Running   Status = "RUNNING"
	Succeeded Status = "SUCCEEDED"
	Failed    Status = "FAILED"
)

// Finished reports whether the job reached its final state.
func (s Status) Finished() bool { return s == Succeeded || s == Failed }

// Pending reports whether the job still waits for or holds a worker.
func (s Status) Pending() bool { return s == Queued || s == Running }

type Trigger string

const (
	Automatic Trigger = "AUTOMATIC"
	Manual    Trigger = "MANUAL"
)

// RequesterKind names what asked for a job.
type RequesterKind string

const (
	// RequesterTrial is a paradigm trial outside any experiment.
	RequesterTrial RequesterKind = "TRIAL"
	// RequesterTestSubject is one animal of a test of an experiment.
	RequesterTestSubject RequesterKind = "TEST_SUBJECT"
)

// Valid reports whether the kind is known.
func (k RequesterKind) Valid() bool { return k == RequesterTrial || k == RequesterTestSubject }

// Requester is an opaque reference to whoever asked for the job.
type Requester struct {
	Kind RequesterKind
	ID   string
}

// ReasonLeaseExpired fails a job whose last attempt lost its lease.
const ReasonLeaseExpired = "LEASE_EXPIRED"

const maxIdempotencyKey = 200

// Request is what a requester asks the service to analyse. The service
// resolves the source video and the calibration into the job's snapshot.
type Request struct {
	Requester       Requester
	ParadigmKey     string
	ParadigmVersion int
	// Parameters are the resolved apparatus, analysis and session values.
	Parameters    map[string]float64
	SourceAssetID string
	ClipStartUs   int64
	ClipEndUs     *int64
	// CalibrationID is empty when the requester has no valid calibration.
	CalibrationID string
	Trigger       Trigger
	// IdempotencyKey, when set, makes a repeated submission return the job
	// the first one created.
	IdempotencyKey string
	CreatedBy      string
}

// Validate checks the request's shape.
func (r Request) Validate() error {
	switch {
	case !r.Requester.Kind.Valid() || strings.TrimSpace(r.Requester.ID) == "":
		return fmt.Errorf("%w: requester", ErrInvalidRequest)
	case !paradigmKey.MatchString(r.ParadigmKey) || r.ParadigmVersion < 1:
		return fmt.Errorf("%w: paradigm", ErrInvalidRequest)
	case strings.TrimSpace(r.SourceAssetID) == "":
		return fmt.Errorf("%w: source video", ErrInvalidRequest)
	case r.ClipStartUs < 0 || (r.ClipEndUs != nil && *r.ClipEndUs <= r.ClipStartUs):
		return fmt.Errorf("%w: clip", ErrInvalidRequest)
	case r.Trigger != Automatic && r.Trigger != Manual:
		return fmt.Errorf("%w: trigger", ErrInvalidRequest)
	case utf8.RuneCountInString(r.IdempotencyKey) > maxIdempotencyKey:
		return fmt.Errorf("%w: idempotency key", ErrInvalidRequest)
	}
	for key, v := range r.Parameters {
		if key == "" || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%w: parameter %q", ErrInvalidRequest, key)
		}
	}
	return nil
}

// SourceVideo is what the video store reports about a source asset.
type SourceVideo struct {
	ID         string
	ObjectName string
	Verified   bool
	Generation int64
	CRC32C     uint32
}

// Source is the pinned source video generation and the clip analysed.
type Source struct {
	AssetID     string
	Generation  int64
	CRC32C      uint32
	ClipStartUs int64
	ClipEndUs   *int64
}

type Rect struct {
	X, Y, Width, Height int
}

// Calibration is the snapshot of the calibration a job analyses with: its
// crop of the frame, the transform from full-frame pixels to centimetres and
// the calibrated zone shapes. ID names where it was copied from.
type Calibration struct {
	ID                      string
	FrameWidth, FrameHeight int
	Crop                    Rect
	Plane                   string
	Transform               [9]float64
	Zones                   []Zone
}

// Validate checks the snapshot's geometry.
func (c Calibration) Validate() error {
	if c.ID == "" || c.FrameWidth < 1 || c.FrameHeight < 1 || c.Crop.X < 0 || c.Crop.Y < 0 || c.Crop.Width < 1 || c.Crop.Height < 1 ||
		c.Crop.X+c.Crop.Width > c.FrameWidth || c.Crop.Y+c.Crop.Height > c.FrameHeight || c.Plane == "" {
		return ErrInvalidCalibration
	}
	for _, v := range c.Transform {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return ErrInvalidCalibration
		}
	}
	return nil
}

// ZoneKeys lists the zones the calibration has shapes for.
func (c Calibration) ZoneKeys() []string {
	keys := make([]string, 0, len(c.Zones))
	for _, z := range c.Zones {
		keys = append(keys, z.Key)
	}
	return keys
}

// Input is a job's immutable snapshot.
type Input struct {
	Requester           Requester
	IdempotencyKey      string
	ParadigmKey         string
	ParadigmVersion     int
	MetricEngineVersion int
	ResultSchemaVersion int
	Parameters          map[string]float64
	Source              Source
	// Calibration is nil when the paradigm needs none.
	Calibration *Calibration
	Trigger     Trigger
	CreatedBy   string
}

// CalibrationID is the id of the calibration the snapshot was copied from, or "".
func (in Input) CalibrationID() string {
	if in.Calibration == nil {
		return ""
	}
	return in.Calibration.ID
}

type Job struct {
	ID string
	Input
	Status Status
	// Attempt counts claims; the current attempt is the fencing token.
	Attempt        int
	MaxAttempts    int
	WorkerID       string
	LeaseExpiresAt *time.Time
	AvailableAt    time.Time
	ModelVersion   string
	FailureReason  string
	CreatedAt      time.Time
	FinishedAt     *time.Time
}

// NewJob snapshots a request for the paradigm's contract, the verified source
// video and, when the contract needs one, the calibration.
func NewJob(req Request, contract Contract, video SourceVideo, calibration *Calibration, now time.Time, maxAttempts int) (Job, error) {
	if err := req.Validate(); err != nil {
		return Job{}, err
	}
	if !contract.TrajectoryAnalyzable {
		return Job{}, ErrNotAnalysable
	}
	if !video.Verified || video.Generation <= 0 || video.ID != req.SourceAssetID {
		return Job{}, ErrSourceNotVerified
	}
	snapshot, err := pinCalibration(contract, calibration)
	if err != nil {
		return Job{}, err
	}
	params := make(map[string]float64, len(req.Parameters))
	for k, v := range req.Parameters {
		params[k] = v
	}
	return Job{
		Input: Input{
			Requester: Requester{Kind: req.Requester.Kind, ID: strings.TrimSpace(req.Requester.ID)}, IdempotencyKey: req.IdempotencyKey,
			ParadigmKey: req.ParadigmKey, ParadigmVersion: req.ParadigmVersion,
			MetricEngineVersion: contract.MetricEngineVersion, ResultSchemaVersion: contract.ResultSchemaVersion, Parameters: params,
			Source:      Source{AssetID: video.ID, Generation: video.Generation, CRC32C: video.CRC32C, ClipStartUs: req.ClipStartUs, ClipEndUs: req.ClipEndUs},
			Calibration: snapshot, Trigger: req.Trigger, CreatedBy: req.CreatedBy,
		},
		Status: Queued, MaxAttempts: maxAttempts, AvailableAt: now,
	}, nil
}

// pinCalibration keeps the calibration a contract needs, with every required
// zone, and drops one it does not need.
func pinCalibration(contract Contract, calibration *Calibration) (*Calibration, error) {
	if !contract.CalibrationRequired {
		return nil, nil
	}
	if calibration == nil {
		return nil, ErrCalibrationRequired
	}
	if err := calibration.Validate(); err != nil {
		return nil, err
	}
	if missing := MissingZones(contract.RequiredZones, calibration.ZoneKeys()); len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrMissingZones, strings.Join(missing, ", "))
	}
	copied := *calibration
	copied.Zones = append([]Zone(nil), calibration.Zones...)
	return &copied, nil
}

// Claim starts the next attempt for a worker with a lease until now+lease.
func (j Job) Claim(workerID string, now time.Time, lease time.Duration) (Job, error) {
	if j.Status != Queued || j.AvailableAt.After(now) {
		return Job{}, ErrNotClaimable
	}
	expires := now.Add(lease)
	j.Status, j.Attempt, j.WorkerID, j.LeaseExpiresAt = Running, j.Attempt+1, workerID, &expires
	return j, nil
}

// CheckLease accepts only the worker holding the current attempt of a running job.
func (j Job) CheckLease(workerID string, attempt int) error {
	if j.Status != Running || j.WorkerID != workerID || j.Attempt != attempt {
		return ErrStaleAttempt
	}
	return nil
}

func (j Job) Heartbeat(workerID string, attempt int, now time.Time, lease time.Duration) (Job, error) {
	if err := j.CheckLease(workerID, attempt); err != nil {
		return Job{}, err
	}
	expires := now.Add(lease)
	j.LeaseExpiresAt = &expires
	return j, nil
}

// Expire returns a job whose lease ended to the queue, or fails it after the
// last attempt. The second result reports whether the lease had expired.
func (j Job) Expire(now time.Time) (Job, bool) {
	if j.Status != Running || j.LeaseExpiresAt == nil || !now.After(*j.LeaseExpiresAt) {
		return j, false
	}
	j.WorkerID, j.LeaseExpiresAt, j.FailureReason = "", nil, ReasonLeaseExpired
	if j.Attempt >= j.MaxAttempts {
		j.Status, j.FinishedAt = Failed, &now
		return j, true
	}
	j.Status, j.AvailableAt = Queued, now
	return j, true
}

// Fail records a worker failure; retryable failures return to the queue while attempts remain.
func (j Job) Fail(workerID string, attempt int, reason string, retryable bool, now time.Time) (Job, error) {
	if err := j.CheckLease(workerID, attempt); err != nil {
		return Job{}, err
	}
	text := strings.TrimSpace(reason)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 2000 {
		return Job{}, ErrInvalidReason
	}
	j.WorkerID, j.LeaseExpiresAt, j.FailureReason = "", nil, text
	if retryable && j.Attempt < j.MaxAttempts {
		j.Status, j.AvailableAt = Queued, now
		return j, nil
	}
	j.Status, j.FinishedAt = Failed, &now
	return j, nil
}

// Succeed marks the current attempt as the accepted result.
func (j Job) Succeed(workerID string, attempt int, modelVersion string, now time.Time) (Job, error) {
	if err := j.CheckLease(workerID, attempt); err != nil {
		return Job{}, err
	}
	model, err := normalizeModel(modelVersion)
	if err != nil {
		return Job{}, err
	}
	j.Status, j.ModelVersion, j.LeaseExpiresAt, j.FailureReason, j.FinishedAt = Succeeded, model, nil, "", &now
	return j, nil
}

// OutputPrefix is where the current attempt writes its outputs. Every attempt
// has its own prefix, so a late attempt cannot overwrite the winning outputs.
func (j Job) OutputPrefix() string {
	return fmt.Sprintf("jobs/%s/attempts/%d/", j.ID, j.Attempt)
}

func normalizeModel(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 120 || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return "", ErrInvalidModelVersion
	}
	return text, nil
}
