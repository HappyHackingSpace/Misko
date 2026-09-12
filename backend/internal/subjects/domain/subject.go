// Package domain holds laboratory animals. A subject's identity is independent
// of the experiments it is enrolled in.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidCode      = errors.New("code must contain 1 to 64 ASCII letters, digits, '.', '_', '/' or '-'")
	ErrInvalidSpecies   = errors.New("species must be MOUSE or RAT")
	ErrInvalidSex       = errors.New("sex must be FEMALE, MALE or UNKNOWN")
	ErrInvalidStrain    = errors.New("strain must contain at most 120 printable characters")
	ErrInvalidBirthDate = errors.New("birth date must be a YYYY-MM-DD date that is not in the future")
	ErrInvalidNotes     = errors.New("notes must contain at most 2000 characters")
)

type Species string

const (
	Mouse Species = "MOUSE"
	Rat   Species = "RAT"
)

type Sex string

const (
	Female     Sex = "FEMALE"
	Male       Sex = "MALE"
	UnknownSex Sex = "UNKNOWN"
)

type Subject struct {
	ID      string
	Code    string
	Species Species
	Sex     Sex
	Strain  string
	// BirthDate is a calendar date at UTC midnight; nil when unknown.
	BirthDate *time.Time
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewSubject(code, species, sex, strain, birthDate, notes string, now time.Time) (Subject, error) {
	var s Subject
	var err error
	if s.Code, err = NormalizeCode(code); err != nil {
		return Subject{}, err
	}
	if s.Species, err = ParseSpecies(species); err != nil {
		return Subject{}, err
	}
	if s.Sex, err = ParseSex(sex); err != nil {
		return Subject{}, err
	}
	if s.Strain, err = NormalizeStrain(strain); err != nil {
		return Subject{}, err
	}
	if s.BirthDate, err = ParseBirthDate(birthDate, now); err != nil {
		return Subject{}, err
	}
	if s.Notes, err = NormalizeNotes(notes); err != nil {
		return Subject{}, err
	}
	return s, nil
}

// NormalizeCode trims a tag or cage code. Uniqueness is case-insensitive.
func NormalizeCode(raw string) (string, error) {
	code := strings.TrimSpace(raw)
	if code == "" || len(code) > 64 {
		return "", ErrInvalidCode
	}
	for _, r := range code {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._/-", r)) {
			return "", ErrInvalidCode
		}
	}
	return code, nil
}

func ParseSpecies(raw string) (Species, error) {
	if s := Species(raw); s == Mouse || s == Rat {
		return s, nil
	}
	return "", ErrInvalidSpecies
}

func ParseSex(raw string) (Sex, error) {
	if s := Sex(raw); s == Female || s == Male || s == UnknownSex {
		return s, nil
	}
	return "", ErrInvalidSex
}

// NormalizeStrain returns "" for an unknown strain.
func NormalizeStrain(raw string) (string, error) {
	strain := strings.TrimSpace(raw)
	if !utf8.ValidString(strain) || utf8.RuneCountInString(strain) > 120 || strings.IndexFunc(strain, unicode.IsControl) >= 0 {
		return "", ErrInvalidStrain
	}
	return strain, nil
}

// ParseBirthDate returns nil for an empty value. A date one day ahead of the
// UTC calendar is accepted because laboratories east of UTC are already there.
func ParseBirthDate(raw string, now time.Time) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	date, err := time.Parse(time.DateOnly, raw)
	if err != nil || date.After(now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)) {
		return nil, ErrInvalidBirthDate
	}
	return &date, nil
}

// NormalizeNotes allows line breaks and tabs but no other control characters.
func NormalizeNotes(raw string) (string, error) {
	notes := strings.TrimSpace(raw)
	invalid := func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }
	if !utf8.ValidString(notes) || utf8.RuneCountInString(notes) > 2000 || strings.IndexFunc(notes, invalid) >= 0 {
		return "", ErrInvalidNotes
	}
	return notes, nil
}
