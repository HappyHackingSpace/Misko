package catalog

import (
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	"slices"
	"testing"
)

func TestRequirementsListThePublishedCalibratedZones(t *testing.T) {
	epm, err := New().Requirement("EPM", 1, map[string]float64{"arm_length_cm": 35, "arm_width_cm": 6})
	if err != nil || !epm.Required || epm.Bounds != nil || !slices.Equal(epm.Zones, []domain.ZoneRequirement{
		{Key: "center", Required: true}, {Key: "open_arms", Required: true}, {Key: "closed_arms", Required: true}}) {
		t.Fatalf("EPM: %+v %v", epm, err)
	}
	chamber, err := New().Requirement("THREE_CHAMBER", 1, nil)
	if err != nil || !slices.Contains(chamber.Zones, domain.ZoneRequirement{Key: "social_interaction", Required: false}) {
		t.Fatalf("THREE_CHAMBER: %+v %v", chamber, err)
	}
	box, err := New().Requirement("OPEN_FIELD", 1, map[string]float64{"arena_width_cm": 50, "arena_height_cm": 40})
	if err != nil || !box.Required || box.Bounds == nil || len(box.Zones) != 0 {
		t.Fatalf("OPEN_FIELD: %+v %v", box, err)
	}
	if rod, err := New().Requirement("ROTAROD", 1, nil); err != nil || rod.Required || len(rod.Zones) != 0 {
		t.Fatalf("ROTAROD: %+v %v", rod, err)
	}
}
