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
		"OPEN_FIELD": {map[string]float64{"arena_width_cm": 50, "arena_height_cm": 40, "center_fraction": 0.5}, true, &Bounds{0, 0, 50, 40}},
		"LIGHT_DARK": {map[string]float64{"box_width_cm": 40, "box_height_cm": 20, "light_fraction": 0.5}, true, &Bounds{0, 0, 40, 20}},
		"MWM": {map[string]float64{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30, "wall_annulus_width_cm": 12},
			true, &Bounds{-60, -60, 60, 60}},
		// Calibrated-zone paradigms choose their own coordinate frame, so no bounds apply.
		"EPM":          {map[string]float64{"arm_length_cm": 35, "arm_width_cm": 6}, true, nil},
		"ROTAROD":      {map[string]float64{"start_rpm": 4, "end_rpm": 40}, false, nil},
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
	if _, err := CalibrationRequirement("OPEN_FIELD", 2, nil); !errors.Is(err, ErrUnknownVersion) {
		t.Errorf("unknown version: %v", err)
	}
}
