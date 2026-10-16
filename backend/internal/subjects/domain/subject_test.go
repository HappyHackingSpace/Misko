package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewSubjectNormalizesInput(t *testing.T) {
	s, err := NewSubject(" F-001 ", "MOUSE", "FEMALE")
	if err != nil {
		t.Fatal(err)
	}
	if s.Code != "F-001" || s.Species != Mouse || s.Sex != Female {
		t.Fatalf("got %+v", s)
	}
	if _, err := NewSubject("R.7", "RAT", "UNKNOWN"); err != nil {
		t.Fatalf("rat with unknown sex: %v", err)
	}
}

func TestNewSubjectRejectsInvalidFields(t *testing.T) {
	valid := [3]string{"M1", "MOUSE", "MALE"}
	for _, tc := range []struct {
		name  string
		field int
		value string
		want  error
	}{
		{"empty code", 0, " ", ErrInvalidCode},
		{"code with space", 0, "M 1", ErrInvalidCode},
		{"long code", 0, strings.Repeat("A", 65), ErrInvalidCode},
		{"non-ascii code", 0, "Ş1", ErrInvalidCode},
		{"lowercase species", 1, "mouse", ErrInvalidSpecies},
		{"missing species", 1, "", ErrInvalidSpecies},
		{"abbreviated sex", 2, "F", ErrInvalidSex},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := valid
			f[tc.field] = tc.value
			if _, err := NewSubject(f[0], f[1], f[2]); !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestGramsRoundTripExactly(t *testing.T) {
	for raw, mg := range map[string]int64{"0.001": 1, "25": 25000, "25.5": 25500, " 31.125 ": 31125, "5000": 5_000_000, "0250.050": 250050} {
		got, err := ParseGrams(raw)
		if err != nil || got != mg {
			t.Errorf("ParseGrams(%q) = %d, %v want %d", raw, got, err, mg)
		}
	}
	for mg, text := range map[int64]string{1: "0.001", 25000: "25", 25500: "25.5", 31125: "31.125", 250050: "250.05"} {
		if got := FormatGrams(mg); got != text {
			t.Errorf("FormatGrams(%d) = %q want %q", mg, got, text)
		}
	}
	for _, raw := range []string{"", "0", "0.000", "-1", "5000.001", "10000", "1.2345", "1e3", "1,5", ".5", "5."} {
		if _, err := ParseGrams(raw); !errors.Is(err, ErrInvalidWeight) {
			t.Errorf("ParseGrams(%q): %v", raw, err)
		}
	}
}

func TestNewWeightRejectsMissingAndFutureTimes(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	if _, err := NewWeight("s", "25", time.Time{}, now, "u"); !errors.Is(err, ErrMissingTime) {
		t.Fatalf("missing time: %v", err)
	}
	if _, err := NewWeight("s", "25", now.Add(ClockSkew+time.Second), now, "u"); !errors.Is(err, ErrFutureTime) {
		t.Fatalf("future time: %v", err)
	}
	w, err := NewWeight("s", "25", now.Add(ClockSkew), now, "u")
	if err != nil || w.Milligrams != 25000 || w.SubjectID != "s" || w.RecordedBy != "u" {
		t.Fatalf("within clock skew: %+v %v", w, err)
	}
}
