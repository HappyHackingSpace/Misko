// Package domain holds laboratory animals. A subject's identity is independent
// of the experiments it is enrolled in.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
)

var (
	ErrInvalidCode    = errors.New("code must contain 1 to 64 ASCII letters, digits, '.', '_', '/' or '-'")
	ErrInvalidSpecies = errors.New("species must be MOUSE or RAT")
	ErrInvalidSex     = errors.New("sex must be FEMALE, MALE or UNKNOWN")
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
	ID        string
	Code      string
	Species   Species
	Sex       Sex
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewSubject(code, species, sex string) (Subject, error) {
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
