package domain

import (
	"maps"
	"testing"
)

func yMazeZones() map[string][]Shape {
	return map[string][]Shape{"arm_a": {rect(0, 0, 10, 10)}, "arm_b": {rect(20, 0, 30, 10)}, "arm_c": {rect(40, 0, 50, 10)}}
}

// The start arm counts as the first entry: A (start), B, C, A, A, B = 6 entries.
// Triplets ABC and BCA alternate, CAA and AAB do not: 2 / (6 - 2) = 0.5.
// Distances 10, 10, 10, 10, 10, 30, 10, 10, 20 = 120 cm over nine 0.5 s intervals (4.5 s).
// Novel arm B holds only the earlier sample at 1.0 s: 0.5 / 4.5.
func TestYMazeAlternationHandCalculated(t *testing.T) {
	r := run(t, "Y_MAZE", Input{Zones: yMazeZones(), Parameters: map[string]float64{"novel_arm": 2}, Samples: []Sample{
		sample(0, 5, 5), sample(0.5, 15, 5), sample(1, 25, 5), sample(1.5, 35, 5), sample(2, 45, 5),
		sample(2.5, 35, 5), sample(3, 5, 5), sample(3.5, 15, 5), sample(4, 5, 5), sample(4.5, 25, 5),
	}})
	expectValue(t, r, "duration_s", 4.5)
	expectValue(t, r, "distance_cm", 120)
	expectValue(t, r, "total_arm_entries_count", 6)
	expectValue(t, r, "spontaneous_alternation_ratio", 0.5)
	expectValue(t, r, "novel_arm_time_ratio", 0.5/4.5)

	short := run(t, "Y_MAZE", Input{Zones: yMazeZones(), Samples: []Sample{sample(0, 5, 5), sample(0.5, 15, 5)}})
	expectValue(t, short, "total_arm_entries_count", 1)
	expectMissing(t, short, "spontaneous_alternation_ratio", ReasonInsufficientEntries)
	expectMissing(t, short, "novel_arm_time_ratio", ReasonNotApplicable)
	expectError(t, "Y_MAZE", Input{Zones: yMazeZones(), Parameters: map[string]float64{"novel_arm": 1.5}}, ErrParameterOutOfRange)
}

func barnesZones() map[string][]Shape {
	return map[string][]Shape{"target_hole": {circle(0, 40, 3)}, "holes": {circle(40, 0, 3), circle(-40, 0, 3)}}
}

// Non-target hole entries at 0.5 (east), 1.5 (east again), 2.5 (west) and 3.5 (east);
// the target is first reached at 3.0 s, so primary errors are 3 and total errors 4.
func TestBarnesMazeErrorsHandCalculated(t *testing.T) {
	samples := []Sample{
		sample(0, 0, 0), sample(0.5, 38, 0), sample(1, 30, 0), sample(1.5, 38, 1), sample(2, 0, 0),
		sample(2.5, -39, 0), sample(3, 0, 38), sample(3.5, 40, 1),
	}
	r := run(t, "BARNES_MAZE", Input{Zones: barnesZones(), Samples: samples})
	expectValue(t, r, "duration_s", 3.5)
	expectValue(t, r, "primary_latency_s", 3)
	expectValue(t, r, "primary_errors_count", 3)
	expectValue(t, r, "total_errors_count", 4)

	lostTarget := run(t, "BARNES_MAZE", Input{Zones: barnesZones(), Samples: samples[:6]})
	expectMissing(t, lostTarget, "primary_latency_s", ReasonNotObserved)
	expectMissing(t, lostTarget, "primary_errors_count", ReasonNotObserved)
	expectValue(t, lostTarget, "total_errors_count", 3)

	// A hole touching the target at (0, 43): entering both at 0.5 s is not a primary error.
	touching := map[string][]Shape{"target_hole": {circle(0, 40, 3)}, "holes": {circle(0, 46, 3)}}
	both := run(t, "BARNES_MAZE", Input{Zones: touching, Samples: []Sample{sample(0, 0, 0), sample(0.5, 0, 43)}})
	expectValue(t, both, "primary_latency_s", 0.5)
	expectValue(t, both, "primary_errors_count", 0)
	expectValue(t, both, "total_errors_count", 1)

	zones := maps.Clone(barnesZones())
	delete(zones, "holes")
	expectError(t, "BARNES_MAZE", Input{Zones: zones}, ErrMissingZone)
}

