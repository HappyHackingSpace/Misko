// Package domain holds the experiment side of automatic analysis: which
// analysis job analyses which animal of a test (a test subject), when an
// animal is ready to be analysed, and the human review of each animal's
// result. The jobs themselves belong to the analysis service; this module
// knows them only through the facts its port reports.
package domain

import (
	"maps"
	"time"
)

// JobStatus is an analysis job's state as the analysis service reports it.
type JobStatus string

const (
	JobQueued    JobStatus = "QUEUED"
	JobRunning   JobStatus = "RUNNING"
	JobSucceeded JobStatus = "SUCCEEDED"
	JobFailed    JobStatus = "FAILED"
)

// Pending reports whether the job still waits for or holds a worker.
func (s JobStatus) Pending() bool { return s == JobQueued || s == JobRunning }

type Trigger string

const (
	Automatic Trigger = "AUTOMATIC"
	Manual    Trigger = "MANUAL"
)

// Job is what the experiment side reads about an analysis job.
type Job struct {
	ID                  string
	Status              JobStatus
	ParadigmKey         string
	ParadigmVersion     int
	MetricEngineVersion int
	ResultSchemaVersion int
	Parameters          map[string]float64
	SourceAssetID       string
	SourceGeneration    int64
	SourceCRC32C        uint32
	ClipStartUs         int64
	ClipEndUs           *int64
	CalibrationID       string
	Attempt             int
	MaxAttempts         int
	WorkerID            string
	LeaseExpiresAt      *time.Time
	ModelVersion        string
	FailureReason       string
	CreatedBy           string
	CreatedAt           time.Time
	FinishedAt          *time.Time
}

// Metric is one published measurement of a job.
type Metric struct {
	Key, Unit     string
	Value         *float64
	MissingReason string
}

// Link records that a job analyses one animal of a test, from which of the
// test's videos, with which effective calibration and why.
type Link struct {
	JobID         string
	ExperimentID  string
	TestID        string
	TestSubjectID string
	RecordingID   string
	CalibrationID string
	Trigger       Trigger
	CreatedBy     string
	CreatedAt     time.Time
}

// Run is a linked job with the job's state: one analysis of an animal.
type Run struct {
	Link
	Job Job
}

// Latest returns the most recently finished succeeded run, or "" when none succeeded.
func Latest(runs []Run) string {
	var best *Run
	for i := range runs {
		r := &runs[i]
		if r.Job.Status != JobSucceeded || r.Job.FinishedAt == nil {
			continue
		}
		if best == nil || r.Job.FinishedAt.After(*best.Job.FinishedAt) ||
			(r.Job.FinishedAt.Equal(*best.Job.FinishedAt) && r.JobID > best.JobID) {
			best = r
		}
	}
	if best == nil {
		return ""
	}
	return best.JobID
}

// AnyPending reports whether one of the runs still waits for or holds a worker.
func AnyPending(runs []Run) bool {
	for _, r := range runs {
		if r.Job.Status.Pending() {
			return true
		}
	}
	return false
}

// Requirement is what the paradigm version asks before an animal can be analysed.
type Requirement struct {
	CalibrationRequired bool
	RequiredZones       []string
}

// Candidate is one animal of a test that may be analysed in the test's
// current video. RecordingID is empty while the test has no video and
// CalibrationID is empty while the animal's arena has no valid effective
// calibration (its own override for this video, else the arena default).
type Candidate struct {
	ExperimentID, TestID, TestSubjectID string
	RecordingID, SourceAssetID          string
	ClipStartUs                         int64
	ClipEndUs                           *int64
	CalibrationID                       string
	// ZoneKeys are the calibrated zones the effective calibration has shapes for.
	ZoneKeys         []string
	ParadigmKey      string
	ParadigmVersion  int
	Parameters       map[string]float64
	VideoVerified    bool
	TestCancelled    bool
	ExperimentStatus string
	Excluded         bool
	// Invalid is set when the candidate's stored inputs cannot be read, such
	// as arena parameters that are not numbers; it is never analysed.
	Invalid error
}

// Eligible reports whether the animal's video and study state allow an analysis.
func (c Candidate) Eligible() bool {
	return c.Invalid == nil && c.RecordingID != "" && c.VideoVerified && !c.TestCancelled && !c.Excluded &&
		(c.ExperimentStatus == "RUNNING" || c.ExperimentStatus == "COMPLETED")
}

// Ready reports whether the animal can be analysed with what its paradigm
// requires: when it needs a calibration, a valid effective one with a shape
// for every required zone.
func (c Candidate) Ready(req Requirement) bool {
	if !c.Eligible() {
		return false
	}
	if !req.CalibrationRequired {
		return true
	}
	if c.CalibrationID == "" {
		return false
	}
	for _, zone := range req.RequiredZones {
		found := false
		for _, k := range c.ZoneKeys {
			found = found || k == zone
		}
		if !found {
			return false
		}
	}
	return true
}

// Calibration is the calibration the analysis pins: the effective one when
// the paradigm needs it, none otherwise.
func (c Candidate) Calibration(req Requirement) string {
	if !req.CalibrationRequired {
		return ""
	}
	return c.CalibrationID
}

// AutomaticKey identifies the one automatic job of an animal, video and
// calibration, so concurrent orchestrators submit it once.
func (c Candidate) AutomaticKey(req Requirement) string {
	calibration := c.Calibration(req)
	if calibration == "" {
		calibration = "none"
	}
	return "test-subject:" + c.TestSubjectID + ":recording:" + c.RecordingID + ":calibration:" + calibration
}

// JobRequest is what the experiment side asks the analysis service for.
type JobRequest struct {
	TestSubjectID   string
	ParadigmKey     string
	ParadigmVersion int
	Parameters      map[string]float64
	SourceAssetID   string
	ClipStartUs     int64
	ClipEndUs       *int64
	CalibrationID   string
	Trigger         Trigger
	IdempotencyKey  string
	CreatedBy       string
}

// Request builds the analysis request of a ready candidate.
func (c Candidate) Request(req Requirement, trigger Trigger, createdBy string) JobRequest {
	r := JobRequest{
		TestSubjectID: c.TestSubjectID, ParadigmKey: c.ParadigmKey, ParadigmVersion: c.ParadigmVersion, Parameters: maps.Clone(c.Parameters),
		SourceAssetID: c.SourceAssetID, ClipStartUs: c.ClipStartUs, ClipEndUs: c.ClipEndUs, CalibrationID: c.Calibration(req),
		Trigger: trigger, CreatedBy: createdBy,
	}
	if trigger == Automatic {
		r.IdempotencyKey = c.AutomaticKey(req)
	}
	return r
}

// Link records a submitted job of the candidate.
func (c Candidate) Link(req Requirement, jobID string, trigger Trigger, createdBy string) Link {
	return Link{JobID: jobID, ExperimentID: c.ExperimentID, TestID: c.TestID, TestSubjectID: c.TestSubjectID, RecordingID: c.RecordingID,
		CalibrationID: c.Calibration(req), Trigger: trigger, CreatedBy: createdBy}
}
