// Package application contains test, test subject, trial and comment use
// cases. Planning tests (creating, adding or removing animals, cancelling)
// needs test:write; starting, completing and recording trials needs test:run;
// reads need *:read. Every authenticated role may comment; only the author
// edits and the author or a privileged role deletes.
package application

import (
	"context"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/domain"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrExperimentNotFound      = errors.New("experiment not found")
	ErrExperimentNotRunning    = errors.New("tests can be planned only while the experiment is running")
	ErrTimepointNotFound       = errors.New("timepoint not found in this experiment")
	ErrProtocolVersionNotFound = errors.New("protocol version not found in this experiment")
	ErrStepNotFound            = errors.New("step not found in this protocol version")
	ErrAllocationNotFound      = errors.New("animal is not allocated in this experiment")
	ErrAnimalExcluded          = errors.New("the animal has been excluded from the study")
	ErrAnimalAlreadyScheduled  = errors.New("the animal is already in another test of this timepoint and protocol step")
	ErrArenaNotFound           = errors.New("arena (environment revision) not found")
	ErrArenaParadigmMismatch   = errors.New("the arena is set up for another paradigm or paradigm version than the protocol step")
	ErrTestNotFound            = errors.New("test not found")
	ErrTestSubjectNotFound     = errors.New("animal not found in this test")
	ErrCommentNotFound         = errors.New("comment not found on this test")
	ErrCannotEditComment       = errors.New("only the author can edit a comment")
	ErrCannotDeleteComment     = errors.New("only the author or a lab manager can delete a comment")
	ErrRateLimited             = errors.New("too many comment changes; try again in a minute")
	ErrInvalidQuery            = errors.New("invalid list query")
)

// Running is the experiment status that allows planning tests.
const Running = "RUNNING"

// TimepointRef is the timepoint a test is planned at. DefaultScheduledAt is
// day 0 plus the study day at midnight in the laboratory's time zone.
type TimepointRef struct {
	ID                 string
	StudyDay           int
	DefaultScheduledAt time.Time
}

// StepRef is the protocol step a test runs.
type StepRef struct {
	ProtocolVersionID string
	Position          int
	ParadigmKey       string
	ParadigmVersion   int
	Trials            int
}

// AllocationRef is an animal's allocation in the test's experiment.
type AllocationRef struct {
	ID, SubjectID string
	Excluded      bool
}

// ArenaRef is an environment revision an animal is tested in.
type ArenaRef struct {
	ID, EnvironmentID string
	ParadigmKey       string
	ParadigmVersion   int
}

// Slot is what an animal can be tested in at most once: a timepoint and a
// protocol step.
type Slot struct {
	TimepointID, ProtocolVersionID string
	StepPosition                   int
}

// TestFilter selects tests. Empty fields do not filter. From and To are
// inclusive calendar dates in the laboratory's time zone.
type TestFilter struct {
	ExperimentID, TimepointID, SubjectID, ParadigmKey string
	// Search matches the study code, the timepoint name or a subject code.
	Search        string
	Statuses      []domain.Status
	From, To      string
	NewestFirst   bool
	Limit, Offset int
}

// Store persists tests. Scoped lookups return the matching not-found error for
// records of another experiment or test. Tests are returned with their
// subjects.
type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	// ExperimentStatus returns ErrExperimentNotFound for an unknown experiment.
	ExperimentStatus(ctx context.Context, experimentID string) (string, error)
	Timepoint(ctx context.Context, experimentID, timepointID string) (TimepointRef, error)
	ProtocolStep(ctx context.Context, experimentID, versionID string, position int) (StepRef, error)
	// LockAllocations returns the allocations of the experiment among ids and
	// locks them until the transaction ends, so tests of one animal are
	// planned one at a time and an exclusion waits for the plan.
	LockAllocations(ctx context.Context, experimentID string, ids []string) ([]AllocationRef, error)
	Arenas(ctx context.Context, ids []string) ([]ArenaRef, error)
	// ScheduledAllocations returns those of ids already in a non-cancelled
	// test of the slot other than exceptTestID.
	ScheduledAllocations(ctx context.Context, slot Slot, ids []string, exceptTestID string) ([]string, error)
	// CreateTest stores t with its subjects.
	CreateTest(ctx context.Context, t domain.Test) (domain.Test, error)
	AddTestSubject(ctx context.Context, experimentID string, s domain.TestSubject, paradigmKey string, paradigmVersion int) error
	RemoveTestSubject(ctx context.Context, testID, subjectID string) error
	Test(ctx context.Context, id string) (domain.Test, error)
	// LockTest locks the test until the transaction ends.
	LockTest(ctx context.Context, id string) (domain.Test, error)
	// UpdateTestStatus stores t only while the stored status is still from.
	UpdateTestStatus(ctx context.Context, t domain.Test, from domain.Status) (domain.Test, error)
	ListTests(ctx context.Context, f TestFilter) ([]domain.Test, int, error)
	// Trials returns the trials of every subject of a test.
	Trials(ctx context.Context, testID string) ([]domain.Trial, error)
	CreateTrial(ctx context.Context, tr domain.Trial) (domain.Trial, error)
	Comments(ctx context.Context, testID string) ([]domain.Comment, error)
	Comment(ctx context.Context, testID, commentID string) (domain.Comment, error)
	CreateComment(ctx context.Context, c domain.Comment) (domain.Comment, error)
	UpdateComment(ctx context.Context, testID, commentID, body string) (domain.Comment, error)
	DeleteComment(ctx context.Context, testID, commentID string) error
}

