package domain

import "time"

type Trigger string

const (
	Automatic Trigger = "AUTOMATIC"
	Manual    Trigger = "MANUAL"
)

// Link records that an analysis job analysed a trial: from which of its
// videos, with which effective calibration and which ANALYSIS overrides.
type Link struct {
	JobID         string
	TrialID       string
	VideoAssetID  string
	CalibrationID string
	Trigger       Trigger
	Overrides     map[string]float64
	CreatedBy     string
	CreatedAt     time.Time
}

// Job is what a trial reads about an analysis job.
type Job struct {
	ID              string
	Status          string
	ParadigmKey     string
	ParadigmVersion int
	Parameters      map[string]float64
	SourceAssetID   string
	CalibrationID   string
	Attempt         int
	MaxAttempts     int
	ModelVersion    string
	FailureReason   string
	CreatedAt       time.Time
	FinishedAt      *time.Time
}

// Metric is one published measurement of a job.
type Metric struct {
	Key, Unit     string
	Value         *float64
	MissingReason string
}

// QC is the tracking quality of the trajectory a job measured.
type QC struct {
	Samples, Tracked, Inferred               int
	LostRatio, InferredRatio, MeanConfidence float64
	Passed                                   bool
}

// Result is a job with what it published so far.
type Result struct {
	Job     Job
	Metrics []Metric
	QC      *QC
}

// Analysis is one analysis of a trial: the trial's link and the job's result.
type Analysis struct {
	Link
	Result
}

// Video is what a trial reads about its video.
type Video struct {
	ID              string
	FileName        string
	ContentType     string
	SizeBytes       int64
	Status          string
	RejectionReason string
	CreatedAt       time.Time
	VerifiedAt      *time.Time
}

// VideoVerified is the status of a verified video.
const VideoVerified = "VERIFIED"

// UploadRequest is what the uploader declares about a trial video.
type UploadRequest struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	// CRC32C is the base64 encoding of the big-endian checksum.
	CRC32C string
	Clip   Clip
}

// SignedUpload is the request the uploader sends straight to storage.
type SignedUpload struct {
	Method    string
	URL       string
	Headers   map[string]string
	ExpiresAt time.Time
}

// Revision is the arena a trial runs in.
type Revision struct {
	ID              string
	EnvironmentID   string
	EnvironmentName string
	Number          int
	ParadigmKey     string
	ParadigmVersion int
	Parameters      map[string]float64
}

// JobRequest is what a trial asks the analysis service for.
type JobRequest struct {
	TrialID         string
	ParadigmKey     string
	ParadigmVersion int
	Parameters      map[string]float64
	SourceAssetID   string
	Clip            Clip
	CalibrationID   string
	Trigger         Trigger
	IdempotencyKey  string
	CreatedBy       string
}
