package catalog

import (
	"slices"
	"testing"

	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
)

// The water maze needs a calibration bounded by its pool; its zones derive
// from parameters, so none is drawn. The other paradigms need their zones
// drawn with the calibration: the novel object arena is bounded by its floor,
// the side views of the pole and the treadmill lane choose their own frame.
func TestRequirementsListThePublishedCalibratedZones(t *testing.T) {
	tank, err := New().Requirement("MWM", 1, map[string]float64{"tank_diameter_cm": 150, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30})
	if err != nil || !tank.Required || tank.ToleranceCm != 2 || tank.Bounds == nil || *tank.Bounds != (domain.Bounds{MinX: -75, MinY: -75, MaxX: 75, MaxY: 75}) || len(tank.Zones) != 0 {
		t.Fatalf("MWM: %+v %v", tank, err)
	}
	for key, tc := range map[string]struct {
		parameters map[string]float64
		zones      []domain.ZoneRequirement
		bounds     *domain.Bounds
	}{
		"NOVEL_OBJECT": {map[string]float64{"arena_width_cm": 50, "arena_height_cm": 40},
			[]domain.ZoneRequirement{{Key: "novel_object", Required: true}, {Key: "familiar_object", Required: true}}, &domain.Bounds{MaxX: 50, MaxY: 40}},
		"POLE":      {map[string]float64{"pole_length_cm": 50}, []domain.ZoneRequirement{{Key: "pole", Required: true}, {Key: "base", Required: true}}, nil},
		"TREADMILL": {map[string]float64{"lane_length_cm": 40}, []domain.ZoneRequirement{{Key: "shock_grid", Required: true}}, nil},
	} {
		req, err := New().Requirement(key, 1, tc.parameters)
		if err != nil || !req.Required || req.ToleranceCm != 2 || !slices.Equal(req.Zones, tc.zones) ||
			(req.Bounds == nil) != (tc.bounds == nil) || (req.Bounds != nil && *req.Bounds != *tc.bounds) {
			t.Fatalf("%s: %+v %v", key, req, err)
		}
	}
	if _, err := New().Requirement("MWM", 2, nil); err == nil {
		t.Fatal("unknown version")
	}
}
