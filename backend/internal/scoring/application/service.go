// Package application contains observer scoring use cases. Reading a scoring
// sheet needs *:read; marking events, completing and reopening a sheet need
// test:run. Completing a sheet computes the paradigm's metrics with the engine
// and publishes them as a SCORED analysis run whose result opens for review
// like any analysed result.
package application

import (
	"context"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/domain"
	"slices"
	"time"
)

var (
	ErrTestSubjectNotFound = errors.New("test subject not found in this test")
	ErrEventNotFound       = errors.New("scored event not found on the current scoring sheet")
	ErrNoVideo             = errors.New("the test has no verified current video to score")
	ErrNotScorable         = errors.New("scoring needs a test in progress or completed, a running experiment and an animal that is not excluded")
	ErrRejectedByEngine    = errors.New("the events break an ordering rule of the paradigm")
	ErrConflict            = errors.New("the scoring sheet was changed concurrently; reload it")
)

// Subject is one animal of a test with everything that decides whether it can
// be scored now.
type Subject struct {
	ExperimentID     string
	TestID           string
	TestSubjectID    string
	ExperimentStatus string
	TestStatus       string
	Excluded         bool
	ParadigmKey      string
	ParadigmVersion  int
	// Parameters are the arena's values over the protocol step's session values.
	Parameters map[string]float64
	// Video is nil while the test has no verified current video.
	Video *domain.Video
	// Invalid is set when stored inputs cannot be read, such as arena
	// parameters that are not numbers.
	Invalid error
}

// Catalog reads paradigm versions and runs the metric engine.
type Catalog interface {
	Paradigm(key string, version int) (domain.Paradigm, error)
	// Evaluate computes metrics and events of a scoring; ordering rules the
	// engine rejects are ErrRejectedByEngine.
	Evaluate(p domain.Paradigm, parameters map[string]float64, s domain.Scoring) (domain.Computed, error)
}

type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	// LockSubject serializes scoring of one animal until the transaction
	// ends; it returns ErrTestSubjectNotFound for a malformed id.
	LockSubject(ctx context.Context, testSubjectID string) error
	// Subject returns ErrTestSubjectNotFound unless the animal belongs to the test.
	Subject(ctx context.Context, testID, testSubjectID string) (Subject, error)
	// LatestSheet returns the newest sheet version, or nil before the first.
	LatestSheet(ctx context.Context, testSubjectID string) (*domain.Sheet, error)
	CreateSheet(ctx context.Context, s domain.Sheet) (domain.Sheet, error)
	CompleteSheet(ctx context.Context, s domain.Sheet) (domain.Sheet, error)
	Observations(ctx context.Context, sheetID string) ([]domain.Observation, error)
	CreateObservation(ctx context.Context, o domain.Observation) (domain.Observation, error)
	UpdateObservation(ctx context.Context, o domain.Observation) (domain.Observation, error)
	DeleteObservation(ctx context.Context, sheetID, id string) error
	// CopyObservations copies the events of one sheet onto another, keeping
	// who scored each one.
	CopyObservations(ctx context.Context, from, to domain.Sheet) error
	// CreateRun stores a SUCCEEDED scored run with its metrics and events in
	// the caller's transaction and returns its id.
	CreateRun(ctx context.Context, r domain.Run) (string, error)
	// OpenResult creates the PENDING result of the animal unless it has one.
	OpenResult(ctx context.Context, testSubjectID, runID string) error
}

type Service struct {
	store   Store
	catalog Catalog
	now     func() time.Time
}

func New(store Store, catalog Catalog, now func() time.Time) *Service {
	return &Service{store: store, catalog: catalog, now: now}
}

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// View is the scoring of one animal: the paradigm's input events, the current
// video and the newest sheet with its events (nil before the first event).
type View struct {
	Subject  Subject
	Paradigm domain.Paradigm
	Sheet    *domain.Sheet
	Events   []domain.Observation
}

// Completion is a completed sheet and the SCORED run computed from it.
type Completion struct {
	View
	RunID    string
	Computed domain.Computed
}

