// Package domain holds analysis runs, their leases and the validation of
// worker results. A run pins its source video generation, clip, calibration,
// paradigm version and parameters; its inputs and published outputs never
// change, and reanalysis is a new run.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotClaimable        = errors.New("the run is not queued or not yet available")
	ErrStaleAttempt        = errors.New("the attempt is not the current lease of this worker")
	ErrInvalidReason       = errors.New("failure reason must contain 1 to 2000 characters")
	ErrInvalidModelVersion = errors.New("model version must contain 1 to 120 printable characters")
)

type Status string

const (
	Queued    Status = "QUEUED"
	Running   Status = "RUNNING"
	Succeeded Status = "SUCCEEDED"
	Failed    Status = "FAILED"
)

type Trigger string

const (
	Automatic Trigger = "AUTOMATIC"
	Manual    Trigger = "MANUAL"
)

// ReasonLeaseExpired fails a run whose last attempt lost its lease.
const ReasonLeaseExpired = "LEASE_EXPIRED"

// Source is everything a run pins about its input.
type Source struct {
	ExperimentID          string
	TestID                string
	RecordingID           string
	SourceAssetID         string
	SourceGeneration      int64
	SourceCRC32C          uint32
	ClipStartUs           int64
	ClipEndUs             *int64
	CalibrationID         string
	ParadigmKey           string
	ParadigmVersion       int
	EnvironmentRevisionID string
	ProtocolVersionID     string
	Parameters            map[string]float64
}

type Run struct {
	ID string
	Source
	MetricEngineVersion int
	ResultSchemaVersion int
	Trigger             Trigger
	Status              Status
	// Attempt counts claims; the current attempt is the fencing token.
	Attempt        int
	MaxAttempts    int
	WorkerID       string
	LeaseExpiresAt *time.Time
	AvailableAt    time.Time
	ModelVersion   string
	FailureReason  string
	CreatedBy      string
	CreatedAt      time.Time
	FinishedAt     *time.Time
}

func NewRun(source Source, trigger Trigger, createdBy string, now time.Time, maxAttempts, metricEngineVersion, resultSchemaVersion int) Run {
	return Run{
		Source: source, Trigger: trigger, Status: Queued, MaxAttempts: maxAttempts, AvailableAt: now, CreatedBy: createdBy,
		MetricEngineVersion: metricEngineVersion, ResultSchemaVersion: resultSchemaVersion,
	}
}

// Claim starts the next attempt for a worker with a lease until now+lease.
func (r Run) Claim(workerID string, now time.Time, lease time.Duration) (Run, error) {
	if r.Status != Queued || r.AvailableAt.After(now) {
		return Run{}, ErrNotClaimable
	}
	expires := now.Add(lease)
	r.Status, r.Attempt, r.WorkerID, r.LeaseExpiresAt = Running, r.Attempt+1, workerID, &expires
	return r, nil
}

// CheckLease accepts only the worker holding the current attempt of a running run.
func (r Run) CheckLease(workerID string, attempt int) error {
	if r.Status != Running || r.WorkerID != workerID || r.Attempt != attempt {
		return ErrStaleAttempt
	}
	return nil
}

func (r Run) Heartbeat(workerID string, attempt int, now time.Time, lease time.Duration) (Run, error) {
	if err := r.CheckLease(workerID, attempt); err != nil {
		return Run{}, err
	}
	expires := now.Add(lease)
	r.LeaseExpiresAt = &expires
	return r, nil
}

// Expire returns a run whose lease ended to the queue, or fails it after the
// last attempt. The second result reports whether the lease had expired.
func (r Run) Expire(now time.Time) (Run, bool) {
	if r.Status != Running || r.LeaseExpiresAt == nil || !now.After(*r.LeaseExpiresAt) {
		return r, false
	}
	r.WorkerID, r.LeaseExpiresAt, r.FailureReason = "", nil, ReasonLeaseExpired
	if r.Attempt >= r.MaxAttempts {
		r.Status, r.FinishedAt = Failed, &now
		return r, true
	}
	r.Status, r.AvailableAt = Queued, now
	return r, true
}

// Fail records a worker failure; retryable failures return to the queue while attempts remain.
func (r Run) Fail(workerID string, attempt int, reason string, retryable bool, now time.Time) (Run, error) {
	if err := r.CheckLease(workerID, attempt); err != nil {
		return Run{}, err
	}
	text := strings.TrimSpace(reason)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 2000 {
		return Run{}, ErrInvalidReason
	}
	r.WorkerID, r.LeaseExpiresAt, r.FailureReason = "", nil, text
	if retryable && r.Attempt < r.MaxAttempts {
		r.Status, r.AvailableAt = Queued, now
		return r, nil
	}
	r.Status, r.FinishedAt = Failed, &now
	return r, nil
}

// Succeed marks the current attempt as the accepted result.
func (r Run) Succeed(workerID string, attempt int, modelVersion string, now time.Time) (Run, error) {
	if err := r.CheckLease(workerID, attempt); err != nil {
		return Run{}, err
	}
	model, err := normalizeModel(modelVersion)
	if err != nil {
		return Run{}, err
	}
	r.Status, r.ModelVersion, r.LeaseExpiresAt, r.FailureReason, r.FinishedAt = Succeeded, model, nil, "", &now
	return r, nil
}

// OutputPrefix is where the current attempt writes its outputs. Every attempt
// has its own prefix, so a late attempt cannot overwrite the winning outputs.
func (r Run) OutputPrefix() string {
	return fmt.Sprintf("runs/%s/attempts/%d/", r.ID, r.Attempt)
}

func normalizeModel(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 120 || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return "", ErrInvalidModelVersion
	}
	return text, nil
}
