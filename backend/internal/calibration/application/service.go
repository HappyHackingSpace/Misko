// Package application contains calibration use cases. An environment's
// calibration needs apparatus:write, a manual override for one animal of a
// test needs test:run, and reading calibrations and the calibration status
// needs *:read.
//
// Which calibration an animal of a test is analysed with comes from the
// effective_calibrations view, so the status shown here, the analysis
// scheduler and the dashboard read the same rule.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	"time"
)

var (
	ErrTestNotFound        = errors.New("test not found")
	ErrTestSubjectNotFound = errors.New("animal not found in this test")
	ErrRevisionNotFound    = errors.New("environment revision not found")
	ErrNoVideo             = errors.New("the test has no video yet")
	ErrVideoNotVerified    = errors.New("the test's current video has not been verified")
	ErrInvalidScope        = errors.New("an override must use the arena of its animal and a video of its test")
)

// VideoVerified is the status of a verified video asset.
const VideoVerified = "VERIFIED"

// TestSubjectRef is what a calibration needs to know about an animal of a test.
type TestSubjectRef struct {
	ID, TestID            string
	EnvironmentRevisionID string
	ParadigmKey           string
	ParadigmVersion       int
	// Parameters are the arena's environment revision values.
	Parameters map[string]float64
}

// VideoRef is the current video of a test.
type VideoRef struct {
	RecordingID, Status string
}

// RevisionRef is what a calibration needs to know about an environment revision.
type RevisionRef struct {
	ID, EnvironmentID string
	ParadigmKey       string
	ParadigmVersion   int
	Parameters        map[string]float64
}

// Effective is one row of the effective_calibrations view joined with what
// deciding the status needs. RecordingID and VideoStatus are empty while the
// test has no video; Source and CalibrationID as the view reports them.
type Effective struct {
	Subject       TestSubjectRef
	RecordingID   string
	VideoStatus   string
	Source        domain.Source
	CalibrationID string
}

// Catalog reads calibration needs from the hardcoded paradigm definitions.
type Catalog interface {
	Requirement(key string, version int, parameters map[string]float64) (domain.Requirement, error)
}

type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	TestExists(ctx context.Context, testID string) (bool, error)
	TestSubject(ctx context.Context, testID, testSubjectID string) (TestSubjectRef, error)
	// LockTestSubject serializes the overrides of one animal of a test.
	LockTestSubject(ctx context.Context, testID, testSubjectID string) (TestSubjectRef, error)
	// CurrentVideo returns the test's current video or ErrNoVideo.
	CurrentVideo(ctx context.Context, testID string) (VideoRef, error)
	Revision(ctx context.Context, environmentID string, number int) (RevisionRef, error)
	// LockRevision serializes the environment calibrations of one revision.
	LockRevision(ctx context.Context, environmentID string, number int) (RevisionRef, error)
	// Overrides lists a test subject's calibrations over all its videos, oldest first.
	Overrides(ctx context.Context, testSubjectID string) ([]domain.Calibration, error)
	// EnvironmentCalibrations lists a revision's default calibrations oldest first.
	EnvironmentCalibrations(ctx context.Context, revisionID string) ([]domain.Calibration, error)
	Calibration(ctx context.Context, id string) (domain.Calibration, error)
	// Effective lists the effective calibration of every animal of a test.
	Effective(ctx context.Context, testID string) ([]Effective, error)
	CreateCalibration(ctx context.Context, c domain.Calibration) (domain.Calibration, error)
}

type Service struct {
	store   Store
	catalog Catalog
	now     func() time.Time
}

func New(store Store, catalog Catalog, now func() time.Time) *Service {
	return &Service{store: store, catalog: catalog, now: now}
}