func (s *Service) Read(ctx context.Context, actor access.Actor, testID, testSubjectID string) (View, error) {
	if err := actor.Require(access.Read); err != nil {
		return View{}, err
	}
	subject, err := s.store.Subject(ctx, testID, testSubjectID)
	if err != nil {
		return View{}, err
	}
	p, err := s.paradigm(subject)
	if err != nil {
		return View{}, err
	}
	view := View{Subject: subject, Paradigm: p, Events: []domain.Observation{}}
	if view.Sheet, err = s.store.LatestSheet(ctx, testSubjectID); err != nil || view.Sheet == nil {
		return view, err
	}
	view.Events, err = s.store.Observations(ctx, view.Sheet.ID)
	return view, err
}

// AddEvent marks an event on the draft sheet, starting the first sheet when
// the animal has none.
func (s *Service) AddEvent(ctx context.Context, actor access.Actor, testID, testSubjectID string, in domain.EventInput) (domain.Observation, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Observation{}, err
	}
	var out domain.Observation
	err := s.draft(ctx, actor, testID, testSubjectID, func(tx Store, subject Subject, p domain.Paradigm, sheet domain.Sheet, events []domain.Observation) error {
		o, err := domain.NewObservation(p, subject.Video.Clip, in)
		if err != nil {
			return err
		}
		if err := domain.CheckCount(p, events, o); err != nil {
			return err
		}
		o.SheetID, o.TestSubjectID, o.ScoredBy = sheet.ID, sheet.TestSubjectID, actor.UserID
		out, err = tx.CreateObservation(ctx, o)
		return err
	})
	return out, err
}

// ChangeEvent replaces an event of the draft sheet.
func (s *Service) ChangeEvent(ctx context.Context, actor access.Actor, testID, testSubjectID, eventID string, in domain.EventInput) (domain.Observation, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Observation{}, err
	}
	var out domain.Observation
	err := s.draft(ctx, actor, testID, testSubjectID, func(tx Store, subject Subject, p domain.Paradigm, sheet domain.Sheet, events []domain.Observation) error {
		i := slices.IndexFunc(events, func(o domain.Observation) bool { return o.ID == eventID })
		if i < 0 {
			return ErrEventNotFound
		}
		o, err := domain.NewObservation(p, subject.Video.Clip, in)
		if err != nil {
			return err
		}
		if err := domain.CheckCount(p, slices.Delete(slices.Clone(events), i, i+1), o); err != nil {
			return err
		}
		o.ID, o.SheetID, o.TestSubjectID, o.ScoredBy = eventID, sheet.ID, sheet.TestSubjectID, actor.UserID
		out, err = tx.UpdateObservation(ctx, o)
		return err
	})
	return out, err
}

func (s *Service) RemoveEvent(ctx context.Context, actor access.Actor, testID, testSubjectID, eventID string) error {
	if err := actor.Require(access.TestRun); err != nil {
		return err
	}
	return s.draft(ctx, actor, testID, testSubjectID, func(tx Store, _ Subject, _ domain.Paradigm, sheet domain.Sheet, events []domain.Observation) error {
		if !slices.ContainsFunc(events, func(o domain.Observation) bool { return o.ID == eventID }) {
			return ErrEventNotFound
		}
		return tx.DeleteObservation(ctx, sheet.ID, eventID)
	})
}

// Complete fixes the scored event types and recording length of the draft,
// computes the metrics with the engine on the current video, publishes them as
// a SUCCEEDED SCORED run and opens the animal's result for review when it has
// none. Completing without a sheet scores an animal without events (for
// example a treadmill trial without exhaustion).
func (s *Service) Complete(ctx context.Context, actor access.Actor, testID, testSubjectID string, in domain.CompleteInput) (Completion, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return Completion{}, err
	}
	var out Completion
	err := s.draft(ctx, actor, testID, testSubjectID, func(tx Store, subject Subject, p domain.Paradigm, sheet domain.Sheet, events []domain.Observation) error {
		scoring, err := domain.Prepare(p, subject.Video.Clip, events, in)
		if err != nil {
			return err
		}
		computed, err := s.catalog.Evaluate(p, subject.Parameters, scoring)
		if err != nil {
			return err
		}
		now := s.clock()
		run := domain.Run{
			ExperimentID: subject.ExperimentID, TestID: subject.TestID, TestSubjectID: subject.TestSubjectID, Video: *subject.Video,
			ParadigmKey: p.Key, ParadigmVersion: p.Version, Parameters: subject.Parameters, Computed: computed, ScoredBy: actor.UserID, FinishedAt: now,
		}
		runID, err := tx.CreateRun(ctx, run)
		if err != nil {
			return err
		}
		done, err := sheet.Complete(scoring, subject.Video.RecordingID, runID, actor.UserID, now)
		if err != nil {
			return err
		}
		if done, err = tx.CompleteSheet(ctx, done); err != nil {
			return err
		}
		if err := tx.OpenResult(ctx, subject.TestSubjectID, runID); err != nil {
			return err
		}
		out = Completion{View: View{Subject: subject, Paradigm: p, Sheet: &done, Events: events}, RunID: runID, Computed: computed}
		return nil
	})
	return out, err
}

