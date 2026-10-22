package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestEnvironmentFieldsAreValidated(t *testing.T) {
	e, err := NewEnvironment("  Arena A  ", "MWM", "  north room  ")
	if err != nil || e.Name != "Arena A" || e.ParadigmKey != "MWM" || e.Notes != "north room" {
		t.Fatalf("normalized: %+v %v", e, err)
	}
	if _, err := NewEnvironment(strings.Repeat("a", 120), "POLE", strings.Repeat("n", 2000)); err != nil {
		t.Fatalf("limits: %v", err)
	}
	for name, tc := range map[string]struct {
		name, key, notes string
		want             error
	}{
		"empty name":         {"   ", "POLE", "", ErrInvalidName},
		"long name":          {strings.Repeat("a", 121), "POLE", "", ErrInvalidName},
		"control character":  {"Arena\x00", "POLE", "", ErrInvalidName},
		"invalid UTF-8":      {"Arena\xff", "POLE", "", ErrInvalidName},
		"lowercase paradigm": {"Arena", "water_maze", "", ErrInvalidParadigmKey},
		"empty paradigm":     {"Arena", "", "", ErrInvalidParadigmKey},
		"long notes":         {"Arena", "POLE", strings.Repeat("n", 2001), ErrInvalidNotes},
	} {
		if _, err := NewEnvironment(tc.name, tc.key, tc.notes); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}

func TestRevisionCopiesItsMeasurements(t *testing.T) {
	parameters := map[string]float64{"tank_diameter_cm": 50, "platform_diameter_cm": 10}
	r, err := NewRevision("env", "MWM", 1, parameters, " re-measured ", "user")
	if err != nil || r.Notes != "re-measured" || r.ParadigmVersion != 1 || r.CreatedBy != "user" {
		t.Fatalf("revision: %+v %v", r, err)
	}
	parameters["tank_diameter_cm"] = 99
	if r.Parameters["tank_diameter_cm"] != 50 {
		t.Fatal("a revision must not share its measurements with the caller")
	}
	if _, err := NewRevision("env", "MWM", 0, parameters, "", "user"); !errors.Is(err, ErrInvalidParadigmVersion) {
		t.Fatalf("version 0: %v", err)
	}
	if _, err := NewRevision("env", "MWM", 1, parameters, strings.Repeat("n", 2001), "user"); !errors.Is(err, ErrInvalidNotes) {
		t.Fatalf("notes: %v", err)
	}
}
