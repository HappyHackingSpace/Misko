package application

import (
	"context"
	"errors"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
)

var (
	ErrTrialNotFound       = errors.New("paradigm trial not found")
	ErrTrialNoVideo        = errors.New("the trial has no video yet")
	ErrCalibrationNotFound = errors.New("calibration not found")
)

// TrialRef is what a calibration needs to know about a paradigm trial.
// VideoAssetID and VideoStatus are empty while the trial has no video.
type TrialRef struct {
	ID                    string
	EnvironmentRevisionID string
	ParadigmKey           string
	ParadigmVersion       int
	// Parameters are the arena's environment revision values.
	Parameters   map[string]float64
	VideoAssetID string
	VideoStatus  string
}

// TrialEffective is the trial_effective_calibrations row of a trial.
type TrialEffective struct {
	Source        domain.Source
	CalibrationID string
}

// CalibrateTrial stores a calibration, VALID or REJECTED, of a paradigm trial
// on the trial's current, verified video. It applies to that trial and that
// video only and never changes the environment's default.
func (s *Service) CalibrateTrial(ctx context.Context, actor access.Actor, trialID string, in domain.Input) (domain.Calibration, error) {
	if err := actor.Require(access.TrialRun); err != nil {
		return domain.Calibration{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	var out domain.Calibration
	err := s.store.Transaction(ctx, func(tx Store) error {
		trial, err := tx.LockTrial(ctx, trialID)
		if err != nil {
			return err
		}
		if err := trialVideoReady(trial); err != nil {
			return err
		}
		req, err := s.catalog.Requirement(trial.ParadigmKey, trial.ParadigmVersion, trial.Parameters)
		if err != nil {
			return err
		}
		chain, err := tx.TrialCalibrations(ctx, trial.ID)
		if err != nil {
			return err
		}
		if err := domain.CheckCorrection(chain, in, trial.VideoAssetID); err != nil {
			return err
		}
		c, err := domain.New(in, req, actor.UserID, now)
		if err != nil {
			return err
		}
		c.TrialID, c.VideoAssetID, c.EnvironmentRevisionID = trial.ID, trial.VideoAssetID, trial.EnvironmentRevisionID
		out, err = tx.CreateCalibration(ctx, c)
		return err
	})
	if err != nil {
		return domain.Calibration{}, err
	}
	return out, nil
}

func trialVideoReady(trial TrialRef) error {
	switch {
	case trial.VideoAssetID == "":
		return ErrTrialNoVideo
	case trial.VideoStatus != VideoVerified:
		return ErrVideoNotVerified
	}
	return nil
}

// TrialCalibrations lists the trial's own calibrations over all its videos.
func (s *Service) TrialCalibrations(ctx context.Context, actor access.Actor, trialID string) ([]domain.Calibration, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	trial, err := s.store.Trial(ctx, trialID)
	if err != nil {
		return nil, err
	}
	return s.store.TrialCalibrations(ctx, trial.ID)
}

// TrialStatus tells whether the trial's current video can be analysed into
// physical metrics, and with which calibration: the trial's own on this
// video, else the default of its arena.
func (s *Service) TrialStatus(ctx context.Context, actor access.Actor, trialID string) (StatusView, error) {
	if err := actor.Require(access.Read); err != nil {
		return StatusView{}, err
	}
	trial, err := s.store.Trial(ctx, trialID)
	if err != nil {
		return StatusView{}, err
	}
	req, err := s.catalog.Requirement(trial.ParadigmKey, trial.ParadigmVersion, trial.Parameters)
	if err != nil {
		return StatusView{}, err
	}
	effective, err := s.store.TrialEffective(ctx, trial.ID)
	if err != nil {
		return StatusView{}, err
	}
	switch {
	case !req.Required:
		return StatusView{Status: NotRequired}, nil
	case trialVideoReady(trial) != nil:
		return StatusView{Status: WaitingForVideo}, nil
	case effective.CalibrationID == "":
		return StatusView{Status: Waiting, Source: effective.Source}, nil
	}
	c, err := s.store.Calibration(ctx, effective.CalibrationID)
	if err != nil {
		return StatusView{}, err
	}
	return resolved(c, req, effective.Source), nil
}

// The calibration port. Other modules use it in process and authorize their
// own users.

// Snapshot returns a calibration for an analysis to copy, or
// ErrCalibrationNotFound.
func (s *Service) Snapshot(ctx context.Context, id string) (domain.Calibration, error) {
	return s.store.Calibration(ctx, id)
}

// TrialCalibration returns the id of the valid calibration a trial is
// analysed with now, or "" while it waits for one.
func (s *Service) TrialCalibration(ctx context.Context, trialID string) (string, error) {
	effective, err := s.store.TrialEffective(ctx, trialID)
	if err != nil {
		return "", err
	}
	return effective.CalibrationID, nil
}