// Limiter bounds how often one user may create or edit comments.
type Limiter interface {
	Allow(key string) bool
}

type Service struct {
	store   Store
	limiter Limiter
	now     func() time.Time
}

func New(store Store, limiter Limiter, now func() time.Time) *Service {
	return &Service{store: store, limiter: limiter, now: now}
}

type TestInput struct {
	TimepointID, ProtocolVersionID string
	StepPosition                   int
	// ScheduledAt defaults to day 0 plus the timepoint's study day.
	ScheduledAt time.Time
	Notes       string
	Subjects    []domain.Participant
}

// CreateTest plans a session of a running experiment at a timepoint on one
// protocol step with its animals. The paradigm and planned trials come from
// the step; every arena must run that paradigm version, and no animal may
// already be in another non-cancelled test of the same timepoint and step.
func (s *Service) CreateTest(ctx context.Context, actor access.Actor, experimentID string, in TestInput) (domain.Test, error) {
	if err := actor.Require(access.TestWrite); err != nil {
		return domain.Test{}, err
	}
	if in.StepPosition < 1 {
		return domain.Test{}, ErrStepNotFound
	}
	if err := domain.ValidateParticipants(in.Subjects); err != nil {
		return domain.Test{}, err
	}
	var created domain.Test
	err := s.store.Transaction(ctx, func(tx Store) error {
		status, err := tx.ExperimentStatus(ctx, experimentID)
		if err != nil {
			return err
		}
		if status != Running {
			return ErrExperimentNotRunning
		}
		timepoint, err := tx.Timepoint(ctx, experimentID, in.TimepointID)
		if err != nil {
			return err
		}
		step, err := tx.ProtocolStep(ctx, experimentID, in.ProtocolVersionID, in.StepPosition)
		if err != nil {
			return err
		}
		scheduled := instant(in.ScheduledAt)
		if scheduled.IsZero() {
			scheduled = instant(timepoint.DefaultScheduledAt)
		}
		t, err := domain.NewTest(domain.Context{
			ExperimentID: experimentID, TimepointID: timepoint.ID, ProtocolVersionID: step.ProtocolVersionID, StepPosition: step.Position,
			ParadigmKey: step.ParadigmKey, ParadigmVersion: step.ParadigmVersion, PlannedTrials: step.Trials,
		}, scheduled, in.Notes, actor.UserID, in.Subjects)
		if err != nil {
			return err
		}
		if err := s.admit(ctx, tx, t, in.Subjects); err != nil {
			return err
		}
		created, err = tx.CreateTest(ctx, t)
		return err
	})
	if err != nil {
		return domain.Test{}, err
	}
	return created, nil
}

// AddSubject places another animal in a planned test.
func (s *Service) AddSubject(ctx context.Context, actor access.Actor, testID string, p domain.Participant) (domain.Test, error) {
	if err := actor.Require(access.TestWrite); err != nil {
		return domain.Test{}, err
	}
	var out domain.Test
	err := s.store.Transaction(ctx, func(tx Store) error {
		t, err := tx.LockTest(ctx, testID)
		if err != nil {
			return err
		}
		subject, err := t.AddSubject(p, actor.UserID)
		if err != nil {
			return err
		}
		if err := s.admit(ctx, tx, t, []domain.Participant{p}); err != nil {
			return err
		}
		if err := tx.AddTestSubject(ctx, t.ExperimentID, subject, t.ParadigmKey, t.ParadigmVersion); err != nil {
			return err
		}
		out, err = tx.Test(ctx, t.ID)
		return err
	})
	if err != nil {
		return domain.Test{}, err
	}
	return out, nil
}

