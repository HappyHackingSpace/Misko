package domain

import (
	"errors"
	"strings"
	"testing"
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
