// Package domain holds the test protocols of an experiment. A protocol has
// numbered versions; a version is an ordered list of paradigm steps, each
// pinned to an environment revision, and never changes once stored.
package domain

import (
	"cmp"
	"errors"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// MaxSteps bounds the steps of one protocol version.
const MaxSteps = 50

var (
	ErrInvalidName            = errors.New("name must contain 1 to 120 printable characters")
	ErrInvalidDescription     = errors.New("description must contain at most 5000 characters")
	ErrInvalidNotes           = errors.New("notes must contain at most 2000 characters")
	ErrNoSteps                = errors.New("a protocol version needs at least one step")
	ErrTooManySteps           = errors.New("a protocol version has at most 50 steps")
	ErrInvalidStepOrder       = errors.New("step positions must run from 1 to the number of steps without gaps or duplicates")
	ErrInvalidParadigmKey     = errors.New("paradigm key must contain 1 to 40 capital letters or underscores")
	ErrInvalidParadigmVersion = errors.New("paradigm version must be positive")
	ErrMissingEnvironment     = errors.New("every step needs an environment revision")
	ErrInvalidTrialType       = errors.New("trial type must contain 1 to 40 capital letters or underscores")
	ErrInvalidTrials          = errors.New("trials must be between 1 and 1000")
	ErrInvalidInterval        = errors.New("inter-trial interval must be between 0 and 86400 seconds")
)

var constantKey = regexp.MustCompile(`^[A-Z][A-Z_]{0,39}$`)

type Protocol struct {
	ID           string
	ExperimentID string
	Name         string
	Description  string
	// LatestVersion is the highest version number.
	LatestVersion int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Version struct {
	ID           string
	ExperimentID string
	ProtocolID   string
	Number       int
	Notes        string
	CreatedBy    string
	// Steps are ordered by Position.
	Steps     []Step
	CreatedAt time.Time
}

// Step runs one paradigm version on one environment revision. Session holds
// every session parameter of the paradigm version once stored.
type Step struct {
	Position              int
	ParadigmKey           string
	ParadigmVersion       int
	EnvironmentRevisionID string
	// EnvironmentID and EnvironmentRevision describe the referenced revision.
	EnvironmentID       string
	EnvironmentRevision int
	TrialType           string
	Trials              int
	InterTrialIntervalS int
	Session             map[string]float64
	Notes               string
}

func NewProtocol(experimentID, name, description string) (Protocol, error) {
	p := Protocol{ExperimentID: experimentID}
	var err error
	if p.Name, err = NormalizeName(name); err != nil {
		return Protocol{}, err
	}
	if p.Description, err = NormalizeDescription(description); err != nil {
		return Protocol{}, err
	}
	return p, nil
}

// NewVersion validates steps and returns them ordered by position, with
// copied session values.
func NewVersion(protocolID, notes, createdBy string, steps []Step) (Version, error) {
	text, err := normalizeText(notes, 2000, ErrInvalidNotes)
	if err != nil {
		return Version{}, err
	}
	switch {
	case len(steps) == 0:
		return Version{}, ErrNoSteps
	case len(steps) > MaxSteps:
		return Version{}, ErrTooManySteps
	}
	ordered := make([]Step, 0, len(steps))
	for _, s := range steps {
		if err := validateStep(&s); err != nil {
			return Version{}, err
		}
		s.Session = maps.Clone(s.Session)
		if s.Session == nil {
			s.Session = map[string]float64{}
		}
		ordered = append(ordered, s)
	}
	slices.SortFunc(ordered, func(a, b Step) int { return cmp.Compare(a.Position, b.Position) })
	for i, s := range ordered {
		if s.Position != i+1 {
			return Version{}, ErrInvalidStepOrder
		}
	}
	return Version{ProtocolID: protocolID, Notes: text, CreatedBy: createdBy, Steps: ordered}, nil
}

func validateStep(s *Step) error {
	var err error
	switch {
	case !constantKey.MatchString(s.ParadigmKey):
		return ErrInvalidParadigmKey
	case s.ParadigmVersion < 1:
		return ErrInvalidParadigmVersion
	case s.EnvironmentRevisionID == "":
		return ErrMissingEnvironment
	case !constantKey.MatchString(s.TrialType):
		return ErrInvalidTrialType
	case s.Trials < 1 || s.Trials > 1000:
		return ErrInvalidTrials
	case s.InterTrialIntervalS < 0 || s.InterTrialIntervalS > 86400:
		return ErrInvalidInterval
	}
	s.Notes, err = normalizeText(s.Notes, 2000, ErrInvalidNotes)
	return err
}

func NormalizeName(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 120 || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return "", ErrInvalidName
	}
	return text, nil
}

// NormalizeDescription returns "" for no description and allows line breaks.
func NormalizeDescription(raw string) (string, error) {
	return normalizeText(raw, 5000, ErrInvalidDescription)
}

func normalizeText(raw string, maxChars int, invalidErr error) (string, error) {
	text := strings.TrimSpace(raw)
	invalid := func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxChars || strings.IndexFunc(text, invalid) >= 0 {
		return "", invalidErr
	}
	return text, nil
}