// RemoveSubject takes an animal out of a planned test; the last one stays.
func (s *Service) RemoveSubject(ctx context.Context, actor access.Actor, testID, subjectID string) (domain.Test, error) {
	if err := actor.Require(access.TestWrite); err != nil {
		return domain.Test{}, err
	}
	var out domain.Test
	err := s.store.Transaction(ctx, func(tx Store) error {
		t, err := tx.LockTest(ctx, testID)
		if err != nil {
			return err
		}
		if _, ok := t.Subject(subjectID); !ok {
			return ErrTestSubjectNotFound
		}
		if err := t.CanRemoveSubject(); err != nil {
			return err
		}
		if err := tx.RemoveTestSubject(ctx, t.ID, subjectID); err != nil {
			return err
		}
		out, err = tx.Test(ctx, t.ID)
		return err
	})
	if err != nil {
		return domain.Test{}, err
	}
	return out, nil
}

// admit checks new participants of t: each arena runs the test's paradigm
// version, each animal is allocated in the experiment and not excluded, and
// none is already in another non-cancelled test of the same slot.
func (s *Service) admit(ctx context.Context, tx Store, t domain.Test, participants []domain.Participant) error {
	animals, arenas := make([]string, 0, len(participants)), make([]string, 0, len(participants))
	for _, p := range participants {
		animals, arenas = append(animals, p.AllocationID), append(arenas, p.EnvironmentRevisionID)
	}
	found, err := tx.Arenas(ctx, arenas)
	if err != nil {
		return err
	}
	for _, id := range arenas {
		i := slices.IndexFunc(found, func(a ArenaRef) bool { return a.ID == id })
		switch {
		case i < 0:
			return fmt.Errorf("%w: %s", ErrArenaNotFound, id)
		case found[i].ParadigmKey != t.ParadigmKey || found[i].ParadigmVersion != t.ParadigmVersion:
			return fmt.Errorf("%w: %s", ErrArenaParadigmMismatch, id)
		}
	}
	allocations, err := tx.LockAllocations(ctx, t.ExperimentID, animals)
	if err != nil {
		return err
	}
	for _, id := range animals {
		i := slices.IndexFunc(allocations, func(a AllocationRef) bool { return a.ID == id })
		switch {
		case i < 0:
			return fmt.Errorf("%w: %s", ErrAllocationNotFound, id)
		case allocations[i].Excluded:
			return fmt.Errorf("%w: %s", ErrAnimalExcluded, id)
		}
	}
	slot := Slot{TimepointID: t.TimepointID, ProtocolVersionID: t.ProtocolVersionID, StepPosition: t.StepPosition}
	taken, err := tx.ScheduledAllocations(ctx, slot, animals, t.ID)
	if err != nil {
		return err
	}
	if len(taken) > 0 {
		return fmt.Errorf("%w: %v", ErrAnimalAlreadyScheduled, taken)
	}
	return nil
}

// TestQuery filters the worklist. From and To are dates (YYYY-MM-DD); Status
// may list several statuses separated by commas; Order is asc or desc by
// scheduled time and defaults to the caller's natural order.
type TestQuery struct {
	ExperimentID, TimepointID, SubjectID, Status, ParadigmKey, From, To, Order, Search string
	Page, PageSize                                                                     int
}

type TestPage struct {
	Tests                 []domain.Test
	Total, Page, PageSize int
}

// ListTests lists an experiment's tests by scheduled time, earliest first.
func (s *Service) ListTests(ctx context.Context, actor access.Actor, experimentID string, q TestQuery) (TestPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return TestPage{}, err
	}
	q.ExperimentID = experimentID
	return s.list(ctx, q, false)
}

// ListAllTests lists tests across every experiment, newest scheduled first,
// so the panel can show one worklist of sessions.
func (s *Service) ListAllTests(ctx context.Context, actor access.Actor, q TestQuery) (TestPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return TestPage{}, err
	}
	return s.list(ctx, q, true)
}

