package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestEnvironmentFieldsAreValidated(t *testing.T) {
	e, err := NewEnvironment("  Arena A  ", "OPEN_FIELD", "  north room  ")
	if err != nil || e.Name != "Arena A" || e.ParadigmKey != "OPEN_FIELD" || e.Notes != "north room" {
		t.Fatalf("normalized: %+v %v", e, err)
	}
	if _, err := NewEnvironment(strings.Repeat("a", 120), "EPM", strings.Repeat("n", 2000)); err != nil {
		t.Fatalf("limits: %v", err)
	}
	for name, tc := range map[string]struct {
		name, key, notes string
		want             error
	}{
		"empty name":         {"   ", "EPM", "", ErrInvalidName},
		"long name":          {strings.Repeat("a", 121), "EPM", "", ErrInvalidName},
		"control character":  {"Arena\x00", "EPM", "", ErrInvalidName},
		"invalid UTF-8":      {"Arena\xff", "EPM", "", ErrInvalidName},
		"lowercase paradigm": {"Arena", "open_field", "", ErrInvalidParadigmKey},
		"empty paradigm":     {"Arena", "", "", ErrInvalidParadigmKey},
		"long notes":         {"Arena", "EPM", strings.Repeat("n", 2001), ErrInvalidNotes},
	} {
		if _, err := NewEnvironment(tc.name, tc.key, tc.notes); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}

func TestRevisionCopiesItsMeasurements(t *testing.T) {
	apparatus := map[string]float64{"arena_width_cm": 50, "arena_height_cm": 40}
	r, err := NewRevision("env", "OPEN_FIELD", 1, apparatus, " re-measured ", "user")
	if err != nil || r.Notes != "re-measured" || r.ParadigmVersion != 1 || r.CreatedBy != "user" {
		t.Fatalf("revision: %+v %v", r, err)
	}
	apparatus["arena_width_cm"] = 99
	if r.Apparatus["arena_width_cm"] != 50 {
		t.Fatal("a revision must not share its measurements with the caller")
	}
	if _, err := NewRevision("env", "OPEN_FIELD", 0, apparatus, "", "user"); !errors.Is(err, ErrInvalidParadigmVersion) {
		t.Fatalf("version 0: %v", err)
	}
	if _, err := NewRevision("env", "OPEN_FIELD", 1, apparatus, strings.Repeat("n", 2001), "user"); !errors.Is(err, ErrInvalidNotes) {
		t.Fatalf("notes: %v", err)
	}
}
