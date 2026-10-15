package domain

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

const (
	// TrajectorySchema names the only trajectory format the API reads.
	TrajectorySchema = "misko.trajectory.v1"
	// TrajectoryContentType is the content type of a trajectory output.
	TrajectoryContentType = "application/json"
	maxSamples            = 5_000_000
)

var (
	ErrInvalidTrajectory = errors.New("the trajectory must be misko.trajectory.v1 JSON for this run, attempt and recording duration, with equal-length sample arrays, strictly increasing times within the recording, confidences from 0 to 1, coordinates for tracked samples and inferred samples only among tracked ones")
	ErrMissingTrajectory = errors.New("a result needs exactly one TRAJECTORY artifact stored as application/json")
)

// Sample is one tracked position in the calibration's centimeter frame. TUs is
// microseconds from the recording (clip) start. Untracked samples carry no
// coordinates. An Inferred sample is tracked, but its position was not seen:
// the worker placed it (a light/dark animal hidden in the dark compartment).
// Metrics count it as tracked; QC reports it separately.
type Sample struct {
	TUs        int64
	X, Y       float64
	Tracked    bool
	Confidence float64
	Inferred   bool
}

// Track is the worker's measurement of a recording; the metric engine
// computes every published metric and event from it.
type Track struct {
	Samples []Sample
}

// trajectoryFile is misko.trajectory.v1. Unknown members, such as worker
// diagnostics, are ignored. samples.inferred is optional: absent means no
// sample was inferred.
type trajectoryFile struct {
	Schema     string `json:"schema"`
	RunID      string `json:"runId"`
	Attempt    int    `json:"attempt"`
	DurationUs int64  `json:"durationUs"`
	Samples    struct {
		TUs        []int64    `json:"tUs"`
		X          []*float64 `json:"xCm"`
		Y          []*float64 `json:"yCm"`
		Tracked    []bool     `json:"tracked"`
		Confidence []float64  `json:"confidence"`
		Inferred   []bool     `json:"inferred"`
	} `json:"samples"`
}

// ParseTrajectory reads a stored trajectory of the run's current attempt.
func ParseTrajectory(data []byte, r Run, durationUs int64) (Track, error) {
	var f trajectoryFile
	if err := json.Unmarshal(data, &f); err != nil {
		return Track{}, fmt.Errorf("%w: %v", ErrInvalidTrajectory, err)
	}
	s := f.Samples
	n := len(s.TUs)
	switch {
	case f.Schema != TrajectorySchema, f.RunID != r.ID, f.Attempt != r.Attempt:
		return Track{}, fmt.Errorf("%w: schema, run or attempt", ErrInvalidTrajectory)
	case f.DurationUs != durationUs:
		return Track{}, fmt.Errorf("%w: duration %d, recording %d", ErrInvalidTrajectory, f.DurationUs, durationUs)
	case n > maxSamples, len(s.X) != n, len(s.Y) != n, len(s.Tracked) != n, len(s.Confidence) != n, s.Inferred != nil && len(s.Inferred) != n:
		return Track{}, fmt.Errorf("%w: sample arrays", ErrInvalidTrajectory)
	}
	out := Track{Samples: make([]Sample, n)}
	for i := range n {
		sample := Sample{TUs: s.TUs[i], Tracked: s.Tracked[i], Confidence: s.Confidence[i]}
		if s.Inferred != nil {
			sample.Inferred = s.Inferred[i]
		}
		switch {
		case sample.TUs < 0 || sample.TUs > durationUs || (i > 0 && sample.TUs <= s.TUs[i-1]):
			return Track{}, fmt.Errorf("%w: sample %d time", ErrInvalidTrajectory, i)
		case sample.Confidence < 0 || sample.Confidence > 1:
			return Track{}, fmt.Errorf("%w: sample %d confidence", ErrInvalidTrajectory, i)
		case sample.Tracked && (s.X[i] == nil || s.Y[i] == nil):
			return Track{}, fmt.Errorf("%w: sample %d coordinates", ErrInvalidTrajectory, i)
		case sample.Inferred && !sample.Tracked:
			return Track{}, fmt.Errorf("%w: sample %d inferred but not tracked", ErrInvalidTrajectory, i)
		}
		if sample.Tracked {
			sample.X, sample.Y = *s.X[i], *s.Y[i]
		}
		out.Samples[i] = sample
	}
	return out, nil
}

