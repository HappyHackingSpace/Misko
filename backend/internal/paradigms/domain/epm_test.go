package domain

import (
	"math"
	"testing"
)

func epmZones() map[string][]Shape {
	return map[string][]Shape{
		"center":      {rect(-3, -3, 3, 3)},
		"open_arms":   {rect(3, -3, 38, 3), rect(-38, -3, -3, 3)},
		"closed_arms": {rect(-3, 3, 3, 38), rect(-3, -38, 3, -3)},
	}
}

// Counted intervals 0 to 2.5 s (2.5 s); 2.5-3 and 3-3.5 touch a lost frame.
// Earlier samples: center at 0 and 1.5 (1.0 s), open at 0.5 and 1.0 (1.0 s), closed at 2.0 (0.5 s).
// Entries: open at 0.5 s (from center), closed at 2.0 s (from center). First open-arm sample: 0.5 s.
func TestElevatedPlusMazeHandCalculated(t *testing.T) {
	in := Input{
		Zones:        epmZones(),
		ScoredEvents: []string{"risk_assessment"},
		Events:       []ObservedEvent{mark("risk_assessment", 1.2), mark("risk_assessment", 2.2)},
		Samples: []Sample{
			sample(0, 0, 0), sample(0.5, 10, 0), sample(1, 20, 0), sample(1.5, 0, 1), sample(2, 0, 20), sample(2.5, 0, 30), lost(3), sample(3.5, 0, 10),
		},
	}
	r := run(t, "EPM", in)
	expectValue(t, r, "duration_s", 2.5)
	expectValue(t, r, "distance_cm", 10+10+math.Hypot(20, 1)+19+10)
	expectValue(t, r, "center_time_s", 1)
	expectValue(t, r, "open_arm_time_s", 1)
	expectValue(t, r, "closed_arm_time_s", 0.5)
	expectValue(t, r, "open_arm_time_ratio", 0.4)
	expectValue(t, r, "closed_arm_time_ratio", 0.2)
	expectValue(t, r, "open_arm_entries_count", 1)
	expectValue(t, r, "closed_arm_entries_count", 1)
	expectValue(t, r, "latency_to_open_arm_s", 0.5)
	expectValue(t, r, "risk_assessment_count", 2)

	in.ScoredEvents, in.Events = nil, nil
	unscored := run(t, "EPM", in)
	expectMissing(t, unscored, "risk_assessment_count", ReasonNotScored)
	expectValue(t, unscored, "open_arm_time_s", 1)

	in.Samples = []Sample{sample(0, 0, 0), sample(0.5, 0, 10)}
	expectMissing(t, run(t, "EPM", in), "latency_to_open_arm_s", ReasonNotObserved)
}

func TestCalibratedZonesAndObservedEventsAreValidated(t *testing.T) {
	samples := []Sample{sample(0, 0, 0), sample(0.5, 10, 0)}
	with := func(mutate func(*Input)) Input {
		in := Input{Zones: epmZones(), Samples: samples}
		mutate(&in)
		return in
	}
	for name, tc := range map[string]struct {
		key  string
		in   Input
		want error
	}{
		"missing required zone": {"EPM", with(func(in *Input) { delete(in.Zones, "center") }), ErrMissingZone},
		"empty required zone":   {"EPM", with(func(in *Input) { in.Zones["center"] = nil }), ErrMissingZone},
		"empty shape list":      {"EPM", with(func(in *Input) { in.Zones["center"] = []Shape{} }), ErrMissingZone},
		"unknown zone":          {"EPM", with(func(in *Input) { in.Zones["tail"] = []Shape{rect(0, 0, 1, 1)} }), ErrUnknownZone},
		"two-vertex polygon":    {"EPM", with(func(in *Input) { in.Zones["center"] = []Shape{{Polygon: []Vertex{{0, 0}, {1, 1}}}} }), ErrInvalidZone},
		"NaN vertex":            {"EPM", with(func(in *Input) { in.Zones["center"] = []Shape{rect(0, 0, math.NaN(), 1)} }), ErrInvalidZone},
		"zero radius":           {"EPM", with(func(in *Input) { in.Zones["center"] = []Shape{circle(0, 0, 0)} }), ErrInvalidZone},
		"circle and polygon together": {"EPM", with(func(in *Input) {
			in.Zones["center"] = []Shape{{Circle: &Circle{Radius: 1}, Polygon: rect(0, 0, 1, 1).Polygon}}
		}), ErrInvalidZone},
		"event type not scored":  {"EPM", with(func(in *Input) { in.Events = []ObservedEvent{mark("risk_assessment", 1)} }), ErrInvalidEvent},
		"undeclared scored type": {"EPM", with(func(in *Input) { in.ScoredEvents = []string{"grooming"} }), ErrInvalidEvent},
		"point event with a duration": {"EPM", with(func(in *Input) {
			in.ScoredEvents = []string{"risk_assessment"}
			in.Events = []ObservedEvent{span("risk_assessment", "", 1, 2)}
		}), ErrInvalidEvent},
		"event after recording end": {"EPM", with(func(in *Input) {
			in.ScoredEvents = []string{"risk_assessment"}
			in.DurationUs = at(1)
			in.Events = []ObservedEvent{mark("risk_assessment", 2)}
		}), ErrInvalidEvent},
		"negative recording length": {"EPM", with(func(in *Input) { in.DurationUs = -1 }), ErrInvalidEvent},
		"unexpected label": {"EPM", with(func(in *Input) {
			in.ScoredEvents = []string{"risk_assessment"}
			in.Events = []ObservedEvent{{Type: "risk_assessment", Label: "left", StartUs: 1, EndUs: 1}}
		}), ErrInvalidEvent},
		"zone for a derived-zone test": {"OPEN_FIELD", Input{Zones: map[string][]Shape{"center": {rect(0, 0, 1, 1)}}}, ErrUnknownZone},
	} {
		t.Run(name, func(t *testing.T) { expectError(t, tc.key, tc.in, tc.want) })
	}
}
