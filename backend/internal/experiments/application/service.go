// Package application contains the study design use cases: experiments and
// their lifecycle, groups, regimen items, timepoints, endpoints and the
// allocation of animals to groups. Writes need study:write; reads need *:read.
package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
)

var (
	ErrExperimentNotFound  = errors.New("experiment not found")
	ErrGroupNotFound       = errors.New("group not found in this experiment")
	ErrRegimenItemNotFound = errors.New("regimen item not found in this experiment")
	ErrTimepointNotFound   = errors.New("timepoint not found in this experiment")
	ErrEndpointNotFound    = errors.New("endpoint not found in this experiment")
	ErrAllocationNotFound  = errors.New("allocation not found in this experiment")

	ErrCodeTaken            = errors.New("experiment code is already in use")
	ErrGroupCodeTaken       = errors.New("another group of this experiment uses this code")
	ErrGroupNameTaken       = errors.New("another group of this experiment uses this name")
	ErrGroupInUse           = errors.New("group has allocated animals and cannot be deleted")
	ErrGroupRegimenConflict = errors.New("the group's regimen items do not fit the new role; a healthy control has only periods without any substance and no other role has one")
	ErrTimepointConflict    = errors.New("another timepoint of this experiment uses this name or position")
	ErrTimepointInUse       = errors.New("timepoint is used by an endpoint or a test and cannot be deleted")
	ErrTreatmentItemTaken   = errors.New("the group already has a treatment item")
	ErrEndpointDuplicate    = errors.New("this metric is already an endpoint at this timepoint")
	ErrPrimaryEndpointTaken = errors.New("the experiment already has a primary endpoint")
	ErrAlreadyAllocated     = errors.New("the animal is already allocated to a group")

	ErrDiseaseModelNotFound     = errors.New("disease model not found")
	ErrSubstanceNotFound        = errors.New("substance not found")
	ErrSubjectNotFound          = errors.New("subject not found")
	ErrGroupNotInExperiment     = errors.New("group does not belong to this experiment")
	ErrTimepointNotInExperiment = errors.New("timepoint does not belong to this experiment")
	ErrMetricUnavailable        = errors.New("no protocol step of this experiment runs a paradigm version with this metric")
	ErrInvalidDayZero           = errors.New("day zero must be a date formatted YYYY-MM-DD")
	ErrInvalidSubjects          = errors.New("subject ids must be 1 to 1000 distinct values")
	ErrInvalidGroups            = errors.New("group ids must be distinct")
	ErrNotEnoughPlaces          = errors.New("the groups do not have enough free places below their target size for these animals")
	ErrDesignFrozen             = errors.New("the study design can change only while the experiment is a draft")
	ErrAllocationFrozen         = errors.New("allocations of a running experiment keep their group, and a completed experiment cannot change")
	ErrNotDraft                 = errors.New("the experiment is not a draft")
	ErrNotRunning               = errors.New("the experiment is not running")
	ErrExperimentCompleted      = errors.New("a completed experiment cannot change")
	ErrOpenTests                = errors.New("every test must be completed or cancelled before the experiment completes")
	ErrInvalidTransition        = errors.New("the experiment cannot change this way")
	ErrNotReady                 = errors.New("the experiment does not meet the start rules")
	ErrNoChanges                = errors.New("no fields to update")
	ErrInvalidQuery             = errors.New("invalid list query")
)

// NotReadyError carries the unmet start rules; it matches ErrNotReady.
type NotReadyError struct{ Problems []domain.Problem }

func (e *NotReadyError) Error() string { return ErrNotReady.Error() }
func (e *NotReadyError) Unwrap() error { return ErrNotReady }

type ExperimentFilter struct {
	Search        string
	Status        string
	Sort          string
	Descending    bool
	Limit, Offset int
}

// GroupFill compares a group's target with its allocated animals; excluded
// animals are counted apart and do not fill the group.
type GroupFill struct {
	Group               domain.Group
	Allocated, Excluded int
}

type ParadigmVersion struct {
	Key     string
	Version int
}

type AllocationFilter struct {
	ExperimentID, GroupID string
	// Excluded filters by exclusion when set.
	Excluded *bool
}

// AllocationView is an allocation with the animal's code.
type AllocationView struct {
	Allocation  domain.Allocation
	SubjectCode string
}

