package domain

import (
	"slices"
	"testing"
)

// Shock grid x in [0, 5]; samples every 0.5 s at x = 20 on the belt unless noted.
// Belt at a constant 10 cm/s, cut-off 60 s, defaults exhaustion_dwell_s 5 and
// min_shock_dwell_s 0.2; the recording lasts 30 s.
//
//	10.0 s on the grid, back on the belt at 10.5 s: a stay of 0.5 s, a shock at 10 s.
//	15.0 s on the grid, back at 15.1 s: 0.1 s, below min_shock_dwell_s, no shock.
//	20.0-26.0 s on the grid, back at 26.5 s: a stay of 6.5 s >= 5 s, exhaustion at 20 s
//	(also a shock at 20 s, which is not before the end of the run).
//
// run time 20 s, distance 10 x 20 = 200 cm, one shock before the end.
func TestTreadmillHandCalculated(t *testing.T) {
	fixed := map[string]float64{"accelerating": 0, "start_speed_cm_s": 10, "max_trial_duration_s": 60}
	trajectory := samples(steady(0, 9.5, 0.5, 20, 3), steady(10, 10, 1, 2, 3), steady(10.5, 14.5, 0.5, 20, 3),
		[]Sample{sample(15, 2, 3), sample(15.1, 20, 3)}, steady(15.5, 19.5, 0.5, 20, 3), steady(20, 26, 0.5, 2, 3), steady(26.5, 30, 0.5, 20, 3))
	r := run(t, "TREADMILL", Input{Parameters: fixed, Zones: gridZones, DurationUs: at(30), Samples: trajectory})
	expectValue(t, r, "latency_to_exhaustion_s", 20)
	expectValue(t, r, "run_time_s", 20)
	expectValue(t, r, "run_distance_cm", 200)
	expectValue(t, r, "shock_count", 1)
	want := []Point{{Type: "shock", AtUs: at(10)}, {Type: "exhaustion", AtUs: at(20)}, {Type: "shock", AtUs: at(20)}}
	if !slices.Equal(r.Points, want) || len(r.Intervals) != 0 {
		t.Errorf("events %+v %+v\nwant %+v", r.Points, r.Intervals, want)
	}

	// Every grid contact is a shock without a minimum dwell; a longer
	// exhaustion criterion leaves the 6.5 s stay a shock only, and the whole
	// recording (30 s of the 60 s cut-off) is the run.
	lenient := set(set(fixed, "min_shock_dwell_s", 0), "exhaustion_dwell_s", 10)
	short := run(t, "TREADMILL", Input{Parameters: lenient, Zones: gridZones, DurationUs: at(30), Samples: trajectory})
	expectMissing(t, short, "latency_to_exhaustion_s", ReasonIncompleteRecording)
	expectValue(t, short, "run_time_s", 30)
	expectValue(t, short, "run_distance_cm", 300)
	expectValue(t, short, "shock_count", 3)
}

func TestTreadmillCensoringAndDwell(t *testing.T) {
	// Accelerating 5 -> 40 cm/s over 1800 s, cut-off 60 s, recording 65 s. The
	// stay from 58 s is cut at 60 s (2 s < 5 s): no exhaustion, the whole cut-off
	// was recorded, so latency is not observed and the run lasts 60 s:
	// 5 x 60 + 35 x 60^2 / (2 x 1800) = 300 + 35 = 335 cm. The shock at 58 s counts.
	ramp := map[string]float64{"accelerating": 1, "start_speed_cm_s": 5, "end_speed_cm_s": 40, "acceleration_duration_s": 1800, "max_trial_duration_s": 60}
	late := run(t, "TREADMILL", Input{Parameters: ramp, Zones: gridZones, DurationUs: at(65),
		Samples: samples(steady(0, 57.5, 0.5, 20, 3), steady(58, 65, 0.5, 2, 3))})
	expectMissing(t, late, "latency_to_exhaustion_s", ReasonNotObserved)
	expectValue(t, late, "run_time_s", 60)
	expectValue(t, late, "run_distance_cm", 335)
	expectValue(t, late, "shock_count", 1)
	if !slices.Equal(late.Points, []Point{{Type: "shock", AtUs: at(58)}}) {
		t.Errorf("events after the cut-off: %+v", late.Points)
	}

	// Exhaustion needs continuous time on the grid: a lost frame at 23.5 s
	// splits 20-27.5 s into stays of 3 s and 3.5 s, two shocks and no exhaustion.
	// Without a known recording length the run ends at the last sample (30 s).
	split := run(t, "TREADMILL", Input{Parameters: ramp, Zones: gridZones,
		Samples: samples(steady(0, 19.5, 0.5, 20, 3), steady(20, 23, 0.5, 2, 3), []Sample{lost(23.5)}, steady(24, 27, 0.5, 2, 3), steady(27.5, 30, 0.5, 20, 3))})
	expectMissing(t, split, "latency_to_exhaustion_s", ReasonIncompleteRecording)
	expectValue(t, split, "run_time_s", 30)
	expectValue(t, split, "shock_count", 2)

	// Exhaustion stays reached within the cut-off: 20-25 s on the grid is exactly 5 s.
	exact := run(t, "TREADMILL", Input{Parameters: ramp, Zones: gridZones, DurationUs: at(26),
		Samples: samples(steady(0, 19.5, 0.5, 20, 3), steady(20, 24.5, 0.5, 2, 3), steady(25, 26, 0.5, 20, 3))})
	expectValue(t, exact, "latency_to_exhaustion_s", 20)
	// 5 x 20 + 35 x 400 / 3600
	expectValue(t, exact, "run_distance_cm", 100+35.0*400/3600)

	for _, s := range [][]Sample{nil, {sample(0, 20, 3), lost(0.5), sample(1, 20, 3)}, {sample(61, 20, 3), sample(61.5, 20, 3)}} {
		r := run(t, "TREADMILL", Input{Parameters: ramp, Zones: gridZones, DurationUs: at(65), Samples: s})
		for _, key := range []string{"latency_to_exhaustion_s", "run_time_s", "run_distance_cm", "shock_count"} {
			expectMissing(t, r, key, ReasonNoValidIntervals)
		}
	}
	expectError(t, "TREADMILL", Input{Samples: samples(steady(0, 1, 0.5, 20, 3))}, ErrMissingZone)
	expectError(t, "TREADMILL", Input{Zones: gridZones, Parameters: map[string]float64{"start_speed_cm_s": 30, "end_speed_cm_s": 20}}, ErrParameterOutOfRange)
}
