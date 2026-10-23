package application

import (
	"context"
	"errors"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
)

// Design records change only while the experiment is a draft: every write
// locks the experiment first and fails with ErrDesignFrozen otherwise.

func (s *Service) Groups(ctx context.Context, actor access.Actor, experimentID string) ([]GroupFill, error) {
	if err := s.readExperiment(ctx, actor, experimentID); err != nil {
		return nil, err
	}
	return s.store.Groups(ctx, experimentID)
}

func (s *Service) CreateGroup(ctx context.Context, actor access.Actor, experimentID string, in domain.GroupFields) (domain.Group, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Group{}, err
	}
	g, err := domain.NewGroup(experimentID, in)
	if err != nil {
		return domain.Group{}, err
	}
	var created domain.Group
	err = s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		created, err = tx.CreateGroup(ctx, g)
		return err
	})
	return created, err
}

// GroupPatch leaves nil fields unchanged. An empty description or disease
// model clears it; a target size of 0 removes the target.
type GroupPatch struct {
	Code, Name, Role, DiseaseModelID, Description *string
	TargetSize                                    *int
}

func (s *Service) UpdateGroup(ctx context.Context, actor access.Actor, experimentID, groupID string, p GroupPatch) (domain.Group, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Group{}, err
	}
	if p == (GroupPatch{}) {
		return domain.Group{}, ErrNoChanges
	}
	var updated domain.Group
	err := s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		g, err := tx.LockGroup(ctx, experimentID, groupID)
		if err != nil {
			return err
		}
		f := g.Fields()
		set(&f.Code, p.Code)
		set(&f.Name, p.Name)
		set(&f.Role, p.Role)
		set(&f.DiseaseModelID, p.DiseaseModelID)
		set(&f.Description, p.Description)
		set(&f.TargetSize, p.TargetSize)
		if err := g.Apply(f); err != nil {
			return err
		}
		updated, err = tx.UpdateGroup(ctx, g)
		return err
	})
	return updated, err
}

// DeleteGroup removes a group and its regimen items; a group with allocated
// animals is kept.
func (s *Service) DeleteGroup(ctx context.Context, actor access.Actor, experimentID, groupID string) error {
	if err := actor.Require(access.StudyWrite); err != nil {
		return err
	}
	return s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		return tx.DeleteGroup(ctx, experimentID, groupID)
	})
}

func (s *Service) RegimenItems(ctx context.Context, actor access.Actor, experimentID string) ([]domain.RegimenItem, error) {
	if err := s.readExperiment(ctx, actor, experimentID); err != nil {
		return nil, err
	}
	return s.store.RegimenItems(ctx, experimentID)
}

func (s *Service) CreateRegimenItem(ctx context.Context, actor access.Actor, experimentID string, in domain.RegimenFields) (domain.RegimenItem, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.RegimenItem{}, err
	}
	r, err := domain.NewRegimenItem(experimentID, in)
	if err != nil {
		return domain.RegimenItem{}, err
	}
	var created domain.RegimenItem
	err = s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		if err := allowPurpose(ctx, tx, r); err != nil {
			return err
		}
		created, err = tx.CreateRegimenItem(ctx, r)
		return err
	})
	return created, err
}

// RegimenPatch leaves nil fields unchanged; intervalDays 0 removes the interval.
type RegimenPatch struct {
	GroupID, Purpose, SubstanceID, Amount, Unit, Route, Frequency *string
	IntervalDays, StartDay, EndDay                                *int
}

func (s *Service) UpdateRegimenItem(ctx context.Context, actor access.Actor, experimentID, itemID string, p RegimenPatch) (domain.RegimenItem, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.RegimenItem{}, err
	}
	if p == (RegimenPatch{}) {
		return domain.RegimenItem{}, ErrNoChanges
	}
	var updated domain.RegimenItem
	err := s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		r, err := tx.LockRegimenItem(ctx, experimentID, itemID)
		if err != nil {
			return err
		}
		f := r.Fields()
		set(&f.GroupID, p.GroupID)
		set(&f.Purpose, p.Purpose)
		set(&f.SubstanceID, p.SubstanceID)
		set(&f.Amount, p.Amount)
		set(&f.Unit, p.Unit)
		set(&f.Route, p.Route)
		set(&f.Frequency, p.Frequency)
		set(&f.IntervalDays, p.IntervalDays)
		set(&f.StartDay, p.StartDay)
		set(&f.EndDay, p.EndDay)
		if f.Purpose == string(domain.NoSubstance) {
			// A period without any substance keeps only its days: what the
			// item gave before is dropped unless the patch names it.
			if p.SubstanceID == nil {
				f.SubstanceID = ""
			}
			if p.Amount == nil {
				f.Amount = ""
			}
			if p.Unit == nil {
				f.Unit = ""
			}
			if p.Route == nil {
				f.Route = ""
			}
			if p.Frequency == nil {
				f.Frequency = ""
			}
			if p.IntervalDays == nil {
				f.IntervalDays = 0
			}
		}
		if err := r.Apply(f); err != nil {
			return err
		}
		if err := allowPurpose(ctx, tx, r); err != nil {
			return err
		}
		updated, err = tx.UpdateRegimenItem(ctx, r)
		return err
	})
	return updated, err
}

// allowPurpose rejects an item whose purpose its group's role can never
// receive: a healthy control takes only periods without any substance, every
// other role never takes one. It locks the group so its role cannot change meanwhile. A second
// treatment item of the group is left to the store, which reports
// ErrTreatmentItemTaken.
func allowPurpose(ctx context.Context, tx Store, r domain.RegimenItem) error {
	g, err := tx.LockGroup(ctx, r.ExperimentID, r.GroupID)
	if errors.Is(err, ErrGroupNotFound) {
		return ErrGroupNotInExperiment
	}
	if err != nil {
		return err
	}
	return g.Role.AllowPurpose(r.Purpose)
}

