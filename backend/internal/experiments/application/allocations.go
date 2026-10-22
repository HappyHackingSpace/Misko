package application

import (
	"context"
	"strings"
	"time"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
)

// Allocations change while the experiment is a draft or running. Every write
// locks the experiment first, which also serialises allocations of one
// experiment so randomisation sees the current group fill.

// AllocationList is every allocation of an experiment that matches a filter,
// with the fill of every group.
type AllocationList struct {
	Allocations []AllocationView
	Groups      []GroupFill
}

type AllocationQuery struct {
	GroupID  string
	Excluded string
}

func (s *Service) Allocations(ctx context.Context, actor access.Actor, experimentID string, q AllocationQuery) (AllocationList, error) {
	if err := actor.Require(access.Read); err != nil {
		return AllocationList{}, err
	}
	f := AllocationFilter{ExperimentID: experimentID, GroupID: strings.TrimSpace(q.GroupID)}
	switch q.Excluded {
	case "":
	case "true", "false":
		excluded := q.Excluded == "true"
		f.Excluded = &excluded
	default:
		return AllocationList{}, ErrInvalidQuery
	}
	if _, err := s.store.Experiment(ctx, experimentID); err != nil {
		return AllocationList{}, err
	}
	groups, err := s.store.Groups(ctx, experimentID)
	if err != nil {
		return AllocationList{}, err
	}
	allocations, err := s.store.Allocations(ctx, f)
	if err != nil {
		return AllocationList{}, err
	}
	return AllocationList{Allocations: allocations, Groups: groups}, nil
}

type AllocateInput struct {
	SubjectID, GroupID, Cage string
	// AllocatedAt defaults to now.
	AllocatedAt time.Time
}

func (s *Service) Allocate(ctx context.Context, actor access.Actor, experimentID string, in AllocateInput) (domain.Allocation, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Allocation{}, err
	}
	a, err := domain.NewAllocation(experimentID, in.GroupID, in.SubjectID, in.Cage, in.AllocatedAt, s.now(), actor.UserID)
	if err != nil {
		return domain.Allocation{}, err
	}
	var created domain.Allocation
	err = s.store.Transaction(ctx, func(tx Store) error {
		if err := allocatable(ctx, tx, experimentID); err != nil {
			return err
		}
		created, err = tx.CreateAllocation(ctx, a)
		return err
	})
	return created, err
}

// AllocationPatch moves an animal to another group (draft only) or sets its
// cage; an empty cage clears it.
type AllocationPatch struct {
	GroupID, Cage *string
}

func (s *Service) UpdateAllocation(ctx context.Context, actor access.Actor, experimentID, allocationID string, p AllocationPatch) (domain.Allocation, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Allocation{}, err
	}
	if p == (AllocationPatch{}) {
		return domain.Allocation{}, ErrNoChanges
	}
	var cage *string
	if p.Cage != nil {
		normalized, err := domain.NormalizeCage(*p.Cage)
		if err != nil {
			return domain.Allocation{}, err
		}
		cage = &normalized
	}
	var updated domain.Allocation
	err := s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.LockExperiment(ctx, experimentID)
		if err != nil {
			return err
		}
		a, err := tx.LockAllocation(ctx, experimentID, allocationID)
		if err != nil {
			return err
		}
		if e.Status == domain.Completed {
			return ErrAllocationFrozen
		}
		if p.GroupID != nil && strings.TrimSpace(*p.GroupID) != a.GroupID {
			if e.Status != domain.Draft {
				return ErrAllocationFrozen
			}
			a.GroupID = strings.TrimSpace(*p.GroupID)
		}
		set(&a.Cage, cage)
		updated, err = tx.UpdateAllocation(ctx, a)
		return err
	})
	return updated, err
}

// RemoveAllocation takes an animal out of a draft study; once the study runs
// it can only be excluded.
func (s *Service) RemoveAllocation(ctx context.Context, actor access.Actor, experimentID, allocationID string) error {
	if err := actor.Require(access.StudyWrite); err != nil {
		return err
	}
	return s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.LockExperiment(ctx, experimentID)
		if err != nil {
			return err
		}
		if _, err := tx.LockAllocation(ctx, experimentID, allocationID); err != nil {
			return err
		}
		if e.Status != domain.Draft {
			return ErrAllocationFrozen
		}
		return tx.DeleteAllocation(ctx, experimentID, allocationID)
	})
}

