// Package application contains dosing use cases. Recording administrations and
// induction outcomes needs test:run; reads need *:read.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/dosing/domain"
	"time"
)

var (
	ErrExperimentNotFound    = errors.New("experiment not found")
	ErrExperimentNotRunning  = errors.New("the experiment is not running")
	ErrExperimentNotStarted  = errors.New("the experiment has not started, so it has no day 0")
	ErrAllocationNotFound    = errors.New("allocation not found in this experiment")
	ErrRegimenItemNotFound   = errors.New("regimen item not found in this experiment")
	ErrRegimenItemOtherGroup = errors.New("the regimen item belongs to another group than the animal's")
	ErrSubstanceNotFound     = errors.New("substance not found")
	ErrWeightNotFound        = errors.New("weight measurement not found")
	ErrSubstanceRequired     = errors.New("an unplanned dose needs a substance, purpose, amount, unit and route")
	ErrSubstanceMismatch     = errors.New("a planned dose gives the regimen item's substance")
	ErrPurposeMismatch       = errors.New("a planned dose has the regimen item's purpose")
	ErrDoseIncomplete        = errors.New("amount and unit must be given together")
	ErrNoDisease             = errors.New("the animal's group has no disease model to confirm")
	ErrOutcomeExists         = errors.New("an induction outcome is already recorded for this animal")
	ErrInvalidQuery          = errors.New("invalid list query")
)

// AllocationRef is an allocation with what records about it are checked against.
type AllocationRef struct {
	domain.Allocation
	ExperimentStatus string
	HasDisease       bool
}

// ExperimentDay is an experiment's status and day 0, and the laboratory's
// current calendar date.
type ExperimentDay struct {
	Status  string
	DayZero *time.Time
	Today   time.Time
}

type AdministrationFilter struct {
	ExperimentID, AllocationID, SubjectID, SubstanceID string
	Purpose                                            domain.Purpose
	From, To                                           *time.Time
	Limit, Offset                                      int
}

type OutcomeFilter struct {
	Status domain.OutcomeStatus
}

// OutcomeRow is an induction outcome with the animal and group it belongs to.
type OutcomeRow struct {
	domain.InductionOutcome
	SubjectID, SubjectCode, GroupID, GroupCode string
}

