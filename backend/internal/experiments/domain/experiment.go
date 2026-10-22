// Package domain holds the study design of an experiment: its lifecycle, its
// groups (role and disease model), what each group receives (regimen items),
// when animals are tested (timepoints), what the study is judged on
// (endpoints) and which animal is in which group (allocations).
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidCode          = errors.New("code must contain 1 to 64 ASCII letters, digits, '.', '_', '/' or '-'")
	ErrInvalidTitle         = errors.New("title must contain 1 to 200 printable characters")
	ErrInvalidDescription   = errors.New("description must contain at most 5000 characters")
	ErrInvalidHypothesis    = errors.New("hypothesis must contain at most 2000 characters")
	ErrInvalidRandomisation = errors.New("randomisation must contain at most 500 characters")
	ErrInvalidStatus        = errors.New("status must be DRAFT, RUNNING or COMPLETED")
)

type Status string

const (
	Draft     Status = "DRAFT"
	Running   Status = "RUNNING"
	Completed Status = "COMPLETED"
)

func ParseStatus(raw string) (Status, error) {
	if s := Status(raw); s == Draft || s == Running || s == Completed {
		return s, nil
	}
	return "", ErrInvalidStatus
}

// Experiment is one study. The design can change only in DRAFT; starting sets
// day 0 (a calendar date) and freezes it; COMPLETED is read-only.
type Experiment struct {
	ID            string
	Code          string
	Title         string
	Description   string
	Hypothesis    string
	Randomisation string
	Blinded       bool
	Status        Status
	// DayZero is the date study days count from, set when the study starts.
	DayZero     *time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ExperimentFields are the editable fields of an experiment.
type ExperimentFields struct {
	Code, Title, Description, Hypothesis, Randomisation string
	Blinded                                             bool
}

// NewExperiment validates the fields of a new draft experiment.
func NewExperiment(f ExperimentFields) (Experiment, error) {
	e := Experiment{Status: Draft}
	if err := e.Apply(f); err != nil {
		return Experiment{}, err
	}
	return e, nil
}

// Apply validates f and copies it onto the experiment.
func (e *Experiment) Apply(f ExperimentFields) error {
	var err error
	out := *e
	if out.Code, err = NormalizeCode(f.Code); err != nil {
		return err
	}
	if out.Title, err = line(f.Title, 200, ErrInvalidTitle); err != nil {
		return err
	}
	if out.Description, err = text(f.Description, 5000, ErrInvalidDescription); err != nil {
		return err
	}
	if out.Hypothesis, err = text(f.Hypothesis, 2000, ErrInvalidHypothesis); err != nil {
		return err
	}
	if out.Randomisation, err = text(f.Randomisation, 500, ErrInvalidRandomisation); err != nil {
		return err
	}
	out.Blinded = f.Blinded
	*e = out
	return nil
}

// Fields returns the editable fields, the base a partial update starts from.
func (e Experiment) Fields() ExperimentFields {
	return ExperimentFields{Code: e.Code, Title: e.Title, Description: e.Description, Hypothesis: e.Hypothesis, Randomisation: e.Randomisation, Blinded: e.Blinded}
}

// DesignChanged reports whether f changes a field that belongs to the frozen
// study design (hypothesis, randomisation, blinding). Code, title and
// description stay editable while the study runs.
func (e Experiment) DesignChanged(f ExperimentFields) bool {
	return f.Hypothesis != e.Hypothesis || f.Randomisation != e.Randomisation || f.Blinded != e.Blinded
}

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

// line is a required single-line text.
func line(raw string, maxChars int, invalid error) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxChars || strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", invalid
	}
	return s, nil
}

// text is an optional multi-line text; "" means none.
func text(raw string, maxChars int, invalid error) (string, error) {
	s := strings.TrimSpace(raw)
	bad := func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxChars || strings.IndexFunc(s, bad) >= 0 {
		return "", invalid
	}
	return s, nil
}