// QCRule is a quality rule of the paradigm version.
type QCRule struct {
	Key, Operator string
	Value         float64
}

// Quality summarizes a trajectory for QC. LostRatio is the share of samples
// that are not tracked; InferredRatio the share that are tracked but inferred;
// MeanConfidence the mean confidence of the observed samples (tracked and not
// inferred), so a confidence the worker assigns to a position it did not see
// never raises or lowers it.
type Quality struct {
	Samples, Tracked, Inferred               int
	LostRatio, InferredRatio, MeanConfidence float64
}

// Quality measures the trajectory. Without samples everything counts as lost.
func (t Track) Quality() Quality {
	q := Quality{Samples: len(t.Samples), LostRatio: 1}
	observed, confidence := 0, 0.0
	for _, s := range t.Samples {
		switch {
		case s.Inferred:
			q.Tracked++
			q.Inferred++
		case s.Tracked:
			q.Tracked++
			observed++
			confidence += s.Confidence
		}
	}
	if q.Tracked > 0 {
		q.LostRatio = float64(q.Samples-q.Tracked) / float64(q.Samples)
	}
	if q.Samples > 0 {
		q.InferredRatio = float64(q.Inferred) / float64(q.Samples)
	}
	if observed > 0 {
		q.MeanConfidence = confidence / float64(observed)
	}
	return q
}

// CheckQC applies the rules a trajectory can measure and returns the first
// failure, or an empty string. min_tracking_confidence is compared with the mean
// confidence of observed samples, max_lost_frame_ratio with the share of
// samples that are not tracked (inferred samples are tracked) and, when a
// paradigm sets it, max_inferred_ratio with the share of inferred samples. A
// trajectory without tracked samples fails the first two. Other rules, such as
// the calibration error, are checked where their inputs live.
func CheckQC(rules []QCRule, t Track) string {
	q := t.Quality()
	for _, r := range rules {
		var measured float64
		switch r.Key {
		case "min_tracking_confidence":
			measured = q.MeanConfidence
		case "max_lost_frame_ratio":
			measured = q.LostRatio
		case "max_inferred_ratio":
			measured = q.InferredRatio
		default:
			continue
		}
		if !holds(measured, r.Operator, r.Value) {
			return fmt.Sprintf("QC_FAILED: %s %.4g %s %.4g not met", r.Key, measured, r.Operator, r.Value)
		}
	}
	return ""
}

func holds(measured float64, operator string, limit float64) bool {
	switch operator {
	case ">=":
		return measured >= limit
	case "<=":
		return measured <= limit
	}
	return false
}

// Interval and Point are events computed by the metric engine, in microseconds
// from the recording start.
type Interval struct {
	Type           string
	StartUs, EndUs int64
}

type Point struct {
	Type string
	AtUs int64
}

// Computed is the metric engine's result for a trajectory.
type Computed struct {
	Metrics   []MetricValue
	Intervals []Interval
	Points    []Point
}

// Events returns the computed events in publication order. The confidence of an
// event is the mean confidence of the tracked samples with start <= t <= end,
// or 0 when none is tracked.
func (t Track) Events(c Computed) []Event {
	out := make([]Event, 0, len(c.Intervals)+len(c.Points))
	for _, i := range c.Intervals {
		out = append(out, Event{Type: i.Type, Kind: "INTERVAL", StartUs: i.StartUs, EndUs: i.EndUs, Confidence: t.confidence(i.StartUs, i.EndUs)})
	}
	for _, p := range c.Points {
		out = append(out, Event{Type: p.Type, Kind: "POINT", StartUs: p.AtUs, EndUs: p.AtUs, Confidence: t.confidence(p.AtUs, p.AtUs)})
	}
	slices.SortStableFunc(out, func(a, b Event) int {
		return cmp.Or(cmp.Compare(a.StartUs, b.StartUs), strings.Compare(a.Type, b.Type))
	})
	return out
}

func (t Track) confidence(start, end int64) float64 {
	first := sort.Search(len(t.Samples), func(i int) bool { return t.Samples[i].TUs >= start })
	sum, n := 0.0, 0
	for _, s := range t.Samples[first:] {
		if s.TUs > end {
			break
		}
		if s.Tracked {
			sum += s.Confidence
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}