func (s *Service) list(ctx context.Context, q TestQuery, newestFirst bool) (TestPage, error) {
	f := TestFilter{ExperimentID: q.ExperimentID, TimepointID: q.TimepointID, SubjectID: q.SubjectID, ParadigmKey: q.ParadigmKey, Search: strings.TrimSpace(q.Search), NewestFirst: newestFirst}
	// PostgreSQL text cannot hold NUL, so it would fail the query instead.
	if !utf8.ValidString(f.Search) || utf8.RuneCountInString(f.Search) > 100 || strings.ContainsRune(f.Search, 0) {
		return TestPage{}, ErrInvalidQuery
	}
	if q.Status != "" {
		for _, raw := range strings.Split(q.Status, ",") {
			status, err := domain.ParseStatus(strings.TrimSpace(raw))
			if err != nil {
				return TestPage{}, ErrInvalidQuery
			}
			f.Statuses = append(f.Statuses, status)
		}
	}
	switch q.Order {
	case "":
	case "asc":
		f.NewestFirst = false
	case "desc":
		f.NewestFirst = true
	default:
		return TestPage{}, ErrInvalidQuery
	}
	var from, to time.Time
	for _, d := range []struct {
		raw    string
		parsed *time.Time
		out    *string
	}{{q.From, &from, &f.From}, {q.To, &to, &f.To}} {
		if d.raw == "" {
			continue
		}
		parsed, err := time.Parse(time.DateOnly, d.raw)
		if err != nil {
			return TestPage{}, ErrInvalidQuery
		}
		*d.parsed, *d.out = parsed, parsed.Format(time.DateOnly)
	}
	if !from.IsZero() && !to.IsZero() && to.Before(from) {
		return TestPage{}, ErrInvalidQuery
	}
	page, limit, offset, err := paging(q.Page, q.PageSize)
	if err != nil {
		return TestPage{}, err
	}
	f.Limit, f.Offset = limit, offset
	tests, total, err := s.store.ListTests(ctx, f)
	if err != nil {
		return TestPage{}, err
	}
	return TestPage{Tests: tests, Total: total, Page: page, PageSize: limit}, nil
}

type TestDetail struct {
	Test   domain.Test
	Trials []domain.Trial
}

func (s *Service) Test(ctx context.Context, actor access.Actor, id string) (TestDetail, error) {
	if err := actor.Require(access.Read); err != nil {
		return TestDetail{}, err
	}
	t, err := s.store.Test(ctx, id)
	if err != nil {
		return TestDetail{}, err
	}
	trials, err := s.store.Trials(ctx, t.ID)
	if err != nil {
		return TestDetail{}, err
	}
	return TestDetail{Test: t, Trials: trials}, nil
}

// StartTest starts a planned test at at, or now when at is zero.
func (s *Service) StartTest(ctx context.Context, actor access.Actor, id string, at time.Time) (domain.Test, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Test{}, err
	}
	now := s.now()
	at = instant(orNow(at, now))
	var out domain.Test
	err := s.store.Transaction(ctx, func(tx Store) error {
		t, err := tx.LockTest(ctx, id)
		if err != nil {
			return err
		}
		started, err := t.Start(at, now)
		if err != nil {
			return err
		}
		out, err = tx.UpdateTestStatus(ctx, started, domain.Planned)
		return err
	})
	if err != nil {
		return domain.Test{}, err
	}
	return out, nil
}

// CompleteTest completes a test in progress at at, or now when at is zero.
func (s *Service) CompleteTest(ctx context.Context, actor access.Actor, id string, at time.Time) (domain.Test, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Test{}, err
	}
	now := s.now()
	at = instant(orNow(at, now))
	var out domain.Test
	err := s.store.Transaction(ctx, func(tx Store) error {
		t, err := tx.LockTest(ctx, id)
		if err != nil {
			return err
		}
		trials, err := tx.Trials(ctx, t.ID)
		if err != nil {
			return err
		}
		completed, err := t.Complete(at, now, trials)
		if err != nil {
			return err
		}
		out, err = tx.UpdateTestStatus(ctx, completed, domain.InProgress)
		return err
	})
	if err != nil {
		return domain.Test{}, err
	}
	return out, nil
}

// CancelTest cancels a planned test or one in progress. Its animals and
// trials are kept; the animals become free for another test of the slot.
func (s *Service) CancelTest(ctx context.Context, actor access.Actor, id, reason string) (domain.Test, error) {
	if err := actor.Require(access.TestWrite); err != nil {
		return domain.Test{}, err
	}
	now := instant(s.now())
	var out domain.Test
	err := s.store.Transaction(ctx, func(tx Store) error {
		t, err := tx.LockTest(ctx, id)
		if err != nil {
			return err
		}
		from := t.Status
		cancelled, err := t.Cancel(reason, now)
		if err != nil {
			return err
		}
		out, err = tx.UpdateTestStatus(ctx, cancelled, from)
		return err
	})
	if err != nil {
		return domain.Test{}, err
	}
	return out, nil
}

