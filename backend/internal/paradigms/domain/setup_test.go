package domain

import (
	"errors"
	"maps"
	"math"
	"testing"
)

func TestResolveSetupRequiresMeasuredApparatusAndDefaultsSession(t *testing.T) {
	tank := map[string]float64{"tank_diameter_cm": 150, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": -30}
	// Analysis settings live on the environment next to the measurements.
	session, err := ResolveSetup("MWM", 1, set(tank, "min_platform_dwell_s", 2), nil)
	if err != nil || len(session) != 0 {
		t.Fatalf("water maze has no session settings: %v %v", session, err)
	}
	lane := map[string]float64{"lane_length_cm": 40}
	session, err = ResolveSetup("TREADMILL", 1, lane, map[string]float64{"start_speed_cm_s": 10, "end_speed_cm_s": 30})
	if err != nil {
		t.Fatal(err)
	}
	if len(session) != 5 || session["start_speed_cm_s"] != 10 || session["end_speed_cm_s"] != 30 || session["max_trial_duration_s"] != 1800 || session["accelerating"] != 1 {
		t.Fatalf("session with defaults: %v", session)
	}

	missing := maps.Clone(tank)
	delete(missing, "tank_diameter_cm")
	for name, tc := range map[string]struct {
		key                  string
		version              int
		environment, session map[string]float64
		want                 error
	}{
		"unknown paradigm":           {"MAZE", 1, tank, nil, ErrUnknownParadigm},
		"unknown version":            {"MWM", 2, tank, nil, ErrUnknownVersion},
		"missing measurement":        {"MWM", 1, missing, nil, ErrMissingParameter},
		"session key on environment": {"TREADMILL", 1, set(lane, "start_speed_cm_s", 4), nil, ErrUnknownParameter},
		"missing lane length":        {"TREADMILL", 1, nil, nil, ErrMissingParameter},
		"analysis key as session":    {"MWM", 1, tank, map[string]float64{"max_sample_gap_s": 1}, ErrUnknownParameter},
		"apparatus key as session":   {"MWM", 1, tank, map[string]float64{"tank_diameter_cm": 120}, ErrUnknownParameter},
		"geometry above range":       {"MWM", 1, set(tank, "tank_diameter_cm", 300), nil, ErrParameterOutOfRange},
		"geometry not a number":      {"MWM", 1, set(tank, "platform_diameter_cm", math.NaN()), nil, ErrParameterOutOfRange},
		"platform outside tank":      {"MWM", 1, set(tank, "platform_x_cm", 70), nil, ErrParameterOutOfRange},
		"analysis outside range":     {"MWM", 1, set(tank, "min_platform_dwell_s", 11), nil, ErrParameterOutOfRange},
		"session cross rule":         {"TREADMILL", 1, lane, map[string]float64{"start_speed_cm_s": 30, "end_speed_cm_s": 20}, ErrParameterOutOfRange},
		"fractional integer session": {"TREADMILL", 1, lane, map[string]float64{"accelerating": 0.5}, ErrParameterOutOfRange},
	} {
		if _, err := ResolveSetup(tc.key, tc.version, tc.environment, tc.session); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}

func set(m map[string]float64, key string, v float64) map[string]float64 {
	out := maps.Clone(m)
	out[key] = v
	return out
}
