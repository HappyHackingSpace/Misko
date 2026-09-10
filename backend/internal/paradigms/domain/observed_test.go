package domain

import (
	"maps"
	"slices"
	"testing"
)

func rotarod(t *testing.T, params map[string]float64, recording float64, scored bool, falls ...float64) Result {
	t.Helper()
	in := Input{Parameters: params, DurationUs: at(recording)}
	if scored {
		in.ScoredEvents = []string{"fall"}
	}
	for _, f := range falls {
		in.Events = append(in.Events, mark("fall", f))
	}
	return run(t, "ROTAROD", in)
}

// Accelerating 4 to 40 rpm over 300 s: a fall at 150 s happens at 4 + 36 x 150/300 = 22 rpm.
// Without a fall the latency is censored at the 300 s cut-off only when the whole trial was recorded.
func TestRotarodHandCalculated(t *testing.T) {
	accel := map[string]float64{"accelerating": 1, "start_rpm": 4, "end_rpm": 40, "acceleration_duration_s": 300, "max_trial_duration_s": 300}
	r := rotarod(t, accel, 300, true, 150)
	expectValue(t, r, "latency_to_fall_s", 150)
	expectValue(t, r, "fall_detected", 1)
	expectValue(t, r, "rpm_at_fall", 22)

	fixed := maps.Clone(accel)
	fixed["accelerating"] = 0
	expectValue(t, rotarod(t, fixed, 300, true, 150), "rpm_at_fall", 4)

	long := maps.Clone(accel)
	long["max_trial_duration_s"] = 600
	expectValue(t, rotarod(t, long, 600, true, 400), "rpm_at_fall", 40)

	full := rotarod(t, accel, 300, true)
	expectValue(t, full, "latency_to_fall_s", 300)
	expectValue(t, full, "fall_detected", 0)
	expectMissing(t, full, "rpm_at_fall", ReasonNotObserved)

	late := rotarod(t, accel, 330, true, 320)
	expectValue(t, late, "latency_to_fall_s", 300)
	expectValue(t, late, "fall_detected", 0)

	short := rotarod(t, accel, 200, true)
	expectMissing(t, short, "latency_to_fall_s", ReasonIncompleteRecording)
	expectMissing(t, short, "fall_detected", ReasonIncompleteRecording)
	expectMissing(t, short, "rpm_at_fall", ReasonIncompleteRecording)

	for _, key := range []string{"latency_to_fall_s", "fall_detected", "rpm_at_fall"} {
		expectMissing(t, rotarod(t, accel, 300, false), key, ReasonNotScored)
	}
	unknownLength := rotarod(t, accel, 0, true, 150)
	expectValue(t, unknownLength, "latency_to_fall_s", 150)

	expectError(t, "ROTAROD", Input{ScoredEvents: []string{"fall"}, Events: []ObservedEvent{mark("fall", 10), mark("fall", 20)}}, ErrInvalidEvent)
	expectError(t, "ROTAROD", Input{Parameters: map[string]float64{"start_rpm": 4, "end_rpm": 3}}, ErrParameterOutOfRange)
	expectError(t, "ROTAROD", Input{Parameters: map[string]float64{"accelerating": 0.5}}, ErrParameterOutOfRange)
}

// Pole of 50 cm: turning completes at 2 s and the base is reached at 6 s, so the descent takes 4 s at 12.5 cm/s.
func TestPoleTestHandCalculated(t *testing.T) {
	all := []string{"turn_complete", "base_reached", "fall"}
	params := map[string]float64{"pole_length_cm": 50}
	r := run(t, "POLE", Input{Parameters: params, ScoredEvents: all, Events: []ObservedEvent{mark("turn_complete", 2), mark("base_reached", 6)}})
	expectValue(t, r, "t_turn_s", 2)
	expectValue(t, r, "t_total_s", 6)
	expectValue(t, r, "descent_time_s", 4)
	expectValue(t, r, "descent_speed_cm_s", 12.5)
	expectValue(t, r, "fall_detected", 0)

	fell := run(t, "POLE", Input{Parameters: params, ScoredEvents: all, Events: []ObservedEvent{mark("turn_complete", 2), mark("fall", 3)}})
	expectMissing(t, fell, "t_total_s", ReasonNotObserved)
	expectMissing(t, fell, "descent_time_s", ReasonNotObserved)
	expectMissing(t, fell, "descent_speed_cm_s", ReasonNotObserved)
	expectValue(t, fell, "fall_detected", 1)

	turnNotScored := run(t, "POLE", Input{Parameters: params, ScoredEvents: []string{"base_reached"}, Events: []ObservedEvent{mark("base_reached", 6)}})
	expectMissing(t, turnNotScored, "t_turn_s", ReasonNotScored)
	expectMissing(t, turnNotScored, "descent_time_s", ReasonNotScored)
	expectMissing(t, turnNotScored, "fall_detected", ReasonNotScored)
	expectValue(t, turnNotScored, "t_total_s", 6)

	instant := run(t, "POLE", Input{Parameters: params, ScoredEvents: all, Events: []ObservedEvent{mark("turn_complete", 4), mark("base_reached", 4)}})
	expectValue(t, instant, "descent_time_s", 0)
	expectMissing(t, instant, "descent_speed_cm_s", ReasonZeroDenominator)

	expectError(t, "POLE", Input{ScoredEvents: all, Events: []ObservedEvent{mark("turn_complete", 5), mark("base_reached", 3)}}, ErrInvalidEvent)
}

