package domain

import (
	"cmp"
	"errors"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

var (
	ErrMissingMetric        = errors.New("every metric of the paradigm version must be reported")
	ErrUnknownMetric        = errors.New("metric is not defined by the paradigm version")
	ErrInvalidMetric        = errors.New("a metric needs either a finite value within its range, integral for counts, or a known missing reason")
	ErrInvalidEvent         = errors.New("events need a type of the paradigm version and a confidence from 0 to 1")
	ErrEventOutOfRange      = errors.New("intervals need 0 <= start < end <= recording duration and points start = end within it")
	ErrUnknownTrial         = errors.New("an event references a trial of another test")
	ErrOverlappingEvents    = errors.New("intervals of the same type must not overlap")
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

const maxEvents = 100_000

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

// Contract is what a paradigm version expects from an analysis.
type Contract struct {
	MetricEngineVersion int
	ResultSchemaVersion int
	CalibrationRequired bool
	Metrics             []MetricSpec
	Events              []EventSpec
	MissingReasons      []string
}

type MetricInput struct {
	Key           string
	Value         *float64
	MissingReason string
}

// EventInput times are microseconds relative to the recording (clip) start.
type EventInput struct {
	Type           string
	StartUs, EndUs int64
	Confidence     float64
	TrialID        string
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

type Submission struct {
	WorkerID     string
	Attempt      int
	ModelVersion string
	// RecordingDurationUs is required when the clip has no end.
	RecordingDurationUs int64
	Metrics             []MetricInput
	Events              []EventInput
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
	Metrics      []MetricValue
	Events       []Event
	Artifacts    []ArtifactInput
	Pair         PairInput
}

// Validate checks a submission against the run and the paradigm contract.
// trialIDs are the trials of the run's test.
func Validate(r Run, c Contract, s Submission, trialIDs []string) (Result, error) {
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
	metrics, err := validateMetrics(c, s.Metrics)
	if err != nil {
		return Result{}, err
	}
	events, err := validateEvents(c, s.Events, duration, trialIDs)
	if err != nil {
		return Result{}, err
	}
	if err := validateArtifacts(r, s); err != nil {
		return Result{}, err
	}
	return Result{ModelVersion: model, DurationUs: duration, Metrics: metrics, Events: events, Artifacts: slices.Clone(s.Artifacts), Pair: s.Pair}, nil
}

func validateMetrics(c Contract, inputs []MetricInput) ([]MetricValue, error) {
	byKey := map[string]MetricInput{}
	for _, m := range inputs {
		if !slices.ContainsFunc(c.Metrics, func(spec MetricSpec) bool { return spec.Key == m.Key }) {
			return nil, ErrUnknownMetric
		}
		if _, seen := byKey[m.Key]; seen {
			return nil, ErrInvalidMetric
		}
		byKey[m.Key] = m
	}
	out := make([]MetricValue, 0, len(c.Metrics))
	for _, spec := range c.Metrics {
		m, ok := byKey[spec.Key]
		if !ok {
			return nil, ErrMissingMetric
		}
		switch {
		case m.Value == nil && !slices.Contains(c.MissingReasons, m.MissingReason):
			return nil, ErrInvalidMetric
		case m.Value != nil && m.MissingReason != "":
			return nil, ErrInvalidMetric
		case m.Value != nil && (math.IsNaN(*m.Value) || math.IsInf(*m.Value, 0) || *m.Value < spec.Min || *m.Value > spec.Max):
			return nil, ErrInvalidMetric
		case m.Value != nil && spec.Integer && *m.Value != math.Trunc(*m.Value):
			return nil, ErrInvalidMetric
		}
		out = append(out, MetricValue{Key: spec.Key, Unit: spec.Unit, Value: m.Value, MissingReason: m.MissingReason})
	}
	return out, nil
}

func validateEvents(c Contract, inputs []EventInput, duration int64, trialIDs []string) ([]Event, error) {
	if len(inputs) > maxEvents {
		return nil, ErrInvalidEvent
	}
	out := make([]Event, 0, len(inputs))
	for _, e := range inputs {
		i := slices.IndexFunc(c.Events, func(spec EventSpec) bool { return spec.Type == e.Type })
		if i < 0 || math.IsNaN(e.Confidence) || e.Confidence < 0 || e.Confidence > 1 {
			return nil, ErrInvalidEvent
		}
		kind := c.Events[i].Kind
		switch {
		case e.StartUs < 0 || e.EndUs > duration:
			return nil, ErrEventOutOfRange
		case kind == "POINT" && e.EndUs != e.StartUs:
			return nil, ErrEventOutOfRange
		case kind != "POINT" && e.EndUs <= e.StartUs:
			return nil, ErrEventOutOfRange
		case e.TrialID != "" && !slices.Contains(trialIDs, e.TrialID):
			return nil, ErrUnknownTrial
		}
		out = append(out, Event{Type: e.Type, Kind: kind, StartUs: e.StartUs, EndUs: e.EndUs, Confidence: e.Confidence, TrialID: e.TrialID})
	}
	slices.SortFunc(out, func(a, b Event) int {
		return cmp.Or(cmp.Compare(a.StartUs, b.StartUs), strings.Compare(a.Type, b.Type), cmp.Compare(a.EndUs, b.EndUs))
	})
	lastEnd := map[string]int64{}
	for _, e := range out {
		if e.Kind == "POINT" {
			continue
		}
		if end, ok := lastEnd[e.Type]; ok && e.StartUs < end {
			return nil, ErrOverlappingEvents
		}
		lastEnd[e.Type] = e.EndUs
	}
	return out, nil
}

func validateArtifacts(r Run, s Submission) error {
	prefix, names, videos := r.OutputPrefix(), map[string]bool{}, 0
	for _, a := range s.Artifacts {
		if (a.Kind != AnalyzedVideo && a.Kind != Trajectory && a.Kind != Thumbnail) || names[a.ObjectName] ||
			!strings.HasPrefix(a.ObjectName, prefix) || len(a.ObjectName) == len(prefix) || strings.Contains(a.ObjectName[len(prefix):], "..") {
			return ErrInvalidArtifact
		}
		names[a.ObjectName] = true
		if a.Kind == AnalyzedVideo {
			videos++
			if a.ObjectName != s.Pair.AnalyzedObjectName {
				return ErrInvalidPair
			}
		}
	}
	if videos != 1 {
		return ErrMissingAnalyzedVideo
	}
	version := strings.TrimSpace(s.Pair.TimeMappingVersion)
	if s.Pair.SourceOffsetUs != r.ClipStartUs || s.Pair.OutputOffsetUs < 0 || version == "" || utf8.RuneCountInString(version) > 60 {
		return ErrInvalidPair
	}
	return nil
}
