package domain

import (
	"errors"
	"slices"
	"testing"
	"time"
)

var (
	rotarod = Paradigm{Key: "ROTAROD", Version: 1, ObserverScored: true, Events: []EventDefinition{{Type: "fall", Kind: Point, Single: true}}}
	novel   = Paradigm{Key: "NOVEL_OBJECT", Version: 1, ObserverScored: true,
		Events: []EventDefinition{{Type: "exploration", Kind: Interval, Labels: []string{"novel", "familiar"}}}}
	pole = Paradigm{Key: "POLE", Version: 1, ObserverScored: true, Events: []EventDefinition{
		{Type: "turn_complete", Kind: Point, Single: true}, {Type: "base_reached", Kind: Point, Single: true}, {Type: "fall", Kind: Point, Single: true}}}
)

func us(v int64) *int64 { return &v }

// A clip from 2 s to 62 s of the video file: event times are relative to its start.
var clip = Clip{RecordingID: "rec-1", StartUs: 2_000_000, EndUs: us(62_000_000)}

func TestEventsAreValidatedAgainstTheParadigmAndTheClip(t *testing.T) {
	for name, tc := range map[string]struct {
		p    Paradigm
		c    Clip
		in   EventInput
		want error
	}{
		"point":                     {rotarod, clip, EventInput{Type: "fall", StartUs: 5_000_000}, nil},
		"point with its end":        {rotarod, clip, EventInput{Type: "fall", StartUs: 5, EndUs: us(5)}, nil},
		"point at the clip end":     {rotarod, clip, EventInput{Type: "fall", StartUs: 60_000_000}, nil},
		"point after the clip end":  {rotarod, clip, EventInput{Type: "fall", StartUs: 60_000_001}, ErrEventOutsideClip},
		"open clip":                 {rotarod, Clip{StartUs: 2_000_000}, EventInput{Type: "fall", StartUs: 900_000_000}, nil},
		"negative time":             {rotarod, clip, EventInput{Type: "fall", StartUs: -1}, ErrInvalidEventTime},
		"point with a length":       {rotarod, clip, EventInput{Type: "fall", StartUs: 5, EndUs: us(6)}, ErrInvalidEventTime},
		"unknown type":              {rotarod, clip, EventInput{Type: "jump", StartUs: 5}, ErrUnknownEventType},
		"label on an unlabelled":    {rotarod, clip, EventInput{Type: "fall", Label: "novel", StartUs: 5}, ErrInvalidLabel},
		"interval":                  {novel, clip, EventInput{Type: "exploration", Label: "novel", StartUs: 1_000_000, EndUs: us(3_000_000)}, nil},
		"interval without a label":  {novel, clip, EventInput{Type: "exploration", StartUs: 1, EndUs: us(3)}, ErrInvalidLabel},
		"interval with a bad label": {novel, clip, EventInput{Type: "exploration", Label: "left", StartUs: 1, EndUs: us(3)}, ErrInvalidLabel},
		"interval without an end":   {novel, clip, EventInput{Type: "exploration", Label: "novel", StartUs: 1}, ErrInvalidEventTime},
		"empty interval":            {novel, clip, EventInput{Type: "exploration", Label: "novel", StartUs: 3, EndUs: us(3)}, ErrInvalidEventTime},
		"interval past the clip":    {novel, clip, EventInput{Type: "exploration", Label: "novel", StartUs: 59_000_000, EndUs: us(61_000_000)}, ErrEventOutsideClip},
	} {
		t.Run(name, func(t *testing.T) {
			o, err := NewObservation(tc.p, tc.c, tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if err == nil && (o.Kind != tc.p.Events[0].Kind || (o.Kind == Point && o.EndUs != o.StartUs)) {
				t.Fatalf("observation: %+v", o)
			}
		})
	}
}

func TestSingleEventsAreScoredOnce(t *testing.T) {
	fall := Observation{ID: "a", Type: "fall", Kind: Point, StartUs: 5, EndUs: 5}
	if err := CheckCount(rotarod, []Observation{fall}, Observation{Type: "fall", Kind: Point}); !errors.Is(err, ErrSingleEvent) {
		t.Fatalf("second fall: %v", err)
	}
	if err := CheckCount(rotarod, nil, fall); err != nil {
		t.Fatalf("first fall: %v", err)
	}
	explore := Observation{Type: "exploration", Kind: Interval, Label: "novel", StartUs: 1, EndUs: 2}
	if err := CheckCount(novel, []Observation{explore, explore}, explore); err != nil {
		t.Fatalf("repeated interval: %v", err)
	}
	many := make([]Observation, MaxEvents)
	if err := CheckCount(novel, many, explore); !errors.Is(err, ErrTooManyEvents) {
		t.Fatalf("too many: %v", err)
	}
}

func TestPrepareFixesScoredTypesAndTheRecordingLength(t *testing.T) {
	fall := Observation{Type: "fall", Kind: Point, StartUs: 30_000_000, EndUs: 30_000_000}
	s, err := Prepare(rotarod, clip, []Observation{fall}, CompleteInput{})
	if err != nil || !slices.Equal(s.ScoredTypes, []string{"fall"}) || s.DurationUs != 60_000_000 ||
		len(s.Events) != 1 || s.Events[0].StartUs != 30_000_000 {
		t.Fatalf("defaults: %+v %v", s, err)
	}
	if s, err := Prepare(rotarod, Clip{StartUs: 2_000_000}, nil, CompleteInput{}); err != nil || s.DurationUs != 0 || len(s.Events) != 0 {
		t.Fatalf("open clip: %+v %v", s, err)
	}
	if s, err := Prepare(rotarod, clip, []Observation{fall}, CompleteInput{DurationUs: us(40_000_000)}); err != nil || s.DurationUs != 40_000_000 {
		t.Fatalf("stated length: %+v %v", s, err)
	}
	if s, err := Prepare(pole, clip, nil, CompleteInput{ScoredTypes: []string{"turn_complete"}}); err != nil || !slices.Equal(s.ScoredTypes, []string{"turn_complete"}) {
		t.Fatalf("partial scoring: %+v %v", s, err)
	}
	for name, tc := range map[string]struct {
		p    Paradigm
		obs  []Observation
		in   CompleteInput
		want error
	}{
		"unknown scored type":       {rotarod, nil, CompleteInput{ScoredTypes: []string{"jump"}}, ErrUnknownScoredType},
		"repeated scored type":      {pole, nil, CompleteInput{ScoredTypes: []string{"fall", "fall"}}, ErrUnknownScoredType},
		"event of unscored type":    {pole, []Observation{{Type: "fall", Kind: Point}}, CompleteInput{ScoredTypes: []string{"turn_complete"}}, ErrUnscoredEvent},
		"zero length":               {rotarod, nil, CompleteInput{DurationUs: us(0)}, ErrInvalidDuration},
		"length beyond the clip":    {rotarod, nil, CompleteInput{DurationUs: us(60_000_001)}, ErrInvalidDuration},
		"event after the length":    {rotarod, []Observation{fall}, CompleteInput{DurationUs: us(20_000_000)}, ErrEventAfterDuration},
		"event outside the clip":    {rotarod, []Observation{{Type: "fall", Kind: Point, StartUs: 61_000_000, EndUs: 61_000_000}}, CompleteInput{}, ErrEventOutsideClip},
		"single event scored twice": {rotarod, []Observation{fall, fall}, CompleteInput{}, ErrSingleEvent},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Prepare(tc.p, clip, tc.obs, tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestSheetVersions(t *testing.T) {
	first, err := Next(nil, "e", "t", "ts", "u")
	if err != nil || first.Version != 1 || first.Status != Draft {
		t.Fatalf("first: %+v %v", first, err)
	}
	if _, err := Next(&first, "e", "t", "ts", "u"); !errors.Is(err, ErrNotCompleted) {
		t.Fatalf("reopen a draft: %v", err)
	}
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	done, err := first.Complete(Scoring{ScoredTypes: []string{"fall"}, DurationUs: 60_000_000}, "rec-1", "run-1", "u", at)
	if err != nil || done.Status != Completed || *done.DurationUs != 60_000_000 || done.RunID != "run-1" || done.RecordingID != "rec-1" {
		t.Fatalf("complete: %+v %v", done, err)
	}
	if _, err := done.Complete(Scoring{}, "rec-1", "run-2", "u", at); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("complete twice: %v", err)
	}
	if unknown, _ := first.Complete(Scoring{ScoredTypes: []string{}}, "rec-1", "run-1", "u", at); unknown.DurationUs != nil {
		t.Fatalf("unknown length: %+v", unknown)
	}
	second, err := Next(&done, "e", "t", "ts", "v")
	if err != nil || second.Version != 2 || second.Status != Draft || second.CreatedBy != "v" {
		t.Fatalf("reopen: %+v %v", second, err)
	}
}
