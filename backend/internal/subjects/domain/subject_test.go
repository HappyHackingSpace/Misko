package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)

func TestNewSubjectNormalizesInput(t *testing.T) {
	s, err := NewSubject(" F-001 ", "MOUSE", "FEMALE", " C57BL/6J ", "2026-05-01", "  left ear tag\nsecond line ", now)
	if err != nil {
		t.Fatal(err)
	}
	if s.Code != "F-001" || s.Species != Mouse || s.Sex != Female || s.Strain != "C57BL/6J" || s.Notes != "left ear tag\nsecond line" {
		t.Fatalf("got %+v", s)
	}
	if s.BirthDate == nil || !s.BirthDate.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("birth date %v", s.BirthDate)
	}
	minimal, err := NewSubject("R.7", "RAT", "UNKNOWN", "", "", "", now)
	if err != nil || minimal.BirthDate != nil || minimal.Strain != "" || minimal.Notes != "" {
		t.Fatalf("optional fields: %+v %v", minimal, err)
	}
	// A birth date one calendar day ahead of UTC is accepted for laboratories east of UTC.
	if _, err := NewSubject("M1", "MOUSE", "MALE", "", "2026-09-12", "", now); err != nil {
		t.Fatalf("tomorrow in UTC rejected: %v", err)
	}
}

func TestNewSubjectRejectsInvalidFields(t *testing.T) {
	valid := [6]string{"M1", "MOUSE", "MALE", "", "", ""}
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
		{"long strain", 3, strings.Repeat("s", 121), ErrInvalidStrain},
		{"control in strain", 3, "C57\nBL", ErrInvalidStrain},
		{"impossible date", 4, "2026-13-01", ErrInvalidBirthDate},
		{"wrong date format", 4, "01/05/2026", ErrInvalidBirthDate},
		{"future date", 4, "2026-09-13", ErrInvalidBirthDate},
		{"long notes", 5, strings.Repeat("n", 2001), ErrInvalidNotes},
		{"null byte in notes", 5, "tag\x00", ErrInvalidNotes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := valid
			f[tc.field] = tc.value
			if _, err := NewSubject(f[0], f[1], f[2], f[3], f[4], f[5], now); !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}