type ExcludeInput struct {
	Reason, Note string
	// ExcludedAt defaults to now.
	ExcludedAt time.Time
}

// Exclude records, once, why an animal leaves the analysis. The animal stays
// allocated so the exclusion is reported.
func (s *Service) Exclude(ctx context.Context, actor access.Actor, experimentID, allocationID string, in ExcludeInput) (domain.Allocation, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Allocation{}, err
	}
	var updated domain.Allocation
	err := s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.LockExperiment(ctx, experimentID)
		if err != nil {
			return err
		}
		a, err := tx.LockAllocation(ctx, experimentID, allocationID)
		if err != nil {
			return err
		}
		if e.Status == domain.Completed {
			return ErrAllocationFrozen
		}
		if err := a.Exclude(in.Reason, in.Note, in.ExcludedAt, s.now()); err != nil {
			return err
		}
		updated, err = tx.UpdateAllocation(ctx, a)
		return err
	})
	return updated, err
}

type RandomiseInput struct {
	SubjectIDs []string
	// GroupIDs limits randomisation to these groups; empty means every group.
	GroupIDs []string
	Cage     string
}

const maxRandomised = 1000

// Randomise distributes animals at random over groups up to each group's
// target size, filling groups evenly. Groups without a target take no animals.
// Either every animal is allocated or none.
func (s *Service) Randomise(ctx context.Context, actor access.Actor, experimentID string, in RandomiseInput) ([]domain.Allocation, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return nil, err
	}
	subjects, ok := distinct(in.SubjectIDs)
	if !ok || len(subjects) == 0 || len(subjects) > maxRandomised {
		return nil, ErrInvalidSubjects
	}
	chosen, ok := distinct(in.GroupIDs)
	if !ok {
		return nil, ErrInvalidGroups
	}
	now := s.now()
	template, err := domain.NewAllocation(experimentID, "", "", in.Cage, now, now, actor.UserID)
	if err != nil {
		return nil, err
	}
	var created []domain.Allocation
	err = s.store.Transaction(ctx, func(tx Store) error {
		if err := allocatable(ctx, tx, experimentID); err != nil {
			return err
		}
		fills, err := tx.Groups(ctx, experimentID)
		if err != nil {
			return err
		}
		capacities, err := capacities(fills, chosen)
		if err != nil {
			return err
		}
		plan := domain.PlanRandomisation(subjects, capacities, s.intn)
		if plan == nil {
			return ErrNotEnoughPlaces
		}
		created = make([]domain.Allocation, 0, len(subjects))
		for _, subject := range subjects {
			a := template
			a.SubjectID, a.GroupID = subject, plan[subject]
			stored, err := tx.CreateAllocation(ctx, a)
			if err != nil {
				return err
			}
			created = append(created, stored)
		}
		return nil
	})
	return created, err
}

func capacities(fills []GroupFill, chosen []string) ([]domain.Capacity, error) {
	known := map[string]GroupFill{}
	for _, f := range fills {
		known[f.Group.ID] = f
	}
	if len(chosen) == 0 {
		for _, f := range fills {
			chosen = append(chosen, f.Group.ID)
		}
	}
	out := make([]domain.Capacity, 0, len(chosen))
	for _, id := range chosen {
		f, ok := known[id]
		if !ok {
			return nil, ErrGroupNotInExperiment
		}
		if f.Group.TargetSize > 0 {
			out = append(out, domain.Capacity{GroupID: id, Free: f.Group.TargetSize - f.Allocated})
		}
	}
	return out, nil
}

// allocatable locks the experiment and checks it accepts animals.
func allocatable(ctx context.Context, tx Store, id string) error {
	e, err := tx.LockExperiment(ctx, id)
	if err != nil {
		return err
	}
	if e.Status == domain.Completed {
		return ErrAllocationFrozen
	}
	return nil
}

func distinct(ids []string) ([]string, bool) {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			return nil, false
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, true
}
