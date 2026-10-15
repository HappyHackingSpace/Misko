// Package domain holds observer scoring: the events a person marks on the
// video of one animal of a test (a test subject) for a paradigm the engine
// computes from observations, collected on a versioned scoring sheet.
// Observation times are microseconds from the clip start of the test's
// current video (0 is the clip start), which is also the trial time the
// engine consumes.
package domain

import (
	"errors"
	"slices"
	"time"
)

var (
	ErrUnknownEventType   = errors.New("the event type is not an input event of the paradigm")
	ErrInvalidEventTime   = errors.New("a point event needs one time and an interval event an end after its start, both from 0")
	ErrInvalidLabel       = errors.New("the label must be one of the event's labels, and events without labels take none")
	ErrEventOutsideClip   = errors.New("the event must lie within the clip of the test's current video")
	ErrSingleEvent        = errors.New("this event may be scored at most once")
	ErrTooManyEvents      = errors.New("a scoring sheet holds at most 10000 events")
	ErrUnknownScoredType  = errors.New("scored event types must be input events of the paradigm, each named once")
	ErrUnscoredEvent      = errors.New("every event must be of a scored event type")
	ErrInvalidDuration    = errors.New("the recording length must be positive and within the clip of the current video")
	ErrEventAfterDuration = errors.New("an event ends after the recording length")
	ErrNotDraft           = errors.New("the scoring sheet is completed; reopen it to change the events")
	ErrNotCompleted       = errors.New("only a completed scoring sheet can be reopened")
	ErrNotObserverScored  = errors.New("the paradigm is analysed from the video by a worker, not scored by an observer")
)

// MaxEvents caps the observations of one sheet.
const MaxEvents = 10000

type Kind string

const (
	Interval Kind = "INTERVAL"
	Point    Kind = "POINT"
)

type Status string

const (
	Draft     Status = "DRAFT"
	Completed Status = "COMPLETED"
)

// Text is user-facing wording in English and Turkish.
type Text struct{ EN, TR string }

// EventDefinition is an input event of a paradigm. A single event occurs at
// most once; when Labels are set, every event carries one of them.
type EventDefinition struct {
	Type       string
	Kind       Kind
	Labels     []string
	Single     bool
	Label      Text
	Definition Text
}

// Paradigm is what scoring needs from a paradigm version.
type Paradigm struct {
	Key            string
	Version        int
	ObserverScored bool
	Events         []EventDefinition
}

func (p Paradigm) event(typ string) (EventDefinition, bool) {
	i := slices.IndexFunc(p.Events, func(e EventDefinition) bool { return e.Type == typ })
	if i < 0 {
		return EventDefinition{}, false
	}
	return p.Events[i], true
}

// Types lists the paradigm's input event types in catalog order.
func (p Paradigm) Types() []string {
	out := make([]string, 0, len(p.Events))
	for _, e := range p.Events {
		out = append(out, e.Type)
	}
	return out
}

// Clip is the part of the test's current video the trial covers, in
// microseconds from the start of the video file. EndUs is nil when the clip
// runs to the end of the video.
type Clip struct {
	RecordingID string
	StartUs     int64
	EndUs       *int64
}

// Length is the clip's length, or 0 when its end is unknown.
func (c Clip) Length() int64 {
	if c.EndUs == nil {
		return 0
	}
	return *c.EndUs - c.StartUs
}

// contains reports whether clip-relative times lie within the clip.
func (c Clip) contains(start, end int64) bool {
	return start >= 0 && (c.EndUs == nil || end <= c.Length())
}

// EventInput is an event as a scorer marks it. EndUs may be omitted for a
// point event.
type EventInput struct {
	Type    string
	Label   string
	StartUs int64
	EndUs   *int64
}