// Reopen starts the next sheet version after a completed one, with a copy of
// its events. The completed sheet, its run and the reviewed result stay.
func (s *Service) Reopen(ctx context.Context, actor access.Actor, testID, testSubjectID string) (View, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return View{}, err
	}
	var out View
	err := s.store.Transaction(ctx, func(tx Store) error {
		subject, p, err := s.writable(ctx, tx, testID, testSubjectID)
		if err != nil {
			return err
		}
		latest, err := tx.LatestSheet(ctx, testSubjectID)
		if err != nil {
			return err
		}
		if latest == nil {
			return domain.ErrNotCompleted
		}
		next, err := domain.Next(latest, subject.ExperimentID, subject.TestID, subject.TestSubjectID, actor.UserID)
		if err != nil {
			return err
		}
		created, err := tx.CreateSheet(ctx, next)
		if err != nil {
			return err
		}
		if err := tx.CopyObservations(ctx, *latest, created); err != nil {
			return err
		}
		events, err := tx.Observations(ctx, created.ID)
		if err != nil {
			return err
		}
		out = View{Subject: subject, Paradigm: p, Sheet: &created, Events: events}
		return nil
	})
	return out, err
}

// draft runs fn on the animal's draft sheet and its events in one
// transaction, starting the first sheet when the animal has none.
func (s *Service) draft(ctx context.Context, actor access.Actor, testID, testSubjectID string,
	fn func(tx Store, subject Subject, p domain.Paradigm, sheet domain.Sheet, events []domain.Observation) error) error {
	return s.store.Transaction(ctx, func(tx Store) error {
		subject, p, err := s.writable(ctx, tx, testID, testSubjectID)
		if err != nil {
			return err
		}
		latest, err := tx.LatestSheet(ctx, testSubjectID)
		if err != nil {
			return err
		}
		var sheet domain.Sheet
		switch {
		case latest == nil:
			first, _ := domain.Next(nil, subject.ExperimentID, subject.TestID, subject.TestSubjectID, actor.UserID)
			if sheet, err = tx.CreateSheet(ctx, first); err != nil {
				return err
			}
		case latest.Status != domain.Draft:
			return domain.ErrNotDraft
		default:
			sheet = *latest
		}
		events, err := tx.Observations(ctx, sheet.ID)
		if err != nil {
			return err
		}
		return fn(tx, subject, p, sheet, events)
	})
}

// writable locks the animal's scoring and checks that it can be scored now.
func (s *Service) writable(ctx context.Context, tx Store, testID, testSubjectID string) (Subject, domain.Paradigm, error) {
	if err := tx.LockSubject(ctx, testSubjectID); err != nil {
		return Subject{}, domain.Paradigm{}, err
	}
	subject, err := tx.Subject(ctx, testID, testSubjectID)
	if err != nil {
		return Subject{}, domain.Paradigm{}, err
	}
	p, err := s.paradigm(subject)
	if err != nil {
		return Subject{}, domain.Paradigm{}, err
	}
	if subject.Invalid != nil {
		return Subject{}, domain.Paradigm{}, fmt.Errorf("test subject %s: %w", testSubjectID, subject.Invalid)
	}
	started := subject.TestStatus == "IN_PROGRESS" || subject.TestStatus == "COMPLETED"
	if !started || subject.ExperimentStatus != "RUNNING" || subject.Excluded {
		return Subject{}, domain.Paradigm{}, ErrNotScorable
	}
	if subject.Video == nil {
		return Subject{}, domain.Paradigm{}, ErrNoVideo
	}
	return subject, p, nil
}

func (s *Service) paradigm(subject Subject) (domain.Paradigm, error) {
	p, err := s.catalog.Paradigm(subject.ParadigmKey, subject.ParadigmVersion)
	if err != nil {
		return domain.Paradigm{}, err
	}
	if !p.ObserverScored {
		return domain.Paradigm{}, domain.ErrNotObserverScored
	}
	return p, nil
}
