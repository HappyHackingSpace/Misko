package domain

import (
	"slices"
	"testing"
)

// Novel object disk r = 4 at (10, 10), familiar square [36, 44] x [6, 14];
// samples every 0.1 s, a run covers contiguous counted intervals whose earlier
// sample is in the zone:
//
//	0.1-0.3 s on the novel object, out at 0.4 s: novel run [0.1, 0.4) = 0.3 s, kept (>= min_bout_s 0.3).
//	0.5-0.6 s on the familiar object, out at 0.7 s: familiar run [0.5, 0.7) = 0.2 s, too short.
//	1.0-1.4 s on the familiar object, out at 1.5 s: familiar run [1.0, 1.5) = 0.5 s.
//	1.6-1.7 s novel, lost frame at 1.8 s, 1.9-2.1 s novel, out at 2.2 s: the lost frame
//	ends the run [1.6, 1.7) (0.1 s, too short); the next run [1.9, 2.2) = 0.3 s is kept.
//
// novel 0.6 s, familiar 0.5 s, total 1.1 s, discrimination index 0.1 / 1.1.
func TestNovelObjectHandCalculated(t *testing.T) {
	trajectory := samples(
		[]Sample{sample(0, 25, 25)},
		[]Sample{sample(0.1, 10, 10), sample(0.2, 11, 10), sample(0.3, 12, 10), sample(0.4, 25, 10)},
		[]Sample{sample(0.5, 40, 10), sample(0.6, 40, 10), sample(0.7, 25, 10), sample(0.8, 25, 20), sample(0.9, 25, 20)},
		steady(1, 1.4, 0.1, 40, 10), []Sample{sample(1.5, 25, 25)},
		[]Sample{sample(1.6, 10, 10), sample(1.7, 10, 10), lost(1.8)}, steady(1.9, 2.1, 0.1, 10, 10), []Sample{sample(2.2, 25, 25)},
	)
	r := run(t, "NOVEL_OBJECT", Input{Zones: objectZones, Samples: trajectory})
	expectValue(t, r, "novel_object_time_s", 0.6)
	expectValue(t, r, "familiar_object_time_s", 0.5)
	expectValue(t, r, "total_exploration_time_s", 1.1)
	expectValue(t, r, "discrimination_index", 0.1/1.1)
	want := []Interval{
		{Type: "exploration", Label: "novel", StartUs: at(0.1), EndUs: at(0.4)},
		{Type: "exploration", Label: "familiar", StartUs: at(1), EndUs: at(1.5)},
		{Type: "exploration", Label: "novel", StartUs: at(1.9), EndUs: at(2.2)},
	}
	if !slices.Equal(r.Intervals, want) || len(r.Points) != 0 {
		t.Errorf("exploration bouts %+v %+v\nwant %+v", r.Intervals, r.Points, want)
	}

	// Without a minimum bout every visit counts: novel 0.3 + 0.1 + 0.3, familiar 0.2 + 0.5.
	every := run(t, "NOVEL_OBJECT", Input{Zones: objectZones, Samples: trajectory, Parameters: map[string]float64{"min_bout_s": 0}})
	expectValue(t, every, "novel_object_time_s", 0.7)
	expectValue(t, every, "familiar_object_time_s", 0.7)
	expectValue(t, every, "discrimination_index", 0)

	strict := run(t, "NOVEL_OBJECT", Input{Zones: objectZones, Samples: trajectory, Parameters: map[string]float64{"min_total_exploration_s": 2}})
	expectValue(t, strict, "total_exploration_time_s", 1.1)
	expectMissing(t, strict, "discrimination_index", ReasonBelowCriterion)

	none := run(t, "NOVEL_OBJECT", Input{Zones: objectZones, Samples: steady(0, 3, 0.1, 25, 25)})
	expectValue(t, none, "total_exploration_time_s", 0)
	expectMissing(t, none, "discrimination_index", ReasonZeroDenominator)

	// Zones are the union of their shapes: a second novel shape around (25, 25) turns the
	// whole of "none" into one bout of 3 s.
	twoShapes := map[string][]Shape{"novel_object": {circle(10, 10, 4), circle(25, 25, 1)}, "familiar_object": objectZones["familiar_object"]}
	expectValue(t, run(t, "NOVEL_OBJECT", Input{Zones: twoShapes, Samples: steady(0, 3, 0.1, 25, 25)}), "novel_object_time_s", 3)

	for _, key := range []string{"novel_object_time_s", "familiar_object_time_s", "total_exploration_time_s", "discrimination_index"} {
		expectMissing(t, run(t, "NOVEL_OBJECT", Input{Zones: objectZones, Samples: []Sample{sample(0, 10, 10), lost(0.1), sample(0.2, 10, 10)}}), key, ReasonNoValidIntervals)
	}
	expectError(t, "NOVEL_OBJECT", Input{Samples: trajectory}, ErrMissingZone)
	expectError(t, "NOVEL_OBJECT", Input{Zones: objectZones, Parameters: map[string]float64{"min_bout_s": 6}}, ErrParameterOutOfRange)
	expectError(t, "NOVEL_OBJECT", Input{Zones: objectZones, Parameters: map[string]float64{"object_zone_radius_cm": 3}}, ErrUnknownParameter)
}