func threeChamberZones() map[string][]Shape {
	return map[string][]Shape{
		"social_chamber": {rect(0, 0, 20, 40)}, "center_chamber": {rect(20, 0, 40, 40)}, "object_chamber": {rect(40, 0, 60, 40)},
		"social_interaction": {circle(10, 20, 5)},
	}
}

// Counted intervals 0 to 2.0 s. Earlier samples: center at 0 (0.5 s), social at 0.5, 1.0 and 1.5 (1.5 s),
// of which 0.5 and 1.0 are within the social interaction zone (1.0 s). The object chamber is entered
// at 2.0 s but its next interval touches a lost frame, so object time is 0: index (1.5 - 0) / 1.5 = 1.
func TestThreeChamberSociabilityHandCalculated(t *testing.T) {
	r := run(t, "THREE_CHAMBER", Input{Zones: threeChamberZones(), Samples: []Sample{
		sample(0, 30, 20), sample(0.5, 10, 20), sample(1, 12, 20), sample(1.5, 15, 30), sample(2, 50, 20), lost(2.5), sample(3, 50, 25),
	}})
	expectValue(t, r, "duration_s", 2)
	expectValue(t, r, "social_chamber_time_s", 1.5)
	expectValue(t, r, "object_chamber_time_s", 0)
	expectValue(t, r, "center_chamber_time_s", 0.5)
	expectValue(t, r, "sociability_index", 1)
	expectValue(t, r, "social_interaction_time_s", 1)
	expectMissing(t, r, "object_interaction_time_s", ReasonZoneNotProvided)
	expectValue(t, r, "social_chamber_entries_count", 1)
	expectValue(t, r, "object_chamber_entries_count", 1)

	// Earlier samples social at 0, 0.5 and 1.0 (1.5 s), object at 1.5 (0.5 s): (1.5 - 0.5) / 2 = 0.5.
	both := run(t, "THREE_CHAMBER", Input{Zones: threeChamberZones(), Samples: []Sample{
		sample(0, 10, 20), sample(0.5, 12, 20), sample(1, 11, 20), sample(1.5, 50, 20), sample(2, 51, 20),
	}})
	expectValue(t, both, "object_chamber_time_s", 0.5)
	expectValue(t, both, "sociability_index", 0.5)

	centerOnly := run(t, "THREE_CHAMBER", Input{Zones: threeChamberZones(), Samples: []Sample{sample(0, 30, 20), sample(0.5, 31, 20)}})
	expectMissing(t, centerOnly, "sociability_index", ReasonZeroDenominator)
}

// Box 40 x 20 cm split at x = 20 (the boundary belongs to the dark side).
// Earlier samples: light at 0, 0.5 and 2.0 (1.5 s), dark at 1.0 and 1.5 (1.0 s); 2.5-3.0 leaves the box.
// Compartment changes at 1.0 (L to D), 2.0 (D to L) and 2.5 (L to D).
func TestLightDarkBoxHandCalculated(t *testing.T) {
	params := map[string]float64{"box_width_cm": 40, "box_height_cm": 20, "light_fraction": 0.5}
	r := run(t, "LIGHT_DARK", Input{Parameters: params, Samples: []Sample{
		sample(0, 5, 10), sample(0.5, 19.999, 10), sample(1, 20, 10), sample(1.5, 30, 10), sample(2, 10, 10), sample(2.5, 25, 10), sample(3, 41, 10),
	}})
	expectValue(t, r, "duration_s", 2.5)
	expectValue(t, r, "light_time_s", 1.5)
	expectValue(t, r, "dark_time_s", 1)
	expectValue(t, r, "light_time_ratio", 0.6)
	expectValue(t, r, "transitions_count", 3)
	expectValue(t, r, "light_entries_count", 1)
	expectValue(t, r, "latency_to_dark_s", 1)

	lightOnly := run(t, "LIGHT_DARK", Input{Parameters: params, Samples: []Sample{sample(0, 5, 10), sample(0.5, 6, 10)}})
	expectMissing(t, lightOnly, "latency_to_dark_s", ReasonNotObserved)
	expectValue(t, lightOnly, "dark_time_s", 0)
}