// Store persists the study design. Lookups scoped by experiment return the
// matching not-found error for records of another experiment. Lock methods
// take a row lock and are meant for use inside Transaction.
type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error

	CreateExperiment(ctx context.Context, e domain.Experiment) (domain.Experiment, error)
	Experiment(ctx context.Context, id string) (domain.Experiment, error)
	LockExperiment(ctx context.Context, id string) (domain.Experiment, error)
	ListExperiments(ctx context.Context, f ExperimentFilter) ([]domain.Experiment, int, error)
	UpdateExperiment(ctx context.Context, e domain.Experiment) (domain.Experiment, error)
	StartExperiment(ctx context.Context, id string, dayZero time.Time) (domain.Experiment, error)
	CompleteExperiment(ctx context.Context, id string) (domain.Experiment, error)
	// OpenTests counts tests of the experiment that are planned or in progress.
	OpenTests(ctx context.Context, experimentID string) (int, error)
	// ProtocolParadigms lists the paradigm versions the experiment's protocol steps run.
	ProtocolParadigms(ctx context.Context, experimentID string) ([]ParadigmVersion, error)

	Groups(ctx context.Context, experimentID string) ([]GroupFill, error)
	LockGroup(ctx context.Context, experimentID, groupID string) (domain.Group, error)
	CreateGroup(ctx context.Context, g domain.Group) (domain.Group, error)
	UpdateGroup(ctx context.Context, g domain.Group) (domain.Group, error)
	// DeleteGroup removes the group with its regimen items.
	DeleteGroup(ctx context.Context, experimentID, groupID string) error

	RegimenItems(ctx context.Context, experimentID string) ([]domain.RegimenItem, error)
	LockRegimenItem(ctx context.Context, experimentID, itemID string) (domain.RegimenItem, error)
	CreateRegimenItem(ctx context.Context, r domain.RegimenItem) (domain.RegimenItem, error)
	UpdateRegimenItem(ctx context.Context, r domain.RegimenItem) (domain.RegimenItem, error)
	DeleteRegimenItem(ctx context.Context, experimentID, itemID string) error

	Timepoints(ctx context.Context, experimentID string) ([]domain.Timepoint, error)
	LockTimepoint(ctx context.Context, experimentID, timepointID string) (domain.Timepoint, error)
	CreateTimepoint(ctx context.Context, t domain.Timepoint) (domain.Timepoint, error)
	UpdateTimepoint(ctx context.Context, t domain.Timepoint) (domain.Timepoint, error)
	DeleteTimepoint(ctx context.Context, experimentID, timepointID string) error

	Endpoints(ctx context.Context, experimentID string) ([]domain.Endpoint, error)
	LockEndpoint(ctx context.Context, experimentID, endpointID string) (domain.Endpoint, error)
	CreateEndpoint(ctx context.Context, e domain.Endpoint) (domain.Endpoint, error)
	UpdateEndpoint(ctx context.Context, e domain.Endpoint) (domain.Endpoint, error)
	DeleteEndpoint(ctx context.Context, experimentID, endpointID string) error

	Allocations(ctx context.Context, f AllocationFilter) ([]AllocationView, error)
	LockAllocation(ctx context.Context, experimentID, allocationID string) (domain.Allocation, error)
	CreateAllocation(ctx context.Context, a domain.Allocation) (domain.Allocation, error)
	UpdateAllocation(ctx context.Context, a domain.Allocation) (domain.Allocation, error)
	DeleteAllocation(ctx context.Context, experimentID, allocationID string) error
}

// Catalog reads the code-defined paradigm catalog.
type Catalog interface {
	// Metrics returns the metric keys a paradigm version computes.
	Metrics(key string, version int) ([]string, error)
}

type Service struct {
	store   Store
	catalog Catalog
	now     func() time.Time
	intn    func(int) int
}

// New wires the use cases. intn(n) returns a uniform random integer in [0, n)
// and drives randomised allocation.
func New(store Store, catalog Catalog, now func() time.Time, intn func(int) int) *Service {
	return &Service{store: store, catalog: catalog, now: now, intn: intn}
}

type ExperimentQuery struct {
	Search, Status, Sort, Order string
	Page, PageSize              int
}

type ExperimentPage struct {
	Experiments           []domain.Experiment
	Total, Page, PageSize int
}

var experimentSorts = []string{"code", "title", "createdAt"}