// Belt from 5 to 40 cm/s over 1800 s, exhaustion at 900 s:
// distance = 5 x 900 + 35 x 900^2 / (2 x 1800) = 4500 + 7875 = 12375 cm.
// Shocks at 100 and 500 s count; the one at 900 s is not before the end and 1000 s is after it.
func TestTreadmillHandCalculated(t *testing.T) {
	scored := []string{"exhaustion", "shock"}
	params := map[string]float64{"accelerating": 1, "start_speed_cm_s": 5, "end_speed_cm_s": 40, "acceleration_duration_s": 1800, "max_trial_duration_s": 1800}
	r := run(t, "TREADMILL", Input{Parameters: params, ScoredEvents: scored, DurationUs: at(1800), Events: []ObservedEvent{
		mark("shock", 100), mark("shock", 500), mark("exhaustion", 900), mark("shock", 900), mark("shock", 1000),
	}})
	expectValue(t, r, "latency_to_exhaustion_s", 900)
	expectValue(t, r, "run_time_s", 900)
	expectValue(t, r, "run_distance_cm", 12375)
	expectValue(t, r, "shock_count", 2)

	// No exhaustion, full 1800 s recorded, acceleration over 600 s:
	// 5 x 600 + 35 x 600 / 2 + 40 x 1200 = 3000 + 10500 + 48000 = 61500 cm.
	quick := maps.Clone(params)
	quick["acceleration_duration_s"] = 600
	full := run(t, "TREADMILL", Input{Parameters: quick, ScoredEvents: scored, DurationUs: at(3600)})
	expectMissing(t, full, "latency_to_exhaustion_s", ReasonNotObserved)
	expectValue(t, full, "run_time_s", 1800)
	expectValue(t, full, "run_distance_cm", 61500)
	expectValue(t, full, "shock_count", 0)

	fixed := map[string]float64{"accelerating": 0, "start_speed_cm_s": 10, "end_speed_cm_s": 40, "max_trial_duration_s": 1800}
	expectValue(t, run(t, "TREADMILL", Input{Parameters: fixed, ScoredEvents: scored, Events: []ObservedEvent{mark("exhaustion", 100)}}), "run_distance_cm", 1000)

	short := run(t, "TREADMILL", Input{Parameters: params, ScoredEvents: scored, DurationUs: at(1000)})
	expectMissing(t, short, "run_time_s", ReasonIncompleteRecording)
	expectMissing(t, short, "run_distance_cm", ReasonIncompleteRecording)
	expectMissing(t, short, "shock_count", ReasonIncompleteRecording)

	noShockScoring := run(t, "TREADMILL", Input{Parameters: params, ScoredEvents: []string{"exhaustion"}, Events: []ObservedEvent{mark("exhaustion", 900)}})
	expectMissing(t, noShockScoring, "shock_count", ReasonNotScored)
	expectError(t, "TREADMILL", Input{Parameters: map[string]float64{"start_speed_cm_s": 30, "end_speed_cm_s": 20}}, ErrParameterOutOfRange)
}

// Novel exploration [1, 3) and [2.5, 4) merge to 3 s; familiar [5, 6) is 1 s;
// discrimination index (3 - 1) / (3 + 1) = 0.5.
func TestNovelObjectRecognitionHandCalculated(t *testing.T) {
	events := []ObservedEvent{span("exploration", "novel", 1, 3), span("exploration", "novel", 2.5, 4), span("exploration", "familiar", 5, 6)}
	r := run(t, "NOVEL_OBJECT", Input{ScoredEvents: []string{"exploration"}, Events: events})
	expectValue(t, r, "novel_object_time_s", 3)
	expectValue(t, r, "familiar_object_time_s", 1)
	expectValue(t, r, "total_exploration_time_s", 4)
	expectValue(t, r, "discrimination_index", 0.5)

	// [2, 3) lies inside [1, 4) and [4, 5) touches it: one merged interval [1, 5) of 4 s.
	nested := run(t, "NOVEL_OBJECT", Input{ScoredEvents: []string{"exploration"}, Events: []ObservedEvent{
		span("exploration", "novel", 1, 4), span("exploration", "novel", 2, 3), span("exploration", "novel", 4, 5),
	}})
	expectValue(t, nested, "novel_object_time_s", 4)
	if want := []Interval{{Type: "exploration_novel", StartUs: 1e6, EndUs: 5e6}}; !slices.Equal(nested.Intervals, want) {
		t.Fatalf("merged exploration: %+v", nested.Intervals)
	}

	strict := run(t, "NOVEL_OBJECT", Input{Parameters: map[string]float64{"min_total_exploration_s": 5}, ScoredEvents: []string{"exploration"}, Events: events})
	expectValue(t, strict, "total_exploration_time_s", 4)
	expectMissing(t, strict, "discrimination_index", ReasonBelowCriterion)

	none := run(t, "NOVEL_OBJECT", Input{ScoredEvents: []string{"exploration"}})
	expectValue(t, none, "total_exploration_time_s", 0)
	expectMissing(t, none, "discrimination_index", ReasonZeroDenominator)

	for _, key := range []string{"novel_object_time_s", "familiar_object_time_s", "total_exploration_time_s", "discrimination_index"} {
		expectMissing(t, run(t, "NOVEL_OBJECT", Input{}), key, ReasonNotScored)
	}
	expectError(t, "NOVEL_OBJECT", Input{ScoredEvents: []string{"exploration"}, Events: []ObservedEvent{span("exploration", "left", 1, 2)}}, ErrInvalidEvent)
	expectError(t, "NOVEL_OBJECT", Input{ScoredEvents: []string{"exploration"}, Events: []ObservedEvent{span("exploration", "novel", 2, 2)}}, ErrInvalidEvent)
}
