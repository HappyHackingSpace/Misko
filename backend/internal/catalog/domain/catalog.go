// Package domain holds the laboratory-wide catalog reused across studies:
// substances given to animals and the disease models a study induces.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidName                  = errors.New("name must contain 1 to 120 printable characters")
	ErrInvalidDescription           = errors.New("description must contain at most 5000 characters")
	ErrInvalidSubstanceKind         = errors.New("kind must be COMPOUND, REFERENCE_DRUG, VEHICLE or INDUCER")
	ErrInvalidInductionType         = errors.New("induction type must be CHEMICAL, SURGICAL, DIET, STRESS or OTHER")
	ErrInvalidConfirmationCriterion = errors.New("confirmation criterion must contain at most 2000 characters")
)

// SubstanceKind only guides pickers; the role of a substance in a study comes
// from the regimen item that uses it.
type SubstanceKind string

const (
	Compound      SubstanceKind = "COMPOUND"
	ReferenceDrug SubstanceKind = "REFERENCE_DRUG"
	Vehicle       SubstanceKind = "VEHICLE"
	Inducer       SubstanceKind = "INDUCER"
)

func ParseSubstanceKind(raw string) (SubstanceKind, error) {
	switch k := SubstanceKind(raw); k {
	case Compound, ReferenceDrug, Vehicle, Inducer:
		return k, nil
	}
	return "", ErrInvalidSubstanceKind
}

// InductionType is how a disease model produces its deficit.
type InductionType string

const (
	Chemical InductionType = "CHEMICAL"
	Surgical InductionType = "SURGICAL"
	Diet     InductionType = "DIET"
	Stress   InductionType = "STRESS"
	Other    InductionType = "OTHER"
)

func ParseInductionType(raw string) (InductionType, error) {
	switch t := InductionType(raw); t {
	case Chemical, Surgical, Diet, Stress, Other:
		return t, nil
	}
	return "", ErrInvalidInductionType
}

// Substance is anything given to an animal: an active compound, a reference
// drug, a vehicle or an agent that induces a disease model.
type Substance struct {
	ID          string
	Name        string
	Kind        SubstanceKind
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewSubstance(name, kind, description string) (Substance, error) {
	var s Substance
	var err error
	if s.Name, err = NormalizeName(name); err != nil {
		return Substance{}, err
	}
	if s.Kind, err = ParseSubstanceKind(kind); err != nil {
		return Substance{}, err
	}
	if s.Description, err = NormalizeDescription(description); err != nil {
		return Substance{}, err
	}
	return s, nil
}

// DiseaseModel says how a deficit is produced and how it is confirmed. The
// inducing agent and its dose belong to each study's regimen.
type DiseaseModel struct {
	ID                    string
	Name                  string
	InductionType         InductionType
	ConfirmationCriterion string
	Description           string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func NewDiseaseModel(name, inductionType, criterion, description string) (DiseaseModel, error) {
	var m DiseaseModel
	var err error
	if m.Name, err = NormalizeName(name); err != nil {
		return DiseaseModel{}, err
	}
	if m.InductionType, err = ParseInductionType(inductionType); err != nil {
		return DiseaseModel{}, err
	}
	if m.ConfirmationCriterion, err = NormalizeConfirmationCriterion(criterion); err != nil {
		return DiseaseModel{}, err
	}
	if m.Description, err = NormalizeDescription(description); err != nil {
		return DiseaseModel{}, err
	}
	return m, nil
}

// NormalizeName trims a catalog name. Names are unique ignoring case.
func NormalizeName(raw string) (string, error) { return text(raw, 1, 120, false, ErrInvalidName) }

// NormalizeDescription returns "" for no description and allows line breaks.
func NormalizeDescription(raw string) (string, error) {
	return text(raw, 0, 5000, true, ErrInvalidDescription)
}

// NormalizeConfirmationCriterion returns "" for no criterion and allows line breaks.
func NormalizeConfirmationCriterion(raw string) (string, error) {
	return text(raw, 0, 2000, true, ErrInvalidConfirmationCriterion)
}

func text(raw string, minChars, maxChars int, multiline bool, invalid error) (string, error) {
	s := strings.TrimSpace(raw)
	bad := func(r rune) bool {
		return unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t'))
	}
	n := utf8.RuneCountInString(s)
	if !utf8.ValidString(s) || n < minChars || n > maxChars || strings.IndexFunc(s, bad) >= 0 {
		return "", invalid
	}
	return s, nil
}