func (s *Service) ListExperiments(ctx context.Context, actor access.Actor, q ExperimentQuery) (ExperimentPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return ExperimentPage{}, err
	}
	f := ExperimentFilter{Search: strings.TrimSpace(q.Search), Status: q.Status, Sort: q.Sort}
	if f.Sort == "" {
		f.Sort = "createdAt"
	}
	if !utf8.ValidString(f.Search) || utf8.RuneCountInString(f.Search) > 100 || !slices.Contains(experimentSorts, f.Sort) {
		return ExperimentPage{}, ErrInvalidQuery
	}
	if f.Status != "" {
		if _, err := domain.ParseStatus(f.Status); err != nil {
			return ExperimentPage{}, ErrInvalidQuery
		}
	}
	page := 0
	var err error
	if f.Descending, err = descending(q.Order, f.Sort == "createdAt"); err != nil {
		return ExperimentPage{}, err
	}
	if page, f.Limit, f.Offset, err = paging(q.Page, q.PageSize); err != nil {
		return ExperimentPage{}, err
	}
	experiments, total, err := s.store.ListExperiments(ctx, f)
	if err != nil {
		return ExperimentPage{}, err
	}
	return ExperimentPage{Experiments: experiments, Total: total, Page: page, PageSize: f.Limit}, nil
}

func (s *Service) Experiment(ctx context.Context, actor access.Actor, id string) (domain.Experiment, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Experiment{}, err
	}
	return s.store.Experiment(ctx, id)
}

func (s *Service) CreateExperiment(ctx context.Context, actor access.Actor, in domain.ExperimentFields) (domain.Experiment, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Experiment{}, err
	}
	e, err := domain.NewExperiment(in)
	if err != nil {
		return domain.Experiment{}, err
	}
	return s.store.CreateExperiment(ctx, e)
}

// ExperimentPatch leaves nil fields unchanged; an empty optional text clears it.
type ExperimentPatch struct {
	Code, Title, Description, Hypothesis, Randomisation *string
	Blinded                                             *bool
}

// UpdateExperiment edits code, title and description until the experiment
// completes; hypothesis, randomisation and blinding only in a draft.
func (s *Service) UpdateExperiment(ctx context.Context, actor access.Actor, id string, p ExperimentPatch) (domain.Experiment, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Experiment{}, err
	}
	if p == (ExperimentPatch{}) {
		return domain.Experiment{}, ErrNoChanges
	}
	var updated domain.Experiment
	err := s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.LockExperiment(ctx, id)
		if err != nil {
			return err
		}
		if e.Status == domain.Completed {
			return ErrExperimentCompleted
		}
		f := e.Fields()
		set(&f.Code, p.Code)
		set(&f.Title, p.Title)
		set(&f.Description, p.Description)
		set(&f.Hypothesis, p.Hypothesis)
		set(&f.Randomisation, p.Randomisation)
		set(&f.Blinded, p.Blinded)
		next := e
		if err := next.Apply(f); err != nil {
			return err
		}
		if e.Status != domain.Draft && e.DesignChanged(next.Fields()) {
			return ErrDesignFrozen
		}
		updated, err = tx.UpdateExperiment(ctx, next)
		return err
	})
	return updated, err
}

// Readiness is the start check of an experiment.
type Readiness struct {
	Status   domain.Status
	Problems []domain.Problem
}

func (s *Service) Readiness(ctx context.Context, actor access.Actor, id string) (Readiness, error) {
	if err := actor.Require(access.Read); err != nil {
		return Readiness{}, err
	}
	e, err := s.store.Experiment(ctx, id)
	if err != nil {
		return Readiness{}, err
	}
	design, err := s.design(ctx, s.store, id)
	if err != nil {
		return Readiness{}, err
	}
	return Readiness{Status: e.Status, Problems: domain.Readiness(design)}, nil
}

// Start moves a draft to RUNNING with day 0 when every start rule holds, which
// freezes groups, regimens, timepoints and endpoints. Otherwise it returns a
// NotReadyError listing the unmet rules.
func (s *Service) Start(ctx context.Context, actor access.Actor, id, dayZero string) (domain.Experiment, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Experiment{}, err
	}
	day, err := time.Parse(time.DateOnly, strings.TrimSpace(dayZero))
	if err != nil {
		return domain.Experiment{}, ErrInvalidDayZero
	}
	var started domain.Experiment
	err = s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.LockExperiment(ctx, id)
		if err != nil {
			return err
		}
		if e.Status != domain.Draft {
			return ErrNotDraft
		}
		design, err := s.design(ctx, tx, id)
		if err != nil {
			return err
		}
		if problems := domain.Readiness(design); len(problems) > 0 {
			return &NotReadyError{Problems: problems}
		}
		started, err = tx.StartExperiment(ctx, id, day)
		return err
	})
	return started, err
}

