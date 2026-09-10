// Package domain holds experiments, their measurement phases and arms, and the
// enrollment and dated group assignment of subjects. A phase is when a subject
// is measured; a group is the arm it is assigned to. Neither implies the other.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidCode        = errors.New("code must contain 1 to 64 ASCII letters, digits, '.', '_', '/' or '-'")
	ErrInvalidTitle       = errors.New("title must contain 1 to 200 printable characters")
	ErrInvalidDescription = errors.New("description must contain at most 5000 characters")
	ErrInvalidName        = errors.New("name must contain 1 to 120 printable characters")
	ErrInvalidPosition    = errors.New("position must be between 1 and 1000")
	ErrInvalidRole        = errors.New("role must be CONTROL or TREATMENT")
	ErrInvalidTargetSize  = errors.New("target size must be between 0 and 100000")

	ErrGroupNotInExperiment       = errors.New("group does not belong to this experiment")
	ErrAssignmentBeforeEnrollment = errors.New("assignment cannot start before enrollment")
	ErrAssignmentOutOfOrder       = errors.New("assignment must start after the current assignment")
	ErrAlreadyInGroup             = errors.New("subject is already assigned to this group")
)

type Experiment struct {
	ID          string
	Code        string
	Title       string
	Description string
	// RequiresControl states that the design needs at least one control group.
	RequiresControl bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewExperiment(code, title, description string, requiresControl bool) (Experiment, error) {
	e := Experiment{RequiresControl: requiresControl}
	var err error
	if e.Code, err = NormalizeCode(code); err != nil {
		return Experiment{}, err
	}
	if e.Title, err = NormalizeTitle(title); err != nil {
		return Experiment{}, err
	}
	if e.Description, err = NormalizeDescription(description); err != nil {
		return Experiment{}, err
	}
	return e, nil
}

// Phase is an ordered measurement period, such as a healthy baseline.
type Phase struct {
	ID           string
	ExperimentID string
	Name         string
	Position     int
	Description  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewPhase(experimentID, name string, position int, description string) (Phase, error) {
	p := Phase{ExperimentID: experimentID, Position: position}
	var err error
	if p.Name, err = NormalizeName(name); err != nil {
		return Phase{}, err
	}
	if err = ValidatePosition(position); err != nil {
		return Phase{}, err
	}
	if p.Description, err = NormalizeDescription(description); err != nil {
		return Phase{}, err
	}
	return p, nil
}

type GroupRole string

const (
	Control   GroupRole = "CONTROL"
	Treatment GroupRole = "TREATMENT"
)

func ParseRole(raw string) (GroupRole, error) {
	if r := GroupRole(raw); r == Control || r == Treatment {
		return r, nil
	}
	return "", ErrInvalidRole
}

// Group is an experimental arm. TargetSize is the planned number of subjects;
// zero means no target was set.
type Group struct {
	ID           string
	ExperimentID string
	Name         string
	Role         GroupRole
	TargetSize   int
	Description  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewGroup(experimentID, name, role string, targetSize int, description string) (Group, error) {
	g := Group{ExperimentID: experimentID, TargetSize: targetSize}
	var err error
	if g.Name, err = NormalizeName(name); err != nil {
		return Group{}, err
	}
	if g.Role, err = ParseRole(role); err != nil {
		return Group{}, err
	}
	if err = ValidateTargetSize(targetSize); err != nil {
		return Group{}, err
	}
	if g.Description, err = NormalizeDescription(description); err != nil {
		return Group{}, err
	}
	return g, nil
}

// Enrollment connects a subject to one experiment; a subject may be enrolled
// in many experiments but only once in each.
type Enrollment struct {
	ID           string
	ExperimentID string
	SubjectID    string
	EnrolledAt   time.Time
	CreatedAt    time.Time
}

// Assignment places an enrollment in a group for [ValidFrom, ValidTo). A nil
// ValidTo is open-ended. Periods of one enrollment never overlap.
type Assignment struct {
	ID           string
	ExperimentID string
	EnrollmentID string
	GroupID      string
	ValidFrom    time.Time
	ValidTo      *time.Time
	CreatedAt    time.Time
}

func (a Assignment) ActiveAt(t time.Time) bool {
	return !t.Before(a.ValidFrom) && (a.ValidTo == nil || t.Before(*a.ValidTo))
}

// PlanAssignment checks moving an enrollment into group from the given time.
// current is the open-ended assignment, if any; it will be closed at from.
// Crossover is modeled by history, so earlier periods are never rewritten.
func PlanAssignment(e Enrollment, current *Assignment, g Group, from time.Time) error {
	if g.ExperimentID != e.ExperimentID {
		return ErrGroupNotInExperiment
	}
	if from.Before(e.EnrolledAt) {
		return ErrAssignmentBeforeEnrollment
	}
	if current != nil {
		if current.GroupID == g.ID {
			return ErrAlreadyInGroup
		}
		if !from.After(current.ValidFrom) {
			return ErrAssignmentOutOfOrder
		}
	}
	return nil
}

// GroupAt returns the group assigned at t. It depends only on the assignment
// history, never on phases or a subject's condition.
func GroupAt(history []Assignment, t time.Time) (string, bool) {
	for _, a := range history {
		if a.ActiveAt(t) {
			return a.GroupID, true
		}
	}
	return "", false
}

// OpenAssignment returns the open-ended assignment, which is always the latest.
func OpenAssignment(history []Assignment) *Assignment {
	for i := range history {
		if history[i].ValidTo == nil {
			return &history[i]
		}
	}
	return nil
}

func ControlRequirementMet(requiresControl bool, groups []Group) bool {
	if !requiresControl {
		return true
	}
	for _, g := range groups {
		if g.Role == Control {
			return true
		}
	}
	return false
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

func NormalizeTitle(raw string) (string, error) { return line(raw, 200, ErrInvalidTitle) }

func NormalizeName(raw string) (string, error) { return line(raw, 120, ErrInvalidName) }

// NormalizeDescription returns "" for no description and allows line breaks.
func NormalizeDescription(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	invalid := func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > 5000 || strings.IndexFunc(text, invalid) >= 0 {
		return "", ErrInvalidDescription
	}
	return text, nil
}

func ValidatePosition(position int) error {
	if position < 1 || position > 1000 {
		return ErrInvalidPosition
	}
	return nil
}

func ValidateTargetSize(size int) error {
	if size < 0 || size > 100000 {
		return ErrInvalidTargetSize
	}
	return nil
}

func line(raw string, maxChars int, invalid error) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxChars || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return "", invalid
	}
	return text, nil
}
