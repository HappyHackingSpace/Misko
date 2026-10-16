package domain

import (
	"errors"
	"testing"
)

func TestCalibrationRequirements(t *testing.T) {
	for key, tc := range map[string]struct {
		apparatus map[string]float64
		required  bool
		bounds    *Bounds
	}{
		"MWM": {map[string]float64{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30, "wall_annulus_width_cm": 12},
			true, &Bounds{-60, -60, 60, 60}},
		// Observer-scored paradigms are never tracked, so they need no calibration.
		"POLE":         {map[string]float64{"pole_length_cm": 50}, false, nil},
		"TREADMILL":    {map[string]float64{"lane_length_cm": 40}, false, nil},
		"NOVEL_OBJECT": {map[string]float64{"arena_width_cm": 50, "arena_height_cm": 50, "object_zone_radius_cm": 3}, false, nil},
	} {
		req, err := CalibrationRequirement(key, 1, tc.apparatus)
		if err != nil || req.Required != tc.required {
			t.Errorf("%s: %+v %v", key, req, err)
			continue
		}
		if tc.required && req.ToleranceCm != 2 {
			t.Errorf("%s: tolerance %v, want the max_calibration_error_cm QC rule", key, req.ToleranceCm)
		}
		if (req.Bounds == nil) != (tc.bounds == nil) || (req.Bounds != nil && *req.Bounds != *tc.bounds) {
			t.Errorf("%s: bounds %+v want %+v", key, req.Bounds, tc.bounds)
		}
	}
	if _, err := CalibrationRequirement("MAZE", 1, nil); !errors.Is(err, ErrUnknownParadigm) {
		t.Errorf("unknown paradigm: %v", err)
	}
	if _, err := CalibrationRequirement("MWM", 2, nil); !errors.Is(err, ErrUnknownVersion) {
		t.Errorf("unknown version: %v", err)
	}
}