// Trials lists the trials of one animal of a test.
func (s *Service) Trials(ctx context.Context, actor access.Actor, testID, subjectID string) ([]domain.Trial, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	t, err := s.store.Test(ctx, testID)
	if err != nil {
		return nil, err
	}
	if _, ok := t.Subject(subjectID); !ok {
		return nil, ErrTestSubjectNotFound
	}
	trials, err := s.store.Trials(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(trials, func(tr domain.Trial) bool { return tr.TestSubjectID != subjectID }), nil
}

// RecordTrial appends a trial of one animal. The test is locked so trial
// numbers are assigned one at a time and a completed test gets no more trials.
func (s *Service) RecordTrial(ctx context.Context, actor access.Actor, testID, subjectID string, in domain.TrialInput) (domain.Trial, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Trial{}, err
	}
	now := s.now()
	in.StartedAt = instant(in.StartedAt)
	if in.EndedAt != nil {
		end := instant(*in.EndedAt)
		in.EndedAt = &end
	}
	var out domain.Trial
	err := s.store.Transaction(ctx, func(tx Store) error {
		t, err := tx.LockTest(ctx, testID)
		if err != nil {
			return err
		}
		if _, ok := t.Subject(subjectID); !ok {
			return ErrTestSubjectNotFound
		}
		earlier, err := tx.Trials(ctx, t.ID)
		if err != nil {
			return err
		}
		trial, err := domain.NextTrial(t, subjectID, earlier, in, actor.UserID, now)
		if err != nil {
			return err
		}
		out, err = tx.CreateTrial(ctx, trial)
		return err
	})
	if err != nil {
		return domain.Trial{}, err
	}
	return out, nil
}

func (s *Service) Comments(ctx context.Context, actor access.Actor, testID string) ([]domain.Comment, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	if _, err := s.store.Test(ctx, testID); err != nil {
		return nil, err
	}
	return s.store.Comments(ctx, testID)
}

// CreateComment adds a comment by the caller. Every role may comment.
func (s *Service) CreateComment(ctx context.Context, actor access.Actor, testID, body string) (domain.Comment, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Comment{}, err
	}
	if !s.limiter.Allow(actor.UserID) {
		return domain.Comment{}, ErrRateLimited
	}
	text, err := domain.NormalizeComment(body)
	if err != nil {
		return domain.Comment{}, err
	}
	return s.store.CreateComment(ctx, domain.Comment{TestID: testID, AuthorID: actor.UserID, Body: text})
}

func (s *Service) UpdateComment(ctx context.Context, actor access.Actor, testID, commentID, body string) (domain.Comment, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Comment{}, err
	}
	if !s.limiter.Allow(actor.UserID) {
		return domain.Comment{}, ErrRateLimited
	}
	text, err := domain.NormalizeComment(body)
	if err != nil {
		return domain.Comment{}, err
	}
	c, err := s.store.Comment(ctx, testID, commentID)
	if err != nil {
		return domain.Comment{}, err
	}
	if !c.EditableBy(actor) {
		return domain.Comment{}, ErrCannotEditComment
	}
	return s.store.UpdateComment(ctx, c.TestID, c.ID, text)
}

func (s *Service) DeleteComment(ctx context.Context, actor access.Actor, testID, commentID string) error {
	if err := actor.Require(access.Read); err != nil {
		return err
	}
	c, err := s.store.Comment(ctx, testID, commentID)
	if err != nil {
		return err
	}
	if !c.DeletableBy(actor) {
		return ErrCannotDeleteComment
	}
	return s.store.DeleteComment(ctx, c.TestID, c.ID)
}

func orNow(t, now time.Time) time.Time {
	if t.IsZero() {
		return now
	}
	return t
}

// instant stores times in UTC at the database's microsecond precision.
func instant(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC().Truncate(time.Microsecond)
}

// paging defaults to 20 rows per page and caps the page size at 100.
func paging(page, size int) (current, limit, offset int, err error) {
	if page < 0 || page > 1<<20 || size < 0 {
		return 0, 0, 0, ErrInvalidQuery
	}
	current, limit = max(page, 1), min(size, 100)
	if limit == 0 {
		limit = 20
	}
	return current, limit, (current - 1) * limit, nil
}
