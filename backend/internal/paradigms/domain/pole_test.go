package domain

import (
	"slices"
	"testing"
)

// Pole band x in [-2, 2], y in [0, 50]; base y <= 1. Defaults: turn_descent_cm 2,
// fall_speed_cm_s 80, max_sample_gap_s 0.5, max_trial_duration_s 60, pole 50 cm.
//
//	0.0 s y = 45, 0.5 s y = 47 (highest), 1.0 s y = 46 (1 cm below), 1.5 s y = 45 (2 cm below): turn at 1.5 s.
//	2.0 s y = 35, 2.5 s y = 20, 3.0 s y = 8, 3.5 s y = 0.5 in the base at 15 cm/s: base reached at 3.5 s.
//	descent 3.5 - 1.5 = 2 s at 50 / 2 = 25 cm/s, no fall.
func TestPoleHandCalculated(t *testing.T) {
	r := run(t, "POLE", minimalInputs()["POLE"])
	expectValue(t, r, "t_turn_s", 1.5)
	expectValue(t, r, "t_total_s", 3.5)
	expectValue(t, r, "descent_time_s", 2)
	expectValue(t, r, "descent_speed_cm_s", 25)
	expectValue(t, r, "fall_detected", 0)
	if want := []Point{{Type: "turn_complete", AtUs: at(1.5)}, {Type: "base_reached", AtUs: at(3.5)}}; !slices.Equal(r.Points, want) || len(r.Intervals) != 0 {
		t.Errorf("events %+v %+v", r.Points, r.Intervals)
	}
}

func TestPoleFalls(t *testing.T) {
	// Turn at 1.0 s (3 cm below 47); 0.1 s later the subject is off the pole at
	// x = 8, 14 cm lower: 140 cm/s > 80, a fall at 1.1 s. Landing in the base
	// afterwards is not reaching the base.
	offPole := run(t, "POLE", Input{Zones: poleZones, Samples: []Sample{
		sample(0, 0, 45), sample(0.5, 0, 47), sample(1, 0, 44), sample(1.1, 8, 30), sample(1.5, 8, 0),
	}})
	expectValue(t, offPole, "t_turn_s", 1)
	expectMissing(t, offPole, "t_total_s", ReasonNotObserved)
	expectMissing(t, offPole, "descent_time_s", ReasonNotObserved)
	expectMissing(t, offPole, "descent_speed_cm_s", ReasonNotObserved)
	expectValue(t, offPole, "fall_detected", 1)
	if want := []Point{{Type: "turn_complete", AtUs: at(1)}, {Type: "fall", AtUs: at(1.1)}}; !slices.Equal(offPole.Points, want) {
		t.Errorf("events %+v", offPole.Points)
	}

	// From 44 cm on the pole straight into the base 0.1 s later: 440 cm/s, a fall
	// at 0.2 s, before any turn.
	intoBase := run(t, "POLE", Input{Zones: poleZones, Samples: []Sample{sample(0, 0, 45), sample(0.1, 0, 44), sample(0.2, 0, 0)}})
	expectValue(t, intoBase, "fall_detected", 1)
	expectMissing(t, intoBase, "t_turn_s", ReasonNotObserved)
	expectMissing(t, intoBase, "t_total_s", ReasonNotObserved)

	// Walking off the pole at 2 cm/s is no fall, and the base reached later
	// still counts without a turn.
	slow := run(t, "POLE", Input{Zones: poleZones, Samples: []Sample{sample(0, 0, 45), sample(0.5, 5, 44), sample(1, 5, 0.5)}})
	expectValue(t, slow, "fall_detected", 0)
	expectValue(t, slow, "t_total_s", 1)
	expectMissing(t, slow, "t_turn_s", ReasonNotObserved)
	expectMissing(t, slow, "descent_speed_cm_s", ReasonNotObserved)

	// A drop across more than max_sample_gap_s (0.3 s > 0.2 s, behind a lost
	// frame) is not judged as a fall: the subject simply reaches the base.
	gap := run(t, "POLE", Input{Zones: poleZones, Parameters: map[string]float64{"max_sample_gap_s": 0.2},
		Samples: []Sample{sample(0, 0, 45), sample(0.1, 0, 47), lost(0.2), sample(0.4, 0, 0.5)}})
	expectValue(t, gap, "fall_detected", 0)
	expectValue(t, gap, "t_total_s", 0.4)

	// A higher fall speed threshold turns the 140 cm/s drop into a descent off the pole.
	tolerant := run(t, "POLE", Input{Zones: poleZones, Parameters: map[string]float64{"fall_speed_cm_s": 200}, Samples: []Sample{
		sample(0, 0, 45), sample(0.5, 0, 47), sample(1, 0, 44), sample(1.1, 8, 30), sample(1.5, 8, 0),
	}})
	expectValue(t, tolerant, "fall_detected", 0)
	expectValue(t, tolerant, "t_total_s", 1.5)
	expectValue(t, tolerant, "descent_time_s", 0.5)
}

func TestPoleTurnBaseAndCutOff(t *testing.T) {
	// A start in the base is not reaching it: the base counts only after a
	// sample on the pole. A larger turn_descent_cm delays the turn: 40 -> 37
	// is 3 cm, below 4; 40 -> 35 at 1.5 s is the turn.
	r := run(t, "POLE", Input{Zones: poleZones, Parameters: map[string]float64{"turn_descent_cm": 4}, Samples: []Sample{
		sample(0, 0, 0.5), sample(0.5, 0, 40), sample(1, 0, 37), sample(1.5, 0, 35), sample(2, 0, 10), sample(2.5, 0, 1),
	}})
	expectValue(t, r, "t_turn_s", 1.5)
	expectValue(t, r, "t_total_s", 2.5)
	expectValue(t, r, "descent_speed_cm_s", 50)

	// Cut-off at 30 s: the turn at 29.5 s and the sample at exactly 30 s
	// belong to the trial, the base at 30.5 s does not.
	cut := run(t, "POLE", Input{Zones: poleZones, Parameters: map[string]float64{"max_trial_duration_s": 30}, Samples: []Sample{
		sample(0, 0, 45), sample(0.5, 0, 47), sample(29.5, 0, 44), sample(30, 0, 40), sample(30.5, 0, 0.5),
	}})
	expectValue(t, cut, "t_turn_s", 29.5)
	expectMissing(t, cut, "t_total_s", ReasonNotObserved)
	expectMissing(t, cut, "descent_time_s", ReasonNotObserved)
	expectValue(t, cut, "fall_detected", 0)

	// Without counted intervals nothing is known, not even the absence of a fall.
	for _, s := range [][]Sample{nil, {sample(0, 0, 45)}, {sample(0, 0, 45), sample(1, 0, 0.5)}, {sample(70, 0, 45), sample(70.5, 0, 0.5)}} {
		r := run(t, "POLE", Input{Zones: poleZones, Samples: s})
		for _, key := range []string{"t_turn_s", "t_total_s", "descent_time_s", "descent_speed_cm_s", "fall_detected"} {
			expectMissing(t, r, key, ReasonNoValidIntervals)
		}
		if len(r.Points) != 0 {
			t.Errorf("events without counted intervals: %+v", r.Points)
		}
	}
	expectError(t, "POLE", Input{Zones: map[string][]Shape{"pole": poleZones["pole"]}}, ErrMissingZone)
}
