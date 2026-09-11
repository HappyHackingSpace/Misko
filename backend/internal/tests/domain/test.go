// Package domain holds tests, their trials and comments. A test is one session
// of a subject on one protocol step, which fixes the paradigm and environment
// revision. Trials are appended repetitions and never replace earlier ones.
// Test status is the session's own lifecycle; analysis status is separate.
package domain

import (
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ClockSkew is how far in the future a recorded time may be.
const ClockSkew = 2 * time.Minute

var (
	ErrInvalidTransition    = errors.New("the test's status does not allow this change")
	ErrInvalidStatus        = errors.New("status must be PLANNED, IN_PROGRESS, COMPLETED or CANCELLED")
	ErrMissingTime          = errors.New("time is required")
	ErrFutureTime           = errors.New("time cannot be in the future")
	ErrBeforeEnrollment     = errors.New("time cannot be before the subject's enrollment")
	ErrGroupChanged         = errors.New("the subject's group at this time differs from the group the test was planned for")
	ErrNoTrials             = errors.New("a test needs at least one trial before it can be completed")
	ErrCompletedBeforeTrial = errors.New("completion cannot be before the test start or the end of a trial")
	ErrInvalidReason        = errors.New("reason must contain 1 to 2000 characters")
	ErrInvalidNotes         = errors.New("notes must contain at most 2000 characters")
	ErrTestNotInProgress    = errors.New("trials can be recorded only while the test is in progress")
	ErrInvalidRepetition    = errors.New("repetition must be between 1 and the planned number of trials")
	ErrTrialBeforeTest      = errors.New("a trial cannot start before its test started")
	ErrInvalidTrialEnd      = errors.New("a trial must end after it starts")
	ErrInvalidComment       = errors.New("comment must contain 1 to 2000 characters")
)

type Status string

const (
	Planned    Status = "PLANNED"
	InProgress Status = "IN_PROGRESS"
	Completed  Status = "COMPLETED"
	Cancelled  Status = "CANCELLED"
)

func ParseStatus(raw string) (Status, error) {
	switch s := Status(raw); s {
	case Planned, InProgress, Completed, Cancelled:
		return s, nil
	}
	return "", ErrInvalidStatus
}

// CanTransition lists the only status changes: planned tests start or are
// cancelled, and tests in progress complete or are cancelled.
func CanTransition(from, to Status) bool {
	switch from {
	case Planned:
		return to == InProgress || to == Cancelled
	case InProgress:
		return to == Completed || to == Cancelled
	}
	return false
}

// Context is what a test is recorded against. GroupID is the subject's group
// at the scheduled time, or empty when it had none.
type Context struct {
	ExperimentID          string
	EnrollmentID          string
	SubjectID             string
	EnrolledAt            time.Time
	PhaseID               string
	GroupID               string
	ProtocolVersionID     string
	StepPosition          int
	ParadigmKey           string
	ParadigmVersion       int
	EnvironmentRevisionID string
	PlannedTrials         int
}

type Test struct {
	ID                    string
	ExperimentID          string
	EnrollmentID          string
	SubjectID             string
	PhaseID               string
	GroupID               string
	ProtocolVersionID     string
	StepPosition          int
	ParadigmKey           string
	ParadigmVersion       int
	EnvironmentRevisionID string
	PlannedTrials         int
	Status                Status
	ScheduledAt           time.Time
	StartedAt             *time.Time
	CompletedAt           *time.Time
	CancelledAt           *time.Time
	CancelReason          string
	Notes                 string
	CreatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func NewTest(c Context, scheduledAt time.Time, notes, createdBy string) (Test, error) {
	switch {
	case scheduledAt.IsZero():
		return Test{}, ErrMissingTime
	case scheduledAt.Before(c.EnrolledAt):
		return Test{}, ErrBeforeEnrollment
	}
	text, err := normalizeText(notes, 2000, false, ErrInvalidNotes)
	if err != nil {
		return Test{}, err
	}
	return Test{
		ExperimentID: c.ExperimentID, EnrollmentID: c.EnrollmentID, SubjectID: c.SubjectID, PhaseID: c.PhaseID, GroupID: c.GroupID,
		ProtocolVersionID: c.ProtocolVersionID, StepPosition: c.StepPosition, ParadigmKey: c.ParadigmKey, ParadigmVersion: c.ParadigmVersion,
		EnvironmentRevisionID: c.EnvironmentRevisionID, PlannedTrials: c.PlannedTrials,
		Status: Planned, ScheduledAt: scheduledAt, Notes: text, CreatedBy: createdBy,
	}, nil
}

// Start begins the session at at. groupAt is the subject's group at that time;
// it must be the group the test was planned for.
func (t Test) Start(at, now, enrolledAt time.Time, groupAt string) (Test, error) {
	switch {
	case !CanTransition(t.Status, InProgress):
		return Test{}, ErrInvalidTransition
	case at.IsZero():
		return Test{}, ErrMissingTime
	case at.After(now.Add(ClockSkew)):
		return Test{}, ErrFutureTime
	case at.Before(enrolledAt):
		return Test{}, ErrBeforeEnrollment
	case groupAt != t.GroupID:
		return Test{}, ErrGroupChanged
	}
	t.Status, t.StartedAt = InProgress, &at
	return t, nil
}

// Complete ends the session at at, after its start and every recorded trial.
func (t Test) Complete(at, now time.Time, trials []Trial) (Test, error) {
	switch {
	case !CanTransition(t.Status, Completed):
		return Test{}, ErrInvalidTransition
	case len(trials) == 0:
		return Test{}, ErrNoTrials
	case at.IsZero():
		return Test{}, ErrMissingTime
	case at.After(now.Add(ClockSkew)):
		return Test{}, ErrFutureTime
	case t.StartedAt != nil && at.Before(*t.StartedAt):
		return Test{}, ErrCompletedBeforeTrial
	}
	for _, tr := range trials {
		last := tr.StartedAt
		if tr.EndedAt != nil {
			last = *tr.EndedAt
		}
		if at.Before(last) {
			return Test{}, ErrCompletedBeforeTrial
		}
	}
	t.Status, t.CompletedAt = Completed, &at
	return t, nil
}

func (t Test) Cancel(reason string, now time.Time) (Test, error) {
	if !CanTransition(t.Status, Cancelled) {
		return Test{}, ErrInvalidTransition
	}
	text, err := normalizeText(reason, 2000, true, ErrInvalidReason)
	if err != nil {
		return Test{}, err
	}
	t.Status, t.CancelledAt, t.CancelReason = Cancelled, &now, text
	return t, nil
}

// Trial is one recorded repetition. Repeating a repetition adds an attempt.
type Trial struct {
	ID         string
	TestID     string
	Number     int
	Repetition int
	Attempt    int
	StartedAt  time.Time
	EndedAt    *time.Time
	Notes      string
	RecordedBy string
	CreatedAt  time.Time
}

type TrialInput struct {
	Repetition int
	StartedAt  time.Time
	EndedAt    *time.Time
	Notes      string
}

// NextTrial validates a trial for a test given its earlier trials and numbers it.
func NextTrial(t Test, earlier []Trial, in TrialInput, recordedBy string, now time.Time) (Trial, error) {
	switch {
	case t.Status != InProgress:
		return Trial{}, ErrTestNotInProgress
	case in.Repetition < 1 || in.Repetition > t.PlannedTrials:
		return Trial{}, ErrInvalidRepetition
	case in.StartedAt.IsZero():
		return Trial{}, ErrMissingTime
	case in.StartedAt.After(now.Add(ClockSkew)):
		return Trial{}, ErrFutureTime
	case t.StartedAt != nil && in.StartedAt.Before(*t.StartedAt):
		return Trial{}, ErrTrialBeforeTest
	case in.EndedAt != nil && !in.EndedAt.After(in.StartedAt):
		return Trial{}, ErrInvalidTrialEnd
	case in.EndedAt != nil && in.EndedAt.After(now.Add(ClockSkew)):
		return Trial{}, ErrFutureTime
	}
	notes, err := normalizeText(in.Notes, 2000, false, ErrInvalidNotes)
	if err != nil {
		return Trial{}, err
	}
	attempt := 1
	for _, e := range earlier {
		if e.Repetition == in.Repetition {
			attempt++
		}
	}
	return Trial{
		TestID: t.ID, Number: len(earlier) + 1, Repetition: in.Repetition, Attempt: attempt,
		StartedAt: in.StartedAt, EndedAt: in.EndedAt, Notes: notes, RecordedBy: recordedBy,
	}, nil
}

// Comment is plain text on a test. AuthorName is empty once the author's
// account no longer exists.
type Comment struct {
	ID         string
	TestID     string
	AuthorID   string
	AuthorName string
	Body       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// EditableBy allows only the author.
func (c Comment) EditableBy(actor access.Actor) bool {
	return actor.UserID != "" && actor.UserID == c.AuthorID
}

// DeletableBy allows the author or a privileged role for moderation.
func (c Comment) DeletableBy(actor access.Actor) bool {
	return c.EditableBy(actor) || (actor.UserID != "" && actor.Role.Privileged())
}

// NormalizeComment removes control characters other than tab and line breaks,
// trims the text and requires 1 to 2000 characters.
func NormalizeComment(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", ErrInvalidComment
	}
	text := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, raw))
	if text == "" || utf8.RuneCountInString(text) > 2000 {
		return "", ErrInvalidComment
	}
	return text, nil
}

func normalizeText(raw string, maxChars int, required bool, invalidErr error) (string, error) {
	text := strings.TrimSpace(raw)
	invalid := func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }
	if (required && text == "") || !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxChars || strings.IndexFunc(text, invalid) >= 0 {
		return "", invalidErr
	}
	return text, nil
}
