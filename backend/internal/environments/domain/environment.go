// Package domain holds environments: physical apparatus set up for one
// paradigm. Measurements live in numbered revisions that never change once
// stored, so anything that references a revision keeps its exact geometry.
package domain

import (
	"errors"
	"maps"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidName            = errors.New("name must contain 1 to 120 printable characters")
	ErrInvalidNotes           = errors.New("notes must contain at most 2000 characters")
	ErrInvalidParadigmKey     = errors.New("paradigm key must contain 1 to 40 capital letters or underscores")
	ErrInvalidParadigmVersion = errors.New("paradigm version must be positive")
)

var paradigmKey = regexp.MustCompile(`^[A-Z][A-Z_]{0,39}$`)

type Environment struct {
	ID          string
	Name        string
	ParadigmKey string
	Notes       string
	// LatestRevision is the highest revision number.
	LatestRevision int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Revision is one set of apparatus measurements validated against a paradigm
// version. Number counts revisions of the same environment from 1.
type Revision struct {
	ID              string
	EnvironmentID   string
	ParadigmKey     string
	Number          int
	ParadigmVersion int
	Apparatus       map[string]float64
	Notes           string
	CreatedBy       string
	CreatedAt       time.Time
}

func NewEnvironment(name, key, notes string) (Environment, error) {
	var e Environment
	var err error
	if e.Name, err = NormalizeName(name); err != nil {
		return Environment{}, err
	}
	if err = ValidateParadigmKey(key); err != nil {
		return Environment{}, err
	}
	e.ParadigmKey = key
	if e.Notes, err = NormalizeNotes(notes); err != nil {
		return Environment{}, err
	}
	return e, nil
}

// NewRevision copies apparatus so later changes by the caller cannot alter it.
func NewRevision(environmentID, key string, version int, apparatus map[string]float64, notes, createdBy string) (Revision, error) {
	if err := ValidateParadigmKey(key); err != nil {
		return Revision{}, err
	}
	if version < 1 {
		return Revision{}, ErrInvalidParadigmVersion
	}
	text, err := NormalizeNotes(notes)
	if err != nil {
		return Revision{}, err
	}
	values := maps.Clone(apparatus)
	if values == nil {
		values = map[string]float64{}
	}
	return Revision{EnvironmentID: environmentID, ParadigmKey: key, ParadigmVersion: version, Apparatus: values, Notes: text, CreatedBy: createdBy}, nil
}

func NormalizeName(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 120 || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return "", ErrInvalidName
	}
	return text, nil
}

// NormalizeNotes returns "" for no notes and allows line breaks.
func NormalizeNotes(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	invalid := func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > 2000 || strings.IndexFunc(text, invalid) >= 0 {
		return "", ErrInvalidNotes
	}
	return text, nil
}

func ValidateParadigmKey(key string) error {
	if !paradigmKey.MatchString(key) {
		return ErrInvalidParadigmKey
	}
	return nil
}