// Calibrate stores a manual calibration, VALID or REJECTED, of one animal of a
// test on the test's current, verified video. It overrides the default of the
// animal's arena for that video only; once a replacement video is attached the
// arena default applies again until the animal is calibrated on the new one.
func (s *Service) Calibrate(ctx context.Context, actor access.Actor, testID, testSubjectID string, in domain.Input) (domain.Calibration, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Calibration{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	var out domain.Calibration
	err := s.store.Transaction(ctx, func(tx Store) error {
		subject, err := tx.LockTestSubject(ctx, testID, testSubjectID)
		if err != nil {
			return err
		}
		video, err := tx.CurrentVideo(ctx, subject.TestID)
		if err != nil {
			return err
		}
		if video.Status != VideoVerified {
			return ErrVideoNotVerified
		}
		req, err := s.catalog.Requirement(subject.ParadigmKey, subject.ParadigmVersion, subject.Parameters)
		if err != nil {
			return err
		}
		chain, err := tx.Overrides(ctx, subject.ID)
		if err != nil {
			return err
		}
		if err := domain.CheckCorrection(chain, in, video.RecordingID); err != nil {
			return err
		}
		c, err := domain.New(in, req, actor.UserID, now)
		if err != nil {
			return err
		}
		c.TestSubjectID, c.RecordingID, c.EnvironmentRevisionID = subject.ID, video.RecordingID, subject.EnvironmentRevisionID
		out, err = tx.CreateCalibration(ctx, c)
		return err
	})
	if err != nil {
		return domain.Calibration{}, err
	}
	return out, nil
}

// CalibrateEnvironment stores the default calibration of an environment
// revision, VALID or REJECTED. Every animal tested in that arena uses it unless
// it has an override on its test's current video. A correction supersedes the
// latest one and changes nothing for analysis runs that already pinned an
// earlier one.
func (s *Service) CalibrateEnvironment(ctx context.Context, actor access.Actor, environmentID string, number int, in domain.Input) (domain.Calibration, error) {
	if err := actor.Require(access.ApparatusWrite); err != nil {
		return domain.Calibration{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	var out domain.Calibration
	err := s.store.Transaction(ctx, func(tx Store) error {
		rev, err := tx.LockRevision(ctx, environmentID, number)
		if err != nil {
			return err
		}
		req, err := s.catalog.Requirement(rev.ParadigmKey, rev.ParadigmVersion, rev.Parameters)
		if err != nil {
			return err
		}
		chain, err := tx.EnvironmentCalibrations(ctx, rev.ID)
		if err != nil {
			return err
		}
		if err := domain.CheckCorrection(chain, in, ""); err != nil {
			return err
		}
		in.ReferenceFrameUs = 0
		c, err := domain.New(in, req, actor.UserID, now)
		if err != nil {
			return err
		}
		c.EnvironmentRevisionID = rev.ID
		out, err = tx.CreateCalibration(ctx, c)
		return err
	})
	if err != nil {
		return domain.Calibration{}, err
	}
	return out, nil
}

// Overrides lists the animal's own calibrations over every video of its test,
// not its arena's default.
func (s *Service) Overrides(ctx context.Context, actor access.Actor, testID, testSubjectID string) ([]domain.Calibration, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	subject, err := s.store.TestSubject(ctx, testID, testSubjectID)
	if err != nil {
		return nil, err
	}
	return s.store.Overrides(ctx, subject.ID)
}

// EnvironmentCalibrations lists the default calibrations of one revision.
func (s *Service) EnvironmentCalibrations(ctx context.Context, actor access.Actor, environmentID string, number int) ([]domain.Calibration, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	rev, err := s.store.Revision(ctx, environmentID, number)
	if err != nil {
		return nil, err
	}
	return s.store.EnvironmentCalibrations(ctx, rev.ID)
}

type CalibrationStatus string

const (
	NotRequired CalibrationStatus = "NOT_REQUIRED"
	// WaitingForVideo means the test has no current verified video to analyse.
	WaitingForVideo CalibrationStatus = "WAITING_FOR_VIDEO"
	Waiting         CalibrationStatus = "WAITING_FOR_CALIBRATION"
	Calibrated      CalibrationStatus = "CALIBRATED"
)

// Drift is what is known about camera or arena movement in a video. No check
// exists yet, so every calibration in use is UNCHECKED; the field is part of
// the contract so clients are ready when a check is added.
type Drift string

const DriftUnchecked Drift = "UNCHECKED"

// StatusView is a calibration state. Source is empty when nothing has been
// calibrated; Drift is empty unless a calibration applies.
type StatusView struct {
	Status  CalibrationStatus
	Current *domain.Calibration
	Source  domain.Source
	Drift   Drift
}

// SubjectStatus is the calibration state of one animal of a test on the
// test's current video. RecordingID is empty while the test has no video.
type SubjectStatus struct {
	TestSubjectID         string
	EnvironmentRevisionID string
	RecordingID           string
	StatusView
}

// TestStatus tells, for every animal of a test, whether its part of the
// current video can be analysed into physical metrics.
func (s *Service) TestStatus(ctx context.Context, actor access.Actor, testID string) ([]SubjectStatus, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	exists, err := s.store.TestExists(ctx, testID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrTestNotFound
	}
	rows, err := s.store.Effective(ctx, testID)
	if err != nil {
		return nil, err
	}
	out := make([]SubjectStatus, 0, len(rows))
	for _, row := range rows {
		status, err := s.subjectStatus(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, status)
	}
	return out, nil
}

// SubjectStatus is TestStatus for one animal.
func (s *Service) SubjectStatus(ctx context.Context, actor access.Actor, testID, testSubjectID string) (SubjectStatus, error) {
	if err := actor.Require(access.Read); err != nil {
		return SubjectStatus{}, err
	}
	subject, err := s.store.TestSubject(ctx, testID, testSubjectID)
	if err != nil {
		return SubjectStatus{}, err
	}
	rows, err := s.store.Effective(ctx, subject.TestID)
	if err != nil {
		return SubjectStatus{}, err
	}
	for _, row := range rows {
		if row.Subject.ID == subject.ID {
			return s.subjectStatus(ctx, row)
		}
	}
	return SubjectStatus{}, ErrTestSubjectNotFound
}

func (s *Service) subjectStatus(ctx context.Context, row Effective) (SubjectStatus, error) {
	out := SubjectStatus{TestSubjectID: row.Subject.ID, EnvironmentRevisionID: row.Subject.EnvironmentRevisionID, RecordingID: row.RecordingID}
	req, err := s.catalog.Requirement(row.Subject.ParadigmKey, row.Subject.ParadigmVersion, row.Subject.Parameters)
	if err != nil {
		return SubjectStatus{}, err
	}
	switch {
	case !req.Required:
		out.Status = NotRequired
	case row.RecordingID == "" || row.VideoStatus != VideoVerified:
		out.Status = WaitingForVideo
	case row.CalibrationID == "":
		out.Status, out.Source = Waiting, row.Source
	default:
		c, err := s.store.Calibration(ctx, row.CalibrationID)
		if err != nil {
			return SubjectStatus{}, err
		}
		out.StatusView = StatusView{Status: Calibrated, Current: &c, Source: row.Source, Drift: DriftUnchecked}
	}
	return out, nil
}

// EnvironmentStatus tells whether animals tested in the revision's arena are
// calibrated by default.
func (s *Service) EnvironmentStatus(ctx context.Context, actor access.Actor, environmentID string, number int) (StatusView, error) {
	if err := actor.Require(access.Read); err != nil {
		return StatusView{}, err
	}
	rev, err := s.store.Revision(ctx, environmentID, number)
	if err != nil {
		return StatusView{}, err
	}
	req, err := s.catalog.Requirement(rev.ParadigmKey, rev.ParadigmVersion, rev.Parameters)
	if err != nil {
		return StatusView{}, err
	}
	if !req.Required {
		return StatusView{Status: NotRequired}, nil
	}
	chain, err := s.store.EnvironmentCalibrations(ctx, rev.ID)
	if err != nil {
		return StatusView{}, err
	}
	r := domain.Resolve(chain, domain.SourceEnvironment)
	if r.Calibration == nil {
		return StatusView{Status: Waiting, Source: r.Source}, nil
	}
	return StatusView{Status: Calibrated, Current: r.Calibration, Source: r.Source, Drift: DriftUnchecked}, nil
}
