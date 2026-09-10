package domain

import (
	"errors"
	"maps"
	"math"
	"testing"
)

func TestResolveSetupRequiresMeasuredApparatusAndDefaultsSession(t *testing.T) {
	tank := map[string]float64{"tank_diameter_cm": 150, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": -30, "wall_annulus_width_cm": 15}
	session, err := ResolveSetup("MWM", 1, tank, map[string]float64{"min_platform_dwell_s": 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(session) != 2 || session["min_platform_dwell_s"] != 2 || session["max_sample_gap_s"] != 0.5 {
		t.Fatalf("session with defaults: %v", session)
	}
	if _, err := ResolveSetup("ROTAROD", 1, map[string]float64{"start_rpm": 4, "end_rpm": 40}, nil); err != nil {
		t.Fatalf("apparatus only: %v", err)
	}

	set := func(m map[string]float64, key string, v float64) map[string]float64 {
		out := maps.Clone(m)
		out[key] = v
		return out
	}
	missing := maps.Clone(tank)
	delete(missing, "tank_diameter_cm")
	for name, tc := range map[string]struct {
		key                string
		version            int
		apparatus, session map[string]float64
		want               error
	}{
		"unknown paradigm":           {"MAZE", 1, tank, nil, ErrUnknownParadigm},
		"unknown version":            {"MWM", 2, tank, nil, ErrUnknownVersion},
		"missing measurement":        {"MWM", 1, missing, nil, ErrMissingParameter},
		"session key as apparatus":   {"MWM", 1, set(tank, "max_sample_gap_s", 1), nil, ErrUnknownParameter},
		"apparatus key as session":   {"MWM", 1, tank, map[string]float64{"tank_diameter_cm": 120}, ErrUnknownParameter},
		"geometry above range":       {"MWM", 1, set(tank, "tank_diameter_cm", 300), nil, ErrParameterOutOfRange},
		"geometry not a number":      {"MWM", 1, set(tank, "platform_diameter_cm", math.NaN()), nil, ErrParameterOutOfRange},
		"platform outside tank":      {"MWM", 1, set(tank, "platform_x_cm", 70), nil, ErrParameterOutOfRange},
		"session outside range":      {"MWM", 1, tank, map[string]float64{"min_platform_dwell_s": 11}, ErrParameterOutOfRange},
		"session cross rule":         {"TREADMILL", 1, map[string]float64{"lane_length_cm": 40}, map[string]float64{"start_speed_cm_s": 30, "end_speed_cm_s": 20}, ErrParameterOutOfRange},
		"apparatus cross rule":       {"ROTAROD", 1, map[string]float64{"start_rpm": 40, "end_rpm": 4}, nil, ErrParameterOutOfRange},
		"fractional integer session": {"Y_MAZE", 1, map[string]float64{"arm_length_cm": 35}, map[string]float64{"novel_arm": 1.5}, ErrParameterOutOfRange},
	} {
		if _, err := ResolveSetup(tc.key, tc.version, tc.apparatus, tc.session); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}