// Store persists dosing records. Scoped lookups return the matching not-found
// error for rows of another experiment.
type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	Allocation(ctx context.Context, experimentID, allocationID string) (AllocationRef, error)
	RegimenItem(ctx context.Context, experimentID, itemID string) (domain.RegimenItem, error)
	SubstanceName(ctx context.Context, id string) (string, error)
	Weight(ctx context.Context, id string) (domain.Weight, error)
	// LatestWeight returns the animal's latest weight measured at or before at, or nil.
	LatestWeight(ctx context.Context, subjectID string, at time.Time) (*domain.Weight, error)
	CreateAdministration(ctx context.Context, a domain.Administration) (domain.Administration, error)
	ListAdministrations(ctx context.Context, f AdministrationFilter) ([]domain.Administration, int, error)
	ExperimentDay(ctx context.Context, experimentID string) (ExperimentDay, error)
	// DueCandidates returns every regimen item of every animal allocated by the
	// end of day and not excluded before it, with the doses of that item given
	// that day and the latest weight by the end of it, in the laboratory's time
	// zone, ordered by group code, subject code, purpose and start day.
	DueCandidates(ctx context.Context, experimentID string, day time.Time) ([]domain.DueCandidate, error)
	CreateInductionOutcome(ctx context.Context, o domain.InductionOutcome) (domain.InductionOutcome, error)
	ListInductionOutcomes(ctx context.Context, experimentID string, f OutcomeFilter) ([]OutcomeRow, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func New(store Store, now func() time.Time) *Service { return &Service{store: store, now: now} }

// AdministrationInput records a planned dose when RegimenItemID is set: the
// purpose, substance, dose and route default to the item's, and a different
// actual amount or route may be given. An unplanned dose names them all.
// WeightID picks the weight of a per-kilogram dose; by default the animal's
// latest weight at AdministeredAt is used.
type AdministrationInput struct {
	RegimenItemID, Purpose, SubstanceID string
	Amount, Unit, Route                 string
	WeightID, Notes                     string
	AdministeredAt                      time.Time
}

// RecordAdministration stores one dose actually given to an allocated animal
// of a running experiment.
func (s *Service) RecordAdministration(ctx context.Context, actor access.Actor, experimentID, allocationID string, in AdministrationInput) (domain.Administration, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Administration{}, err
	}
	a := domain.Administration{AdministeredAt: instant(in.AdministeredAt), RecordedBy: actor.UserID}
	var err error
	if a.Notes, err = domain.NormalizeNotes(in.Notes); err != nil {
		return domain.Administration{}, err
	}
	if (in.Amount == "") != (in.Unit == "") {
		return domain.Administration{}, ErrDoseIncomplete
	}
	if in.Amount != "" {
		if a.Dose, err = domain.ParseDose(in.Amount, in.Unit); err != nil {
			return domain.Administration{}, err
		}
	}
	if in.Route != "" {
		if a.Route, err = domain.ParseRoute(in.Route); err != nil {
			return domain.Administration{}, err
		}
	}
	if in.Purpose != "" {
		if a.Purpose, err = domain.ParsePurpose(in.Purpose); err != nil {
			return domain.Administration{}, err
		}
	}
	if in.RegimenItemID == "" && (in.SubstanceID == "" || a.Purpose == "" || in.Amount == "" || a.Route == "") {
		return domain.Administration{}, ErrSubstanceRequired
	}
	var created domain.Administration
	err = s.store.Transaction(ctx, func(tx Store) error {
		alloc, err := s.allocation(ctx, tx, experimentID, allocationID, a.AdministeredAt)
		if err != nil {
			return err
		}
		a.ExperimentID, a.AllocationID, a.GroupID, a.SubjectID = alloc.ExperimentID, alloc.ID, alloc.GroupID, alloc.SubjectID
		if in.RegimenItemID != "" {
			item, err := tx.RegimenItem(ctx, experimentID, in.RegimenItemID)
			if err != nil {
				return err
			}
			switch {
			case item.GroupID != alloc.GroupID:
				return ErrRegimenItemOtherGroup
			case in.SubstanceID != "" && in.SubstanceID != item.SubstanceID:
				return ErrSubstanceMismatch
			case a.Purpose != "" && a.Purpose != item.Purpose:
				return ErrPurposeMismatch
			}
			a.RegimenItemID, a.Purpose, a.SubstanceID, a.SubstanceName = item.ID, item.Purpose, item.SubstanceID, item.SubstanceName
			if a.Dose == (domain.Dose{}) {
				a.Dose = item.Dose
			}
			if a.Route == "" {
				a.Route = item.Route
			}
		} else {
			if a.SubstanceName, err = tx.SubstanceName(ctx, in.SubstanceID); err != nil {
				return err
			}
			a.SubstanceID = in.SubstanceID
		}
		weight, err := s.weight(ctx, tx, a, in.WeightID)
		if err != nil {
			return err
		}
		if err := domain.CheckWeight(a, weight); err != nil {
			return err
		}
		if weight != nil {
			a.WeightID, a.BodyMilligrams = weight.ID, weight.Milligrams
			// The amount actually given must be representable.
			if _, err := a.Dose.Absolute(a.BodyMilligrams); err != nil {
				return err
			}
		}
		created, err = tx.CreateAdministration(ctx, a)
		return err
	})
	if err != nil {
		return domain.Administration{}, err
	}
	return created, nil
}

// allocation loads the animal and checks the experiment runs and the time fits
// the allocation.
func (s *Service) allocation(ctx context.Context, tx Store, experimentID, allocationID string, at time.Time) (AllocationRef, error) {
	alloc, err := tx.Allocation(ctx, experimentID, allocationID)
	if err != nil {
		return AllocationRef{}, err
	}
	if alloc.ExperimentStatus != "RUNNING" {
		return AllocationRef{}, ErrExperimentNotRunning
	}
	if err := domain.CheckTime(at, alloc.Allocation, s.now()); err != nil {
		return AllocationRef{}, err
	}
	return alloc, nil
}

// weight resolves the weight of a dose: the named one, or for a per-kilogram
// dose the animal's latest weight measured by the time of the dose.
func (s *Service) weight(ctx context.Context, tx Store, a domain.Administration, weightID string) (*domain.Weight, error) {
	if weightID != "" {
		w, err := tx.Weight(ctx, weightID)
		if err != nil {
			return nil, err
		}
		return &w, nil
	}
	if !a.Dose.Unit.PerBodyMass() {
		return nil, nil
	}
	return tx.LatestWeight(ctx, a.SubjectID, a.AdministeredAt)
}

// AdministrationQuery filters by animal, substance, purpose and a [From, To)
// range of administration times.
type AdministrationQuery struct {
	AllocationID, SubstanceID, Purpose string
	From, To                           time.Time
	Page, PageSize                     int
}

type AdministrationPage struct {
	Administrations       []domain.Administration
	Total, Page, PageSize int
}

