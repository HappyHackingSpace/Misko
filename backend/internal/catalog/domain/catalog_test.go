package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewSubstanceNormalizesInput(t *testing.T) {
	s, err := NewSubstance("  Saline 0.9%  ", "VEHICLE", " sterile\nisotonic ")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Saline 0.9%" || s.Kind != Vehicle || s.Description != "sterile\nisotonic" {
		t.Fatalf("got %+v", s)
	}
	for _, kind := range []string{"COMPOUND", "REFERENCE_DRUG", "VEHICLE", "INDUCER"} {
		if _, err := NewSubstance("X", kind, ""); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}
}

func TestNewSubstanceRejectsInvalidFields(t *testing.T) {
	for name, tc := range map[string]struct {
		name, kind, description string
		want                    error
	}{
		"empty name":         {" ", "COMPOUND", "", ErrInvalidName},
		"long name":          {strings.Repeat("a", 121), "COMPOUND", "", ErrInvalidName},
		"line break in name": {"a\nb", "COMPOUND", "", ErrInvalidName},
		"lowercase kind":     {"MPTP", "inducer", "", ErrInvalidSubstanceKind},
		"missing kind":       {"MPTP", "", "", ErrInvalidSubstanceKind},
		"long description":   {"MPTP", "INDUCER", strings.Repeat("d", 5001), ErrInvalidDescription},
		"control character":  {"MPTP", "INDUCER", "a\x00b", ErrInvalidDescription},
	} {
		if _, err := NewSubstance(tc.name, tc.kind, tc.description); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}

func TestNewDiseaseModelValidatesEveryField(t *testing.T) {
	m, err := NewDiseaseModel(" MPTP parkinsonism ", "CHEMICAL", " pole descent time > 150% of baseline ", "")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "MPTP parkinsonism" || m.InductionType != Chemical || m.ConfirmationCriterion != "pole descent time > 150% of baseline" || m.Description != "" {
		t.Fatalf("got %+v", m)
	}
	for _, typ := range []string{"CHEMICAL", "SURGICAL", "DIET", "STRESS", "OTHER"} {
		if _, err := NewDiseaseModel("X", typ, "", ""); err != nil {
			t.Errorf("%s: %v", typ, err)
		}
	}
	for name, tc := range map[string]struct {
		name, typ, criterion, description string
		want                              error
	}{
		"empty name":        {"", "CHEMICAL", "", "", ErrInvalidName},
		"genetic model":     {"Tg2576", "GENETIC", "", "", ErrInvalidInductionType},
		"long criterion":    {"MCAO", "SURGICAL", strings.Repeat("c", 2001), "", ErrInvalidConfirmationCriterion},
		"long description":  {"MCAO", "SURGICAL", "", strings.Repeat("d", 5001), ErrInvalidDescription},
		"invalid utf8 name": {"\xff", "SURGICAL", "", "", ErrInvalidName},
	} {
		if _, err := NewDiseaseModel(tc.name, tc.typ, tc.criterion, tc.description); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}
