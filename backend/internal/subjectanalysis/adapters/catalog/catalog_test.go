package catalog

import (
	"slices"
	"testing"
)

func TestRequirementsComeFromThePublishedParadigms(t *testing.T) {
	for _, tc := range []struct {
		key   string
		zones []string
	}{
		{"MWM", nil},
		{"NOVEL_OBJECT", []string{"novel_object", "familiar_object"}},
		{"POLE", []string{"pole", "base"}},
		{"TREADMILL", []string{"shock_grid"}},
	} {
		req, err := New().Requirement(tc.key, 1)
		if err != nil || !req.CalibrationRequired {
			t.Fatalf("%s: %+v %v", tc.key, req, err)
		}
		if !slices.Equal(slices.Sorted(slices.Values(req.RequiredZones)), slices.Sorted(slices.Values(tc.zones))) {
			t.Errorf("%s zones: %v want %v", tc.key, req.RequiredZones, tc.zones)
		}
		if !slices.Contains(New().Calibrated(), tc.key+"@1") {
			t.Errorf("%s v1 not listed as calibrated", tc.key)
		}
	}
	if _, err := New().Requirement("MAZE", 1); err == nil {
		t.Fatal("unknown paradigm accepted")
	}
}