func (s *Service) DeleteRegimenItem(ctx context.Context, actor access.Actor, experimentID, itemID string) error {
	if err := actor.Require(access.StudyWrite); err != nil {
		return err
	}
	return s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		return tx.DeleteRegimenItem(ctx, experimentID, itemID)
	})
}

func (s *Service) Timepoints(ctx context.Context, actor access.Actor, experimentID string) ([]domain.Timepoint, error) {
	if err := s.readExperiment(ctx, actor, experimentID); err != nil {
		return nil, err
	}
	return s.store.Timepoints(ctx, experimentID)
}

func (s *Service) CreateTimepoint(ctx context.Context, actor access.Actor, experimentID string, in domain.TimepointFields) (domain.Timepoint, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Timepoint{}, err
	}
	t, err := domain.NewTimepoint(experimentID, in)
	if err != nil {
		return domain.Timepoint{}, err
	}
	var created domain.Timepoint
	err = s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		created, err = tx.CreateTimepoint(ctx, t)
		return err
	})
	return created, err
}

type TimepointPatch struct {
	Name               *string
	StudyDay, Position *int
}

func (s *Service) UpdateTimepoint(ctx context.Context, actor access.Actor, experimentID, timepointID string, p TimepointPatch) (domain.Timepoint, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Timepoint{}, err
	}
	if p == (TimepointPatch{}) {
		return domain.Timepoint{}, ErrNoChanges
	}
	var updated domain.Timepoint
	err := s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		t, err := tx.LockTimepoint(ctx, experimentID, timepointID)
		if err != nil {
			return err
		}
		f := t.Fields()
		set(&f.Name, p.Name)
		set(&f.StudyDay, p.StudyDay)
		set(&f.Position, p.Position)
		if err := t.Apply(f); err != nil {
			return err
		}
		updated, err = tx.UpdateTimepoint(ctx, t)
		return err
	})
	return updated, err
}

func (s *Service) DeleteTimepoint(ctx context.Context, actor access.Actor, experimentID, timepointID string) error {
	if err := actor.Require(access.StudyWrite); err != nil {
		return err
	}
	return s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		return tx.DeleteTimepoint(ctx, experimentID, timepointID)
	})
}

func (s *Service) Endpoints(ctx context.Context, actor access.Actor, experimentID string) ([]domain.Endpoint, error) {
	if err := s.readExperiment(ctx, actor, experimentID); err != nil {
		return nil, err
	}
	return s.store.Endpoints(ctx, experimentID)
}

// CreateEndpoint accepts only a metric that a paradigm version of the
// experiment's protocol steps computes.
func (s *Service) CreateEndpoint(ctx context.Context, actor access.Actor, experimentID string, in domain.EndpointFields) (domain.Endpoint, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Endpoint{}, err
	}
	e, err := domain.NewEndpoint(experimentID, in)
	if err != nil {
		return domain.Endpoint{}, err
	}
	var created domain.Endpoint
	err = s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		if err := s.checkMetric(ctx, tx, e); err != nil {
			return err
		}
		created, err = tx.CreateEndpoint(ctx, e)
		return err
	})
	return created, err
}

// EndpointPatch leaves nil fields unchanged; an empty baseline clears it.
type EndpointPatch struct {
	TimepointID, BaselineTimepointID, ParadigmKey, MetricKey, Kind, Direction *string
}

func (s *Service) UpdateEndpoint(ctx context.Context, actor access.Actor, experimentID, endpointID string, p EndpointPatch) (domain.Endpoint, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Endpoint{}, err
	}
	if p == (EndpointPatch{}) {
		return domain.Endpoint{}, ErrNoChanges
	}
	var updated domain.Endpoint
	err := s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		e, err := tx.LockEndpoint(ctx, experimentID, endpointID)
		if err != nil {
			return err
		}
		f := e.Fields()
		set(&f.TimepointID, p.TimepointID)
		set(&f.BaselineTimepointID, p.BaselineTimepointID)
		set(&f.ParadigmKey, p.ParadigmKey)
		set(&f.MetricKey, p.MetricKey)
		set(&f.Kind, p.Kind)
		set(&f.Direction, p.Direction)
		before := e
		if err := e.Apply(f); err != nil {
			return err
		}
		if e.ParadigmKey != before.ParadigmKey || e.MetricKey != before.MetricKey {
			if err := s.checkMetric(ctx, tx, e); err != nil {
				return err
			}
		}
		updated, err = tx.UpdateEndpoint(ctx, e)
		return err
	})
	return updated, err
}

func (s *Service) DeleteEndpoint(ctx context.Context, actor access.Actor, experimentID, endpointID string) error {
	if err := actor.Require(access.StudyWrite); err != nil {
		return err
	}
	return s.store.Transaction(ctx, func(tx Store) error {
		if err := draft(ctx, tx, experimentID); err != nil {
			return err
		}
		return tx.DeleteEndpoint(ctx, experimentID, endpointID)
	})
}

func (s *Service) checkMetric(ctx context.Context, tx Store, e domain.Endpoint) error {
	metrics, err := s.metrics(ctx, tx, e.ExperimentID)
	if err != nil {
		return err
	}
	if !(domain.Design{Metrics: metrics}).MetricAvailable(e.ParadigmKey, e.MetricKey) {
		return ErrMetricUnavailable
	}
	return nil
}
