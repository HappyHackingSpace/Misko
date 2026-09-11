// Package application contains test, trial and comment use cases. Creating and
// cancelling tests needs test:write; starting, completing and recording trials
// needs test:run; reads need *:read. Every authenticated role may comment; only
// the author edits and the author or a privileged role deletes.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/domain"
	"time"
)

var (
	ErrTestNotFound            = errors.New("test not found")
	ErrEnrollmentNotFound      = errors.New("enrollment not found in this experiment")
	ErrPhaseNotFound           = errors.New("phase not found in this experiment")
	ErrProtocolVersionNotFound = errors.New("protocol version not found in this experiment")
	ErrStepNotFound            = errors.New("step not found in this protocol version")
	ErrCommentNotFound         = errors.New("comment not found on this test")
	ErrCannotEditComment       = errors.New("only the author can edit a comment")
	ErrCannotDeleteComment     = errors.New("only the author or a lab manager can delete a comment")
	ErrRateLimited             = errors.New("too many comment changes; try again in a minute")
	ErrInvalidQuery            = errors.New("invalid list query")
)

// EnrollmentRef is the part of an enrollment a test needs.
type EnrollmentRef struct {
	ID, ExperimentID, SubjectID string
	EnrolledAt                  time.Time
}

// StepRef is the protocol step a test runs.
type StepRef struct {
	ProtocolVersionID     string
	Position              int
	ParadigmKey           string
	ParadigmVersion       int
	EnvironmentRevisionID string
	Trials                int
}

type TestFilter struct {
	ExperimentID, SubjectID, PhaseID, ParadigmKey string
	Status                                        domain.Status
	Limit, Offset                                 int
}

// Store persists tests. Scoped lookups return the matching not-found error for
// records of another experiment or test.
type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	Enrollment(ctx context.Context, experimentID, enrollmentID string) (EnrollmentRef, error)
	// LockEnrollment blocks group reassignments of the enrollment until the transaction ends.
	LockEnrollment(ctx context.Context, experimentID, enrollmentID string) (EnrollmentRef, error)
	Phase(ctx context.Context, experimentID, phaseID string) error
	// GroupAt returns the group assigned to the enrollment at t, or "".
	GroupAt(ctx context.Context, enrollmentID string, t time.Time) (string, error)
	ProtocolStep(ctx context.Context, experimentID, versionID string, position int) (StepRef, error)
	CreateTest(ctx context.Context, t domain.Test) (domain.Test, error)
	Test(ctx context.Context, id string) (domain.Test, error)
	// LockTest locks the test until the transaction ends.
	LockTest(ctx context.Context, id string) (domain.Test, error)
	// UpdateTestStatus stores t only while the stored status is still from.
	UpdateTestStatus(ctx context.Context, t domain.Test, from domain.Status) (domain.Test, error)
	ListTests(ctx context.Context, f TestFilter) ([]domain.Test, int, error)
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
	EnrollmentID, PhaseID, ProtocolVersionID string
	StepPosition                             int
	ScheduledAt                              time.Time
	Notes                                    string
}

// CreateTest plans a test for an enrollment on one protocol step. The subject,
// paradigm, environment revision and planned trials come from the experiment's
// records, and the group is the one assigned at the scheduled time.
func (s *Service) CreateTest(ctx context.Context, actor access.Actor, experimentID string, in TestInput) (domain.Test, error) {
	if err := actor.Require(access.TestWrite); err != nil {
		return domain.Test{}, err
	}
	if in.StepPosition < 1 {
		return domain.Test{}, ErrStepNotFound
	}
	scheduled := instant(in.ScheduledAt)
	var created domain.Test
	err := s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.Enrollment(ctx, experimentID, in.EnrollmentID)
		if err != nil {
			return err
		}
		if in.PhaseID != "" {
			if err := tx.Phase(ctx, e.ExperimentID, in.PhaseID); err != nil {
				return err
			}
		}
		step, err := tx.ProtocolStep(ctx, e.ExperimentID, in.ProtocolVersionID, in.StepPosition)
		if err != nil {
			return err
		}
		group := ""
		if !scheduled.IsZero() {
			if group, err = tx.GroupAt(ctx, e.ID, scheduled); err != nil {
				return err
			}
		}
		t, err := domain.NewTest(domain.Context{
			ExperimentID: e.ExperimentID, EnrollmentID: e.ID, SubjectID: e.SubjectID, EnrolledAt: e.EnrolledAt, PhaseID: in.PhaseID, GroupID: group,
			ProtocolVersionID: step.ProtocolVersionID, StepPosition: step.Position, ParadigmKey: step.ParadigmKey, ParadigmVersion: step.ParadigmVersion,
			EnvironmentRevisionID: step.EnvironmentRevisionID, PlannedTrials: step.Trials,
		}, scheduled, in.Notes, actor.UserID)
		if err != nil {
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

type TestQuery struct {
	SubjectID, PhaseID, Status, ParadigmKey string
	Page, PageSize                          int
}

type TestPage struct {
	Tests                 []domain.Test
	Total, Page, PageSize int
}

// ListTests lists an experiment's tests by scheduled time.
func (s *Service) ListTests(ctx context.Context, actor access.Actor, experimentID string, q TestQuery) (TestPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return TestPage{}, err
	}
	f := TestFilter{ExperimentID: experimentID, SubjectID: q.SubjectID, PhaseID: q.PhaseID, ParadigmKey: q.ParadigmKey}
	if q.Status != "" {
		status, err := domain.ParseStatus(q.Status)
		if err != nil {
			return TestPage{}, ErrInvalidQuery
		}
		f.Status = status
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

// StartTest starts a planned test at at, or now when at is zero. The enrollment
// is locked so the group cannot change while it is checked.
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
		e, err := tx.LockEnrollment(ctx, t.ExperimentID, t.EnrollmentID)
		if err != nil {
			return err
		}
		group, err := tx.GroupAt(ctx, e.ID, at)
		if err != nil {
			return err
		}
		started, err := t.Start(at, now, e.EnrolledAt, group)
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

// CancelTest cancels a planned test or one in progress. Its trials are kept.
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

func (s *Service) Trials(ctx context.Context, actor access.Actor, testID string) ([]domain.Trial, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	if _, err := s.store.Test(ctx, testID); err != nil {
		return nil, err
	}
	return s.store.Trials(ctx, testID)
}

// RecordTrial appends a trial. The test is locked so trial numbers are
// assigned one at a time and a completed test gets no more trials.
func (s *Service) RecordTrial(ctx context.Context, actor access.Actor, testID string, in domain.TrialInput) (domain.Trial, error) {
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
		earlier, err := tx.Trials(ctx, t.ID)
		if err != nil {
			return err
		}
		trial, err := domain.NextTrial(t, earlier, in, actor.UserID, now)
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
