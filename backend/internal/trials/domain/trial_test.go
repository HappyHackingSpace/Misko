package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestTrialsAreNamedAndClipped(t *testing.T) {
	trial, err := NewTrial("  Camera 2  ", "MWM", 1, "rev", " first try ", "user")
	if err != nil || trial.Name != "Camera 2" || trial.Notes != "first try" || trial.Active() != nil {
		t.Fatalf("trial: %+v %v", trial, err)
	}
	for name, tc := range map[string]struct {
		name, notes string
		want        error
	}{
		"empty name":      {" ", "", ErrInvalidName},
		"long name":       {strings.Repeat("n", 121), "", ErrInvalidName},
		"control in name": {"a\tb", "", ErrInvalidName},
		"long notes":      {"x", strings.Repeat("n", 2001), ErrInvalidNotes},
	} {
		if _, err := NewTrial(tc.name, "MWM", 1, "rev", tc.notes, "user"); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	end, start := int64(10), int64(10)
	for name, tc := range map[string]struct {
		clip Clip
		want error
	}{
		"whole video":  {Clip{}, nil},
		"negative":     {Clip{StartUs: -1}, ErrInvalidClip},
		"empty":        {Clip{StartUs: start, EndUs: &end}, ErrInvalidClip},
		"start to end": {Clip{StartUs: 0, EndUs: &end}, nil},
	} {
		if err := tc.clip.Validate(); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	trial.ID, trial.VideoAssetID = "t", "v"
	if trial.AutomaticKey() != "trial:t:video:v" {
		t.Fatalf("key: %s", trial.AutomaticKey())
	}
}

func TestOverridesApplyOnTheArenasParameters(t *testing.T) {
	for name, tc := range map[string]struct {
		overrides map[string]float64
		want      error
	}{
		"none":     {nil, nil},
		"finite":   {map[string]float64{"min_bout_s": 1}, nil},
		"NaN":      {map[string]float64{"min_bout_s": math.NaN()}, ErrInvalidOverrides},
		"infinite": {map[string]float64{"min_bout_s": math.Inf(1)}, ErrInvalidOverrides},
		"no key":   {map[string]float64{"": 1}, ErrInvalidOverrides},
	} {
		if err := CheckOverrides(tc.overrides); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	arena := map[string]float64{"arena_width_cm": 40, "min_bout_s": 0.3}
	merged := Merge(arena, map[string]float64{"min_bout_s": 1})
	if merged["min_bout_s"] != 1 || merged["arena_width_cm"] != 40 || arena["min_bout_s"] != 0.3 {
		t.Fatalf("merge: %v %v", merged, arena)
	}
}