// Observation is one scored event on a sheet.
type Observation struct {
	ID            string
	SheetID       string
	TestSubjectID string
	Type          string
	Kind          Kind
	Label         string
	StartUs       int64
	EndUs         int64
	ScoredBy      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewObservation validates an event against the paradigm's definition and
// the clip of the current video.
func NewObservation(p Paradigm, clip Clip, in EventInput) (Observation, error) {
	def, ok := p.event(in.Type)
	if !ok {
		return Observation{}, ErrUnknownEventType
	}
	end := in.StartUs
	if in.EndUs != nil {
		end = *in.EndUs
	}
	switch {
	case in.StartUs < 0,
		def.Kind == Point && end != in.StartUs,
		def.Kind == Interval && (in.EndUs == nil || end <= in.StartUs):
		return Observation{}, ErrInvalidEventTime
	case len(def.Labels) == 0 && in.Label != "",
		len(def.Labels) > 0 && !slices.Contains(def.Labels, in.Label):
		return Observation{}, ErrInvalidLabel
	case !clip.contains(in.StartUs, end):
		return Observation{}, ErrEventOutsideClip
	}
	return Observation{Type: def.Type, Kind: def.Kind, Label: in.Label, StartUs: in.StartUs, EndUs: end}, nil
}

// CheckCount rejects a single event scored twice and sheets over MaxEvents.
// others are the sheet's other observations; o is the one being added or changed.
func CheckCount(p Paradigm, others []Observation, o Observation) error {
	if len(others) >= MaxEvents {
		return ErrTooManyEvents
	}
	def, _ := p.event(o.Type)
	if def.Single && slices.ContainsFunc(others, func(x Observation) bool { return x.Type == o.Type }) {
		return ErrSingleEvent
	}
	return nil
}

// Sheet is one version of the scoring of a test subject.
type Sheet struct {
	ID            string
	ExperimentID  string
	TestID        string
	TestSubjectID string
	Version       int
	Status        Status
	// ScoredTypes, DurationUs and RecordingID are fixed at completion.
	ScoredTypes []string
	DurationUs  *int64
	RecordingID string
	RunID       string
	CreatedBy   string
	CreatedAt   time.Time
	CompletedBy string
	CompletedAt *time.Time
}

// Next is the draft that follows this sheet: the first one when prev is nil,
// otherwise the next version after a completed sheet.
func Next(prev *Sheet, experimentID, testID, testSubjectID, createdBy string) (Sheet, error) {
	version := 1
	if prev != nil {
		if prev.Status != Completed {
			return Sheet{}, ErrNotCompleted
		}
		version = prev.Version + 1
	}
	return Sheet{ExperimentID: experimentID, TestID: testID, TestSubjectID: testSubjectID, Version: version, Status: Draft, CreatedBy: createdBy}, nil
}

// TrialEvent is an observation in microseconds from the clip start, as the
// engine consumes it.
type TrialEvent struct {
	Type, Label    string
	StartUs, EndUs int64
}

// Scoring is a validated completion: what the engine computes from.
type Scoring struct {
	ScoredTypes []string
	// DurationUs is the recording length, or 0 when unknown.
	DurationUs int64
	Events     []TrialEvent
}

// CompleteInput is what the scorer states when completing a sheet. Nil
// ScoredTypes means every input event of the paradigm was scored; a nil
// DurationUs takes the clip length of the current video.
type CompleteInput struct {
	ScoredTypes []string
	DurationUs  *int64
}

// Prepare validates the observations of a draft for completion against the
// paradigm and the clip of the current video.
func Prepare(p Paradigm, clip Clip, observations []Observation, in CompleteInput) (Scoring, error) {
	scored := in.ScoredTypes
	if scored == nil {
		scored = p.Types()
	}
	seen := map[string]bool{}
	for _, typ := range scored {
		if _, ok := p.event(typ); !ok || seen[typ] {
			return Scoring{}, ErrUnknownScoredType
		}
		seen[typ] = true
	}
	duration := clip.Length()
	if in.DurationUs != nil {
		duration = *in.DurationUs
		if duration <= 0 || (clip.EndUs != nil && duration > clip.Length()) {
			return Scoring{}, ErrInvalidDuration
		}
	}
	out := Scoring{ScoredTypes: slices.Clone(scored), DurationUs: duration, Events: []TrialEvent{}}
	for i, o := range observations {
		if !seen[o.Type] {
			return Scoring{}, ErrUnscoredEvent
		}
		checked, err := NewObservation(p, clip, EventInput{Type: o.Type, Label: o.Label, StartUs: o.StartUs, EndUs: &o.EndUs})
		if err != nil {
			return Scoring{}, err
		}
		if err := CheckCount(p, observations[:i], checked); err != nil {
			return Scoring{}, err
		}
		e := TrialEvent{Type: o.Type, Label: o.Label, StartUs: o.StartUs, EndUs: o.EndUs}
		if duration > 0 && e.EndUs > duration {
			return Scoring{}, ErrEventAfterDuration
		}
		out.Events = append(out.Events, e)
	}
	return out, nil
}

// Complete fixes a draft's scored types, recording length and video, and
// links the run computed from it.
func (s Sheet) Complete(scoring Scoring, recordingID, runID, by string, at time.Time) (Sheet, error) {
	if s.Status != Draft {
		return Sheet{}, ErrNotDraft
	}
	s.Status, s.ScoredTypes, s.RecordingID, s.RunID, s.CompletedBy, s.CompletedAt = Completed, slices.Clone(scoring.ScoredTypes), recordingID, runID, by, &at
	s.DurationUs = nil
	if scoring.DurationUs > 0 {
		d := scoring.DurationUs
		s.DurationUs = &d
	}
	return s, nil
}