// ExperimentAdministrations lists the doses given in an experiment, newest first.
func (s *Service) ExperimentAdministrations(ctx context.Context, actor access.Actor, experimentID string, q AdministrationQuery) (AdministrationPage, error) {
	return s.administrations(ctx, actor, AdministrationFilter{ExperimentID: experimentID, AllocationID: q.AllocationID}, q)
}

// SubjectAdministrations lists the doses given to one animal, newest first.
func (s *Service) SubjectAdministrations(ctx context.Context, actor access.Actor, subjectID string, q AdministrationQuery) (AdministrationPage, error) {
	return s.administrations(ctx, actor, AdministrationFilter{SubjectID: subjectID}, q)
}

func (s *Service) administrations(ctx context.Context, actor access.Actor, f AdministrationFilter, q AdministrationQuery) (AdministrationPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return AdministrationPage{}, err
	}
	if !q.From.IsZero() && !q.To.IsZero() && !q.To.After(q.From) {
		return AdministrationPage{}, ErrInvalidQuery
	}
	if q.Purpose != "" {
		p, err := domain.ParsePurpose(q.Purpose)
		if err != nil {
			return AdministrationPage{}, ErrInvalidQuery
		}
		f.Purpose = p
	}
	page, limit, offset, err := paging(q.Page, q.PageSize)
	if err != nil {
		return AdministrationPage{}, err
	}
	f.SubstanceID, f.Limit, f.Offset = q.SubstanceID, limit, offset
	if !q.From.IsZero() {
		from := instant(q.From)
		f.From = &from
	}
	if !q.To.IsZero() {
		to := instant(q.To)
		f.To = &to
	}
	list, total, err := s.store.ListAdministrations(ctx, f)
	if err != nil {
		return AdministrationPage{}, err
	}
	return AdministrationPage{Administrations: list, Total: total, Page: page, PageSize: limit}, nil
}

// DueDoses lists every dose due on a calendar date (the laboratory's today
// when date is empty) from the experiment's regimen items and day 0, per
// animal, with how many were given that day.
func (s *Service) DueDoses(ctx context.Context, actor access.Actor, experimentID, date string) (domain.Schedule, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Schedule{}, err
	}
	var day time.Time
	if date != "" {
		var err error
		if day, err = domain.ParseDate(date); err != nil {
			return domain.Schedule{}, err
		}
	}
	exp, err := s.store.ExperimentDay(ctx, experimentID)
	if err != nil {
		return domain.Schedule{}, err
	}
	if exp.DayZero == nil {
		return domain.Schedule{}, ErrExperimentNotStarted
	}
	if day.IsZero() {
		day = exp.Today
	}
	candidates, err := s.store.DueCandidates(ctx, experimentID, day)
	if err != nil {
		return domain.Schedule{}, err
	}
	return domain.BuildSchedule(*exp.DayZero, day, candidates), nil
}

type OutcomeInput struct {
	Status, Note string
	AssessedAt   time.Time
}

// RecordInductionOutcome stores whether an animal's disease model was
// confirmed. Each animal has at most one outcome and it never changes.
func (s *Service) RecordInductionOutcome(ctx context.Context, actor access.Actor, experimentID, allocationID string, in OutcomeInput) (domain.InductionOutcome, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.InductionOutcome{}, err
	}
	o := domain.InductionOutcome{AssessedAt: instant(in.AssessedAt), RecordedBy: actor.UserID}
	var err error
	if o.Status, err = domain.ParseOutcomeStatus(in.Status); err != nil {
		return domain.InductionOutcome{}, err
	}
	if o.Note, err = domain.NormalizeNotes(in.Note); err != nil {
		return domain.InductionOutcome{}, err
	}
	var created domain.InductionOutcome
	err = s.store.Transaction(ctx, func(tx Store) error {
		alloc, err := s.allocation(ctx, tx, experimentID, allocationID, o.AssessedAt)
		if err != nil {
			return err
		}
		if !alloc.HasDisease {
			return ErrNoDisease
		}
		o.ExperimentID, o.AllocationID = alloc.ExperimentID, alloc.ID
		created, err = tx.CreateInductionOutcome(ctx, o)
		return err
	})
	if err != nil {
		return domain.InductionOutcome{}, err
	}
	return created, nil
}

// InductionOutcomes lists an experiment's outcomes by group and animal code.
func (s *Service) InductionOutcomes(ctx context.Context, actor access.Actor, experimentID, status string) ([]OutcomeRow, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	var f OutcomeFilter
	if status != "" {
		st, err := domain.ParseOutcomeStatus(status)
		if err != nil {
			return nil, ErrInvalidQuery
		}
		f.Status = st
	}
	return s.store.ListInductionOutcomes(ctx, experimentID, f)
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
