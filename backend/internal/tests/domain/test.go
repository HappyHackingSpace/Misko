// Package domain holds tests, their animals, trials and comments. A test is
// one session of a running experiment: one timepoint, one protocol step (which
// fixes the paradigm and the planned trials) and one video. The animals filmed
// in it are its test subjects, each in its own arena. Trials belong to one
// test subject and are appended repetitions that never replace earlier ones.
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

// MaxSubjects bounds the animals of one session; arenas share a camera frame.
const MaxSubjects = 12

var (
	ErrInvalidTransition    = errors.New("the test's status does not allow this change")
	ErrInvalidStatus        = errors.New("status must be PLANNED, IN_PROGRESS, COMPLETED or CANCELLED")
	ErrMissingTime          = errors.New("time is required")
	ErrFutureTime           = errors.New("time cannot be in the future")
	ErrNoSubjects           = errors.New("a test needs at least one animal")
	ErrTooManySubjects      = errors.New("a test holds at most 12 animals")
	ErrMissingAllocation    = errors.New("every animal needs an allocation")
	ErrMissingArena         = errors.New("every animal needs an arena")
	ErrDuplicateAnimal      = errors.New("an animal can appear only once in a test")
	ErrDuplicateArena       = errors.New("each animal in a test needs its own arena")
	ErrNotPlanned           = errors.New("animals can be added or removed only while the test is planned")
	ErrLastSubject          = errors.New("the last animal of a test cannot be removed; cancel the test instead")
	ErrSubjectExcluded      = errors.New("an animal of the test has been excluded from the study")
	ErrNoTrials             = errors.New("every animal in the test needs at least one trial before it can be completed")
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

// Context is what a test is recorded against: a timepoint of the experiment
// and one step of one of its protocol versions.
type Context struct {
	ExperimentID      string
	TimepointID       string
	ProtocolVersionID string
	StepPosition      int
	ParadigmKey       string
	ParadigmVersion   int
	PlannedTrials     int
}

// Participant is an animal (its allocation) placed in an arena (an
// environment revision).
type Participant struct {
	AllocationID          string
	EnvironmentRevisionID string
}

type Test struct {
	ID           string
	ExperimentID string
	// ExperimentCode, TimepointName, StudyDay, ProtocolID, ProtocolName and
	// ProtocolVersion describe the referenced records when read back.
	ExperimentCode    string
	TimepointID       string
	TimepointName     string
	StudyDay          int
	ProtocolID        string
	ProtocolName      string
	ProtocolVersionID string
	ProtocolVersion   int
	StepPosition      int
	ParadigmKey       string
	ParadigmVersion   int
	PlannedTrials     int
	Status            Status
	ScheduledAt       time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
	CancelledAt       *time.Time
	CancelReason      string
	Notes             string
	CreatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Subjects          []TestSubject
}

// TestSubject is one animal of a test in its arena. SubjectCode, Excluded and
// the environment fields describe the referenced records when read back.
type TestSubject struct {
	ID                    string
	TestID                string
	AllocationID          string
	SubjectID             string
	SubjectCode           string
	Excluded              bool
	EnvironmentRevisionID string
	EnvironmentID         string
	EnvironmentName       string
	EnvironmentRevision   int
	CreatedBy             string
	CreatedAt             time.Time
}

// NewTest plans a session with its animals.
func NewTest(c Context, scheduledAt time.Time, notes, createdBy string, participants []Participant) (Test, error) {
	if scheduledAt.IsZero() {
		return Test{}, ErrMissingTime
	}
	text, err := normalizeText(notes, 2000, false, ErrInvalidNotes)
	if err != nil {
		return Test{}, err
	}
	if err := ValidateParticipants(participants); err != nil {
		return Test{}, err
	}
	t := Test{
		ExperimentID: c.ExperimentID, TimepointID: c.TimepointID, ProtocolVersionID: c.ProtocolVersionID, StepPosition: c.StepPosition,
		ParadigmKey: c.ParadigmKey, ParadigmVersion: c.ParadigmVersion, PlannedTrials: c.PlannedTrials,
		Status: Planned, ScheduledAt: scheduledAt, Notes: text, CreatedBy: createdBy,
	}
	for _, p := range participants {
		t.Subjects = append(t.Subjects, TestSubject{AllocationID: p.AllocationID, EnvironmentRevisionID: p.EnvironmentRevisionID, CreatedBy: createdBy})
	}
	return t, nil
}

// ValidateParticipants checks the animals of a new test: at least one, at most
// MaxSubjects, each animal once and each arena once.
func ValidateParticipants(participants []Participant) error {
	switch {
	case len(participants) == 0:
		return ErrNoSubjects
	case len(participants) > MaxSubjects:
		return ErrTooManySubjects
	}
	animals, arenas := map[string]bool{}, map[string]bool{}
	for _, p := range participants {
		switch {
		case p.AllocationID == "":
			return ErrMissingAllocation
		case p.EnvironmentRevisionID == "":
			return ErrMissingArena
		case animals[p.AllocationID]:
			return ErrDuplicateAnimal
		case arenas[p.EnvironmentRevisionID]:
			return ErrDuplicateArena
		}
		animals[p.AllocationID], arenas[p.EnvironmentRevisionID] = true, true
	}
	return nil
}

// AddSubject places another animal in a planned test.
func (t Test) AddSubject(p Participant, createdBy string) (TestSubject, error) {
	if t.Status != Planned {
		return TestSubject{}, ErrNotPlanned
	}
	participants := make([]Participant, 0, len(t.Subjects)+1)
	for _, s := range t.Subjects {
		participants = append(participants, Participant{s.AllocationID, s.EnvironmentRevisionID})
	}
	if err := ValidateParticipants(append(participants, p)); err != nil {
		return TestSubject{}, err
	}
	return TestSubject{TestID: t.ID, AllocationID: p.AllocationID, EnvironmentRevisionID: p.EnvironmentRevisionID, CreatedBy: createdBy}, nil
}

// Subject returns the test subject with id, or false.
func (t Test) Subject(id string) (TestSubject, bool) {
	for _, s := range t.Subjects {
		if s.ID == id {
			return s, true
		}
	}
	return TestSubject{}, false
}

// CanRemoveSubject allows removing an animal while the test is planned and
// other animals remain.
func (t Test) CanRemoveSubject() error {
	switch {
	case t.Status != Planned:
		return ErrNotPlanned
	case len(t.Subjects) <= 1:
		return ErrLastSubject
	}
	return nil
}

// Start begins the session at at. Excluded animals must be removed first.
func (t Test) Start(at, now time.Time) (Test, error) {
	switch {
	case !CanTransition(t.Status, InProgress):
		return Test{}, ErrInvalidTransition
	case at.IsZero():
		return Test{}, ErrMissingTime
	case at.After(now.Add(ClockSkew)):
		return Test{}, ErrFutureTime
	case len(t.Subjects) == 0:
		return Test{}, ErrNoSubjects
	}
	for _, s := range t.Subjects {
		if s.Excluded {
			return Test{}, ErrSubjectExcluded
		}
	}
	t.Status, t.StartedAt = InProgress, &at
	return t, nil
}

// Complete ends the session at at, after its start and every recorded trial.
// Every animal needs at least one trial.
func (t Test) Complete(at, now time.Time, trials []Trial) (Test, error) {
	switch {
	case !CanTransition(t.Status, Completed):
		return Test{}, ErrInvalidTransition
	case at.IsZero():
		return Test{}, ErrMissingTime
	case at.After(now.Add(ClockSkew)):
		return Test{}, ErrFutureTime
	case t.StartedAt != nil && at.Before(*t.StartedAt):
		return Test{}, ErrCompletedBeforeTrial
	case len(t.Subjects) == 0:
		return Test{}, ErrNoTrials
	}
	tried := map[string]bool{}
	for _, tr := range trials {
		tried[tr.TestSubjectID] = true
		last := tr.StartedAt
		if tr.EndedAt != nil {
			last = *tr.EndedAt
		}
		if at.Before(last) {
			return Test{}, ErrCompletedBeforeTrial
		}
	}
	for _, s := range t.Subjects {
		if !tried[s.ID] {
			return Test{}, ErrNoTrials
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

// Trial is one recorded repetition of one animal. Repeating a repetition adds
// an attempt.
type Trial struct {
	ID            string
	TestID        string
	TestSubjectID string
	Number        int
	Repetition    int
	Attempt       int
	StartedAt     time.Time
	EndedAt       *time.Time
	Notes         string
	RecordedBy    string
	CreatedAt     time.Time
}

type TrialInput struct {
	Repetition int
	StartedAt  time.Time
	EndedAt    *time.Time
	Notes      string
}

// NextTrial validates a trial of one test subject given that subject's
// earlier trials and numbers it.
func NextTrial(t Test, subjectID string, earlier []Trial, in TrialInput, recordedBy string, now time.Time) (Trial, error) {
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
	number, attempt := 1, 1
	for _, e := range earlier {
		if e.TestSubjectID != subjectID {
			continue
		}
		number++
		if e.Repetition == in.Repetition {
			attempt++
		}
	}
	return Trial{
		TestID: t.ID, TestSubjectID: subjectID, Number: number, Repetition: in.Repetition, Attempt: attempt,
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
