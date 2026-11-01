package catalog

import (
	"errors"
	"testing"

	"github.com/HappyHackingSpace/Misko/backend/internal/trials/domain"
)

func TestOverridesAreAnalysisParametersWithinRange(t *testing.T) {
	arena := map[string]float64{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30}
	for name, tc := range map[string]struct {
		overrides map[string]float64
		want      error
	}{
		"none":                {nil, nil},
		"analysis threshold":  {map[string]float64{"max_sample_gap_s": 1}, nil},
		"apparatus value":     {map[string]float64{"tank_diameter_cm": 150}, domain.ErrInvalidOverrides},
		"unknown key":         {map[string]float64{"speed": 1}, domain.ErrInvalidOverrides},
		"threshold too large": {map[string]float64{"max_sample_gap_s": 1e9}, domain.ErrInvalidOverrides},
	} {
		if err := New().CheckOverrides("MWM", 1, arena, tc.overrides); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v want %v", name, err, tc.want)
		}
	}
}
