package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidArtifact      = errors.New("artifacts need a known kind and a unique object under the attempt's output prefix")
	ErrMissingAnalyzedVideo = errors.New("a result needs exactly one analyzed video")
	ErrInvalidPair          = errors.New("the video pair must name the analyzed video, offset the source by the clip start and have a time mapping version")
	ErrInvalidDuration      = errors.New("recording duration must be positive and match the clip")
)

// Artifact kinds.
const (
	AnalyzedVideo = "ANALYZED_VIDEO"
	Trajectory    = "TRAJECTORY"
	Thumbnail     = "THUMBNAIL"
)

type MetricSpec struct {
	Key, Unit string
	Integer   bool
	Min, Max  float64
}

type EventSpec struct {
	Type string
	// Kind is INTERVAL or POINT.
	Kind string
}

// Contract is what a paradigm version expects from an analysis. Metrics and
// events are computed by the metric engine from the trajectory; they are listed
// so workers can draw and label them.
type Contract struct {
	MetricEngineVersion int
	ResultSchemaVersion int
	CalibrationRequired bool
	// TrajectoryAnalyzable is false when the engine needs inputs a trajectory
	// cannot provide, such as calibrated zone shapes or scored observations.
	TrajectoryAnalyzable bool
	Metrics              []MetricSpec
	Events               []EventSpec
	MissingReasons       []string
	QC                   []QCRule
}

// CalibrationRef is the pinned calibration a worker needs to map full-frame
// pixels to arena centimeters.
type CalibrationRef struct {
	ID                                  string
	FrameWidth, FrameHeight             int
	CropX, CropY, CropWidth, CropHeight int
	Plane                               string
	Transform                           [9]float64
}

type ArtifactInput struct {
	Kind, ObjectName string
}

// PairInput maps the analyzed video onto its source: source time =
// SourceOffsetUs + recording time, output time = OutputOffsetUs + recording time.
type PairInput struct {
	AnalyzedObjectName string
	SourceOffsetUs     int64
	OutputOffsetUs     int64
	TimeMappingVersion string
}

// Submission is a worker's result. It carries no metrics or events: the API
// computes them from the stored trajectory.
type Submission struct {
	WorkerID     string
	Attempt      int
	ModelVersion string
	// RecordingDurationUs is required when the clip has no end.
	RecordingDurationUs int64
	Artifacts           []ArtifactInput
	Pair                PairInput
}

type MetricValue struct {
	Key, Unit     string
	Value         *float64
	MissingReason string
}

type Event struct {
	Type, Kind     string
	StartUs, EndUs int64
	Confidence     float64
	TrialID        string
}

type Result struct {
	ModelVersion string
	DurationUs   int64
	// TrajectoryObject is the stored trajectory the metrics are computed from.
	TrajectoryObject string
	Metrics          []MetricValue
	Events           []Event
	Artifacts        []ArtifactInput
	Pair             PairInput
}

// Validate checks the shape of a submission against its run. Metrics and
// events are added from the metric engine after the outputs are verified.
func Validate(r Run, s Submission) (Result, error) {
	model, err := normalizeModel(s.ModelVersion)
	if err != nil {
		return Result{}, err
	}
	duration := s.RecordingDurationUs
	if r.ClipEndUs != nil {
		clip := *r.ClipEndUs - r.ClipStartUs
		if duration != 0 && duration != clip {
			return Result{}, ErrInvalidDuration
		}
		duration = clip
	}
	if duration <= 0 {
		return Result{}, ErrInvalidDuration
	}
	trajectory, err := validateArtifacts(r, s)
	if err != nil {
		return Result{}, err
	}
	return Result{ModelVersion: model, DurationUs: duration, TrajectoryObject: trajectory, Artifacts: append([]ArtifactInput(nil), s.Artifacts...), Pair: s.Pair}, nil
}

func validateArtifacts(r Run, s Submission) (string, error) {
	prefix, names, videos := r.OutputPrefix(), map[string]bool{}, 0
	var trajectories []string
	for _, a := range s.Artifacts {
		if (a.Kind != AnalyzedVideo && a.Kind != Trajectory && a.Kind != Thumbnail) || names[a.ObjectName] ||
			!strings.HasPrefix(a.ObjectName, prefix) || len(a.ObjectName) == len(prefix) || strings.Contains(a.ObjectName[len(prefix):], "..") {
			return "", ErrInvalidArtifact
		}
		names[a.ObjectName] = true
		switch a.Kind {
		case AnalyzedVideo:
			videos++
			if a.ObjectName != s.Pair.AnalyzedObjectName {
				return "", ErrInvalidPair
			}
		case Trajectory:
			trajectories = append(trajectories, a.ObjectName)
		}
	}
	if videos != 1 {
		return "", ErrMissingAnalyzedVideo
	}
	if len(trajectories) != 1 {
		return "", ErrMissingTrajectory
	}
	version := strings.TrimSpace(s.Pair.TimeMappingVersion)
	if s.Pair.SourceOffsetUs != r.ClipStartUs || s.Pair.OutputOffsetUs < 0 || version == "" || utf8.RuneCountInString(version) > 60 {
		return "", ErrInvalidPair
	}
	return trajectories[0], nil
}
