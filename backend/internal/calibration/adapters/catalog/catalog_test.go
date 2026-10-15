package catalog

import (
	"testing"

	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
)

// The water maze needs a calibration bounded by its pool; its zones derive
// from parameters, so none is drawn. Observer-scored paradigms are never
// tracked and need no calibration.
func TestRequirementsListThePublishedCalibratedZones(t *testing.T) {
	tank, err := New().Requirement("MWM", 1, map[string]float64{"tank_diameter_cm": 150, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30})
	if err != nil || !tank.Required || tank.ToleranceCm != 2 || tank.Bounds == nil || *tank.Bounds != (domain.Bounds{MinX: -75, MinY: -75, MaxX: 75, MaxY: 75}) || len(tank.Zones) != 0 {
		t.Fatalf("MWM: %+v %v", tank, err)
	}
	for _, key := range []string{"NOVEL_OBJECT", "POLE", "TREADMILL"} {
		if scored, err := New().Requirement(key, 1, nil); err != nil || scored.Required || len(scored.Zones) != 0 || scored.Bounds != nil {
			t.Fatalf("%s: %+v %v", key, scored, err)
		}
	}
	if _, err := New().Requirement("MWM", 2, nil); err == nil {
		t.Fatal("unknown version")
	}
}