// Complete closes a running experiment for good once every test is completed
// or cancelled.
func (s *Service) Complete(ctx context.Context, actor access.Actor, id string) (domain.Experiment, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Experiment{}, err
	}
	var completed domain.Experiment
	err := s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.LockExperiment(ctx, id)
		if err != nil {
			return err
		}
		if e.Status != domain.Running {
			return ErrNotRunning
		}
		open, err := tx.OpenTests(ctx, id)
		if err != nil {
			return err
		}
		if open > 0 {
			return ErrOpenTests
		}
		completed, err = tx.CompleteExperiment(ctx, id)
		return err
	})
	return completed, err
}

// MetricOption is a paradigm version used by the experiment's protocols with
// the metrics an endpoint can name.
type MetricOption struct {
	Paradigm ParadigmVersion
	Metrics  []string
}

// MetricOptions lists what endpoints of the experiment can measure.
func (s *Service) MetricOptions(ctx context.Context, actor access.Actor, id string) ([]MetricOption, error) {
	if err := s.readExperiment(ctx, actor, id); err != nil {
		return nil, err
	}
	paradigms, err := s.store.ProtocolParadigms(ctx, id)
	if err != nil {
		return nil, err
	}
	options := make([]MetricOption, 0, len(paradigms))
	for _, p := range paradigms {
		metrics, err := s.catalog.Metrics(p.Key, p.Version)
		if err != nil {
			return nil, err
		}
		options = append(options, MetricOption{Paradigm: p, Metrics: metrics})
	}
	return options, nil
}

func (s *Service) design(ctx context.Context, store Store, id string) (domain.Design, error) {
	var d domain.Design
	fills, err := store.Groups(ctx, id)
	if err != nil {
		return d, err
	}
	for _, f := range fills {
		d.Groups = append(d.Groups, f.Group)
	}
	if d.Items, err = store.RegimenItems(ctx, id); err != nil {
		return d, err
	}
	if d.Timepoints, err = store.Timepoints(ctx, id); err != nil {
		return d, err
	}
	if d.Endpoints, err = store.Endpoints(ctx, id); err != nil {
		return d, err
	}
	d.Metrics, err = s.metrics(ctx, store, id)
	return d, err
}

// metrics returns, per paradigm key, the metrics of the paradigm versions the
// experiment's protocol steps run.
func (s *Service) metrics(ctx context.Context, store Store, id string) (map[string]map[string]bool, error) {
	paradigms, err := store.ProtocolParadigms(ctx, id)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	for _, p := range paradigms {
		metrics, err := s.catalog.Metrics(p.Key, p.Version)
		if err != nil {
			return nil, err
		}
		if out[p.Key] == nil {
			out[p.Key] = map[string]bool{}
		}
		for _, m := range metrics {
			out[p.Key][m] = true
		}
	}
	return out, nil
}

// draft locks the experiment and checks its design can still change.
func draft(ctx context.Context, tx Store, id string) error {
	e, err := tx.LockExperiment(ctx, id)
	if err != nil {
		return err
	}
	if e.Status != domain.Draft {
		return ErrDesignFrozen
	}
	return nil
}

func (s *Service) readExperiment(ctx context.Context, actor access.Actor, id string) error {
	if err := actor.Require(access.Read); err != nil {
		return err
	}
	_, err := s.store.Experiment(ctx, id)
	return err
}

func set[T any](dst *T, value *T) {
	if value != nil {
		*dst = *value
	}
}

func descending(order string, fallback bool) (bool, error) {
	switch order {
	case "":
		return fallback, nil
	case "asc":
		return false, nil
	case "desc":
		return true, nil
	}
	return false, ErrInvalidQuery
}

// paging defaults to 10 rows per page and caps the page size at 100.
func paging(page, size int) (current, limit, offset int, err error) {
	if page < 0 || page > 1<<20 || size < 0 {
		return 0, 0, 0, ErrInvalidQuery
	}
	current, limit = max(page, 1), min(size, 100)
	if limit == 0 {
		limit = 10
	}
	return current, limit, (current - 1) * limit, nil
}
