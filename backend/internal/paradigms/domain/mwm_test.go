package domain

import (
	"math"
	"testing"
)

// Tank radius 60 cm centered at the origin (x east, y north), platform radius 5 cm
// at (30, 30) in the NE quadrant, wall annulus from r = 48 cm.
//
//	samples 0, 0.5, 1.0, 1.5, 2.0, 2.5 s; all five intervals count (2.5 s).
//	platform (distance <= 5): samples at 1.5 (4 cm), 2.0 (0 cm), 2.5 (1 cm); the dwell from 1.5 to 2.5 lasts 1 s, so escape = 1.5 s.
//	path until escape: 10 + 40 + hypot(30, 26); swim speed divides by the 1.5 s before escape.
//	target quadrant (x >= 0, y >= 0) holds the earlier samples at 1.0, 1.5 and 2.0 s: 1.5 / 2.5 = 0.6.
//	thigmotaxis: only the first sample has r >= 48: 0.5 / 2.5 = 0.2.
func TestMorrisWaterMazeHandCalculated(t *testing.T) {
	params := map[string]float64{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30, "wall_annulus_width_cm": 12, "min_platform_dwell_s": 1}
	r := run(t, "MWM", Input{Parameters: params, Samples: []Sample{
		sample(0, -50, 0), sample(0.5, -40, 0), sample(1, 0, 0), sample(1.5, 30, 26), sample(2, 30, 30), sample(2.5, 31, 30),
	}})
	path := 10 + 40 + math.Hypot(30, 26)
	expectValue(t, r, "duration_s", 2.5)
	expectValue(t, r, "escape_latency_s", 1.5)
	expectValue(t, r, "path_length_cm", path)
	expectValue(t, r, "mean_swim_speed_cm_s", path/1.5)
	expectValue(t, r, "target_quadrant_time_ratio", 0.6)
	expectValue(t, r, "thigmotaxis_time_ratio", 0.2)
	expectValue(t, r, "platform_crossings_count", 1)
	expectValue(t, r, "mean_distance_to_platform_cm", (math.Hypot(80, 30)+math.Hypot(70, 30)+math.Hypot(30, 30)+4+0)*0.5/2.5)
}

// A dwell shorter than the minimum is not an escape, a start on the platform is
// not a crossing, and positions outside the tank are lost.
func TestMorrisWaterMazeWithoutEscape(t *testing.T) {
	params := map[string]float64{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": -30, "platform_y_cm": -30, "min_platform_dwell_s": 1}
	r := run(t, "MWM", Input{Parameters: params, Samples: []Sample{
		sample(0, -30, -30), sample(0.5, -30, -29), sample(1, 0, 0), sample(1.5, 61, 0), sample(2, 0, 0),
	}})
	path := 1 + math.Hypot(30, 29)
	expectValue(t, r, "duration_s", 1)
	expectMissing(t, r, "escape_latency_s", ReasonNotObserved)
	expectValue(t, r, "path_length_cm", path)
	expectValue(t, r, "mean_swim_speed_cm_s", path)
	expectValue(t, r, "platform_crossings_count", 0)
	expectValue(t, r, "target_quadrant_time_ratio", 1)

	for name, bad := range map[string]map[string]float64{
		"platform outside tank":       {"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 58, "platform_y_cm": 0},
		"annulus as wide as radius":   {"tank_diameter_cm": 60, "platform_diameter_cm": 4, "platform_x_cm": 5, "platform_y_cm": 5, "wall_annulus_width_cm": 30},
		"platform diameter too small": {"platform_diameter_cm": 3},
	} {
		t.Run(name, func(t *testing.T) { expectError(t, "MWM", Input{Parameters: bad}, ErrParameterOutOfRange) })
	}
}
