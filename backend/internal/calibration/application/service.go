// Package application contains calibration use cases. An environment's
// calibration needs apparatus:write, a manual calibration of one recording
// needs test:run, and reading calibrations and the calibration status needs
// *:read.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
	"time"
)

var (
	ErrRecordingNotFound = errors.New("recording not found on this test")
	ErrRevisionNotFound  = errors.New("environment revision not found")
	ErrVideoNotVerified  = errors.New("the recording's video has not been verified")
)

// RecordingRef is what a calibration needs to know about a recording and its test.
type RecordingRef struct {
	ID, TestID, VideoStatus string
	ParadigmKey             string
	ParadigmVersion         int
	EnvironmentRevisionID   string
	// Apparatus is the environment revision's measurements.
	Apparatus map[string]float64
}

// RevisionRef is what a calibration needs to know about an environment revision.
type RevisionRef struct {
	ID, EnvironmentID string
	ParadigmKey       string
	ParadigmVersion   int
	Apparatus         map[string]float64
}

// Catalog reads calibration needs from the hardcoded paradigm definitions.
type Catalog interface {
	Requirement(key string, version int, apparatus map[string]float64) (domain.Requirement, error)
}

type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	Recording(ctx context.Context, testID, recordingID string) (RecordingRef, error)
	// LockRecording serializes calibrations of one recording.
	LockRecording(ctx context.Context, testID, recordingID string) (RecordingRef, error)
	Revision(ctx context.Context, environmentID string, number int) (RevisionRef, error)
	// LockRevision serializes the environment calibrations of one revision.
	LockRevision(ctx context.Context, environmentID string, number int) (RevisionRef, error)
	// Calibrations lists a recording's own calibrations oldest first.
	Calibrations(ctx context.Context, recordingID string) ([]domain.Calibration, error)
	// EnvironmentCalibrations lists a revision's default calibrations oldest first.
	EnvironmentCalibrations(ctx context.Context, revisionID string) ([]domain.Calibration, error)
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

// Calibrate stores a manual calibration, VALID or REJECTED, for a verified
// video. It overrides the environment's calibration for this recording only.
func (s *Service) Calibrate(ctx context.Context, actor access.Actor, testID, recordingID string, in domain.Input) (domain.Calibration, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Calibration{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	var out domain.Calibration
	err := s.store.Transaction(ctx, func(tx Store) error {
		rec, err := tx.LockRecording(ctx, testID, recordingID)
		if err != nil {
			return err
		}
		if rec.VideoStatus != "VERIFIED" {
			return ErrVideoNotVerified
		}
		req, err := s.catalog.Requirement(rec.ParadigmKey, rec.ParadigmVersion, rec.Apparatus)
		if err != nil {
			return err
		}
		chain, err := tx.Calibrations(ctx, rec.ID)
		if err != nil {
			return err
		}
		if err := domain.CanCorrect(chain, in.SupersedesID); err != nil {
			return err
		}
		if latest := domain.Latest(chain); latest != nil {
			if err := domain.SameSetup(*latest, in); err != nil {
				return err
			}
		}
		c, err := domain.New(in, req, actor.UserID, now)
		if err != nil {
			return err
		}
		c.RecordingID, c.EnvironmentRevisionID = rec.ID, rec.EnvironmentRevisionID
		out, err = tx.CreateCalibration(ctx, c)
		return err
	})
	if err != nil {
		return domain.Calibration{}, err
	}
	return out, nil
}

// CalibrateEnvironment stores the default calibration of an environment
// revision, VALID or REJECTED. Every recording of the revision uses it unless
// it has a calibration of its own. A correction supersedes the latest one and
// changes nothing for analysis runs that already pinned an earlier one.
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
		req, err := s.catalog.Requirement(rev.ParadigmKey, rev.ParadigmVersion, rev.Apparatus)
		if err != nil {
			return err
		}
		chain, err := tx.EnvironmentCalibrations(ctx, rev.ID)
		if err != nil {
			return err
		}
		if err := domain.CanCorrect(chain, in.SupersedesID); err != nil {
			return err
		}
		if latest := domain.Latest(chain); latest != nil {
			if err := domain.SameSetup(*latest, in); err != nil {
				return err
			}
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

// Calibrations lists the recording's own calibrations, not the environment's.
func (s *Service) Calibrations(ctx context.Context, actor access.Actor, testID, recordingID string) ([]domain.Calibration, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	rec, err := s.store.Recording(ctx, testID, recordingID)
	if err != nil {
		return nil, err
	}
	return s.store.Calibrations(ctx, rec.ID)
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
	Waiting     CalibrationStatus = "WAITING_FOR_CALIBRATION"
	Calibrated  CalibrationStatus = "CALIBRATED"
)

// Drift is what is known about camera or arena movement in a video. No check
// exists yet, so every calibrated recording is UNCHECKED; the field is part of
// the contract so clients are ready when a check is added.
type Drift string

const DriftUnchecked Drift = "UNCHECKED"

// StatusView is the calibration state of a recording. Source is empty when
// nothing has been calibrated; Drift is empty unless a calibration applies.
type StatusView struct {
	Status  CalibrationStatus
	Current *domain.Calibration
	Source  domain.Source
	Drift   Drift
}

// Status tells whether a recording can be analyzed into physical metrics.
func (s *Service) Status(ctx context.Context, actor access.Actor, testID, recordingID string) (StatusView, error) {
	if err := actor.Require(access.Read); err != nil {
		return StatusView{}, err
	}
	rec, err := s.store.Recording(ctx, testID, recordingID)
	if err != nil {
		return StatusView{}, err
	}
	req, err := s.catalog.Requirement(rec.ParadigmKey, rec.ParadigmVersion, rec.Apparatus)
	if err != nil {
		return StatusView{}, err
	}
	if !req.Required {
		return StatusView{Status: NotRequired}, nil
	}
	if rec.VideoStatus != "VERIFIED" {
		return StatusView{Status: Waiting}, nil
	}
	own, err := s.store.Calibrations(ctx, rec.ID)
	if err != nil {
		return StatusView{}, err
	}
	inherited, err := s.store.EnvironmentCalibrations(ctx, rec.EnvironmentRevisionID)
	if err != nil {
		return StatusView{}, err
	}
	return view(domain.Effective(own, inherited)), nil
}

// EnvironmentStatus tells whether recordings of the revision are calibrated by
// default.
func (s *Service) EnvironmentStatus(ctx context.Context, actor access.Actor, environmentID string, number int) (StatusView, error) {
	if err := actor.Require(access.Read); err != nil {
		return StatusView{}, err
	}
	rev, err := s.store.Revision(ctx, environmentID, number)
	if err != nil {
		return StatusView{}, err
	}
	req, err := s.catalog.Requirement(rev.ParadigmKey, rev.ParadigmVersion, rev.Apparatus)
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
	return view(domain.Effective(nil, chain)), nil
}

func view(r domain.Resolution) StatusView {
	if r.Calibration == nil {
		return StatusView{Status: Waiting, Source: r.Source}
	}
	return StatusView{Status: Calibrated, Current: r.Calibration, Source: r.Source, Drift: DriftUnchecked}
}
