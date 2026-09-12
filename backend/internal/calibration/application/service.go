// Package application contains calibration use cases. Calibrating needs
// test:run; reading calibrations and the calibration status needs *:read.
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
	ErrVideoNotVerified  = errors.New("the recording's video has not been verified")
)

// RecordingRef is what a calibration needs to know about a recording and its test.
type RecordingRef struct {
	ID, TestID, VideoStatus string
	ParadigmKey             string
	ParadigmVersion         int
	// Apparatus is the environment revision's measurements.
	Apparatus map[string]float64
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
	// Calibrations lists a recording's calibrations oldest first.
	Calibrations(ctx context.Context, recordingID string) ([]domain.Calibration, error)
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

// Calibrate stores a calibration, VALID or REJECTED, for a verified video.
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
		c.RecordingID = rec.ID
		out, err = tx.CreateCalibration(ctx, c)
		return err
	})
	if err != nil {
		return domain.Calibration{}, err
	}
	return out, nil
}

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

type CalibrationStatus string

const (
	NotRequired CalibrationStatus = "NOT_REQUIRED"
	Waiting     CalibrationStatus = "WAITING_FOR_CALIBRATION"
	Calibrated  CalibrationStatus = "CALIBRATED"
)

type StatusView struct {
	Status  CalibrationStatus
	Current *domain.Calibration
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
	chain, err := s.store.Calibrations(ctx, rec.ID)
	if err != nil {
		return StatusView{}, err
	}
	if current := domain.Current(chain); current != nil {
		return StatusView{Status: Calibrated, Current: current}, nil
	}
	return StatusView{Status: Waiting}, nil
}
