// Package application contains intervention use cases. Catalog and plan changes
// need study:write, weights need weight:write, and actual conditions and
// administrations need test:run. Reads need *:read.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/interventions/domain"
	"slices"
	"time"
)

var (
	ErrDiseaseModelNotFound   = errors.New("disease model not found")
	ErrSubstanceNotFound      = errors.New("substance not found")
	ErrPlanNotFound           = errors.New("intervention plan not found in this experiment")
	ErrWeightNotFound         = errors.New("weight measurement not found")
	ErrEnrollmentNotFound     = errors.New("enrollment not found in this experiment")
	ErrEnrollmentOtherSubject = errors.New("enrollment belongs to another subject")
	ErrSubjectNotFound        = errors.New("subject not found")
	ErrExperimentNotFound     = errors.New("experiment not found")
	ErrGroupNotFound          = errors.New("group not found in this experiment")
	ErrPhaseNotFound          = errors.New("phase not found in this experiment")
	ErrNameTaken              = errors.New("name is already in use")
	ErrPlanGroupMismatch      = errors.New("subject is not assigned to the plan's group at administration time")
	ErrPlanSubstanceMismatch  = errors.New("administration substance differs from the plan's substance")
	ErrDoseIncomplete         = errors.New("amount and unit must be changed together")
	ErrNoChanges              = errors.New("no fields to update")
	ErrInvalidQuery           = errors.New("invalid list query")
)

// Changes types hold validated fields; nil leaves a field unchanged and empty
// optional text clears it.
type CatalogChanges struct {
	Name, Description *string
}

type PlanChanges struct {
	Dose            *domain.Dose
	Route           *domain.Route
	Schedule, Notes *string
}

// EnrollmentRef is the part of an enrollment needed to validate a record.
type EnrollmentRef struct {
	ID, ExperimentID, SubjectID string
	EnrolledAt                  time.Time
}

type ConditionFilter struct {
	DiseaseModelID string
	Status         domain.ConditionStatus
	CurrentOnly    bool
	Limit, Offset  int
}

type AdministrationFilter struct {
	SubjectID, SubstanceID, ExperimentID string
	From, To                             *time.Time
	Limit, Offset                        int
}

// Store persists intervention records. Scoped lookups return the matching
// not-found error for records of another experiment.
type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	DiseaseModels(ctx context.Context) ([]domain.DiseaseModel, error)
	DiseaseModel(ctx context.Context, id string) (domain.DiseaseModel, error)
	CreateDiseaseModel(ctx context.Context, m domain.DiseaseModel) (domain.DiseaseModel, error)
	UpdateDiseaseModel(ctx context.Context, id string, c CatalogChanges) (domain.DiseaseModel, error)
	Substances(ctx context.Context) ([]domain.Substance, error)
	Substance(ctx context.Context, id string) (domain.Substance, error)
	CreateSubstance(ctx context.Context, s domain.Substance) (domain.Substance, error)
	UpdateSubstance(ctx context.Context, id string, c CatalogChanges) (domain.Substance, error)
	Plans(ctx context.Context, experimentID string) ([]domain.Plan, error)
	Plan(ctx context.Context, experimentID, planID string) (domain.Plan, error)
	CreatePlan(ctx context.Context, p domain.Plan) (domain.Plan, error)
	UpdatePlan(ctx context.Context, experimentID, planID string, c PlanChanges) (domain.Plan, error)
	CreateWeight(ctx context.Context, w domain.Weight) (domain.Weight, error)
	Weight(ctx context.Context, id string) (domain.Weight, error)
	Weights(ctx context.Context, subjectID string) ([]domain.Weight, error)
	CreateCondition(ctx context.Context, c domain.Condition) (domain.Condition, error)
	SubjectConditions(ctx context.Context, subjectID, diseaseModelID string) ([]domain.Condition, error)
	ListConditions(ctx context.Context, f ConditionFilter) ([]domain.Condition, int, error)
	Enrollment(ctx context.Context, experimentID, enrollmentID string) (EnrollmentRef, error)
	// GroupAt returns the group an enrollment was assigned to at t, or "".
	GroupAt(ctx context.Context, enrollmentID string, t time.Time) (string, error)
	CreateAdministration(ctx context.Context, a domain.Administration) (domain.Administration, error)
	ListAdministrations(ctx context.Context, f AdministrationFilter) ([]domain.Administration, int, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func New(store Store, now func() time.Time) *Service { return &Service{store: store, now: now} }

type CatalogInput struct {
	Name, Description string
}

type CatalogPatch struct {
	Name, Description *string
}

func (s *Service) DiseaseModels(ctx context.Context, actor access.Actor) ([]domain.DiseaseModel, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	return s.store.DiseaseModels(ctx)
}

func (s *Service) CreateDiseaseModel(ctx context.Context, actor access.Actor, in CatalogInput) (domain.DiseaseModel, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.DiseaseModel{}, err
	}
	m, err := domain.NewDiseaseModel(in.Name, in.Description)
	if err != nil {
		return domain.DiseaseModel{}, err
	}
	return s.store.CreateDiseaseModel(ctx, m)
}

// UpdateDiseaseModel changes the current definition; recorded conditions keep
// the name they were recorded with.
func (s *Service) UpdateDiseaseModel(ctx context.Context, actor access.Actor, id string, p CatalogPatch) (domain.DiseaseModel, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.DiseaseModel{}, err
	}
	c, err := catalogChanges(p)
	if err != nil {
		return domain.DiseaseModel{}, err
	}
	return s.store.UpdateDiseaseModel(ctx, id, c)
}

func (s *Service) Substances(ctx context.Context, actor access.Actor) ([]domain.Substance, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	return s.store.Substances(ctx)
}

func (s *Service) CreateSubstance(ctx context.Context, actor access.Actor, in CatalogInput) (domain.Substance, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Substance{}, err
	}
	sub, err := domain.NewSubstance(in.Name, in.Description)
	if err != nil {
		return domain.Substance{}, err
	}
	return s.store.CreateSubstance(ctx, sub)
}

// UpdateSubstance changes the current definition; recorded administrations keep
// the name they were recorded with.
func (s *Service) UpdateSubstance(ctx context.Context, actor access.Actor, id string, p CatalogPatch) (domain.Substance, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Substance{}, err
	}
	c, err := catalogChanges(p)
	if err != nil {
		return domain.Substance{}, err
	}
	return s.store.UpdateSubstance(ctx, id, c)
}

func catalogChanges(p CatalogPatch) (CatalogChanges, error) {
	if p == (CatalogPatch{}) {
		return CatalogChanges{}, ErrNoChanges
	}
	var c CatalogChanges
	var err error
	if c.Name, err = optional(p.Name, domain.NormalizeName); err != nil {
		return CatalogChanges{}, err
	}
	if c.Description, err = optional(p.Description, domain.NormalizeDescription); err != nil {
		return CatalogChanges{}, err
	}
	return c, nil
}

func (s *Service) Plans(ctx context.Context, actor access.Actor, experimentID string) ([]domain.Plan, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	return s.store.Plans(ctx, experimentID)
}

type PlanInput struct {
	GroupID, PhaseID, SubstanceID string
	Amount, Unit, Route           string
	Schedule, Notes               string
}

// CreatePlan records what a group is meant to receive. It creates no administration.
func (s *Service) CreatePlan(ctx context.Context, actor access.Actor, experimentID string, in PlanInput) (domain.Plan, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Plan{}, err
	}
	dose, err := domain.ParseDose(in.Amount, in.Unit)
	if err != nil {
		return domain.Plan{}, err
	}
	route, err := domain.ParseRoute(in.Route)
	if err != nil {
		return domain.Plan{}, err
	}
	plan, err := domain.NewPlan(experimentID, in.GroupID, in.PhaseID, in.SubstanceID, dose, route, in.Schedule, in.Notes)
	if err != nil {
		return domain.Plan{}, err
	}
	return s.store.CreatePlan(ctx, plan)
}

type PlanPatch struct {
	Amount, Unit, Route, Schedule, Notes *string
}

// UpdatePlan changes a plan; administrations already recorded keep their own values.
func (s *Service) UpdatePlan(ctx context.Context, actor access.Actor, experimentID, planID string, p PlanPatch) (domain.Plan, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Plan{}, err
	}
	if p == (PlanPatch{}) {
		return domain.Plan{}, ErrNoChanges
	}
	if (p.Amount == nil) != (p.Unit == nil) {
		return domain.Plan{}, ErrDoseIncomplete
	}
	var c PlanChanges
	var err error
	if p.Amount != nil {
		dose, err := domain.ParseDose(*p.Amount, *p.Unit)
		if err != nil {
			return domain.Plan{}, err
		}
		c.Dose = &dose
	}
	if c.Route, err = optional(p.Route, domain.ParseRoute); err != nil {
		return domain.Plan{}, err
	}
	if c.Schedule, err = optional(p.Schedule, domain.NormalizeSchedule); err != nil {
		return domain.Plan{}, err
	}
	if c.Notes, err = optional(p.Notes, domain.NormalizeNotes); err != nil {
		return domain.Plan{}, err
	}
	return s.store.UpdatePlan(ctx, experimentID, planID, c)
}

type WeightInput struct {
	Grams      string
	MeasuredAt time.Time
}

func (s *Service) RecordWeight(ctx context.Context, actor access.Actor, subjectID string, in WeightInput) (domain.Weight, error) {
	if err := actor.Require(access.WeightWrite); err != nil {
		return domain.Weight{}, err
	}
	w, err := domain.NewWeight(subjectID, in.Grams, instant(in.MeasuredAt), s.now(), actor.UserID)
	if err != nil {
		return domain.Weight{}, err
	}
	return s.store.CreateWeight(ctx, w)
}

func (s *Service) Weights(ctx context.Context, actor access.Actor, subjectID string) ([]domain.Weight, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	return s.store.Weights(ctx, subjectID)
}

type ConditionInput struct {
	DiseaseModelID, Status, EnrollmentID, Notes string
	ObservedAt                                  time.Time
}

// RecordCondition appends an observation. Induction and confirmation are
// separate observations; neither is inferred from the other.
func (s *Service) RecordCondition(ctx context.Context, actor access.Actor, subjectID string, in ConditionInput) (domain.Condition, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Condition{}, err
	}
	c, err := domain.NewCondition(subjectID, in.DiseaseModelID, in.Status, instant(in.ObservedAt), s.now(), in.EnrollmentID, in.Notes, actor.UserID)
	if err != nil {
		return domain.Condition{}, err
	}
	model, err := s.store.DiseaseModel(ctx, in.DiseaseModelID)
	if err != nil {
		return domain.Condition{}, err
	}
	c.DiseaseModelName = model.Name
	return s.store.CreateCondition(ctx, c)
}

// ConditionHistory lists observations newest first and the latest per model.
type ConditionHistory struct {
	History, Current []domain.Condition
}

func (s *Service) SubjectConditions(ctx context.Context, actor access.Actor, subjectID, diseaseModelID string) (ConditionHistory, error) {
	if err := actor.Require(access.Read); err != nil {
		return ConditionHistory{}, err
	}
	history, err := s.store.SubjectConditions(ctx, subjectID, diseaseModelID)
	if err != nil {
		return ConditionHistory{}, err
	}
	current := make([]domain.Condition, 0)
	for _, c := range domain.CurrentConditions(history) {
		current = append(current, c)
	}
	slices.SortFunc(current, func(a, b domain.Condition) int { return b.ObservedAt.Compare(a.ObservedAt) })
	return ConditionHistory{History: history, Current: current}, nil
}

type ConditionQuery struct {
	DiseaseModelID, Status string
	// CurrentOnly keeps only the latest observation per subject and model.
	CurrentOnly    bool
	Page, PageSize int
}

type ConditionPage struct {
	Conditions            []domain.Condition
	Total, Page, PageSize int
}

func (s *Service) ListConditions(ctx context.Context, actor access.Actor, q ConditionQuery) (ConditionPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return ConditionPage{}, err
	}
	f := ConditionFilter{DiseaseModelID: q.DiseaseModelID, CurrentOnly: q.CurrentOnly}
	if q.Status != "" {
		status, err := domain.ParseStatus(q.Status)
		if err != nil {
			return ConditionPage{}, ErrInvalidQuery
		}
		f.Status = status
	}
	page, limit, offset, err := paging(q.Page, q.PageSize)
	if err != nil {
		return ConditionPage{}, err
	}
	f.Limit, f.Offset = limit, offset
	conditions, total, err := s.store.ListConditions(ctx, f)
	if err != nil {
		return ConditionPage{}, err
	}
	return ConditionPage{Conditions: conditions, Total: total, Page: page, PageSize: limit}, nil
}

type AdministrationInput struct {
	SubstanceID, PlanID, WeightID string
	Amount, Unit, Route, Notes    string
	AdministeredAt                time.Time
}

// RecordAdministration stores an actual administration for an enrollment. The
// subject comes from the enrollment; a plan, if given, must match the
// substance and the subject's group at that time.
func (s *Service) RecordAdministration(ctx context.Context, actor access.Actor, experimentID, enrollmentID string, in AdministrationInput) (domain.Administration, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Administration{}, err
	}
	dose, err := domain.ParseDose(in.Amount, in.Unit)
	if err != nil {
		return domain.Administration{}, err
	}
	route, err := domain.ParseRoute(in.Route)
	if err != nil {
		return domain.Administration{}, err
	}
	notes, err := domain.NormalizeNotes(in.Notes)
	if err != nil {
		return domain.Administration{}, err
	}
	at := instant(in.AdministeredAt)
	switch {
	case at.IsZero():
		return domain.Administration{}, domain.ErrMissingTime
	case at.After(s.now().Add(domain.ClockSkew)):
		return domain.Administration{}, domain.ErrFutureTime
	}
	var created domain.Administration
	err = s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.Enrollment(ctx, experimentID, enrollmentID)
		if err != nil {
			return err
		}
		substance, err := tx.Substance(ctx, in.SubstanceID)
		if err != nil {
			return err
		}
		a := domain.Administration{
			ExperimentID: e.ExperimentID, EnrollmentID: e.ID, SubjectID: e.SubjectID,
			SubstanceID: substance.ID, SubstanceName: substance.Name, PlanID: in.PlanID, WeightID: in.WeightID,
			Dose: dose, Route: route, AdministeredAt: at, Notes: notes, RecordedBy: actor.UserID,
		}
		var weight *domain.Weight
		if in.WeightID != "" {
			w, err := tx.Weight(ctx, in.WeightID)
			if err != nil {
				return err
			}
			weight, a.BodyMilligrams = &w, w.Milligrams
		}
		if err := domain.CheckAdministration(a, e.EnrolledAt, weight, s.now()); err != nil {
			return err
		}
		if in.PlanID != "" {
			plan, err := tx.Plan(ctx, e.ExperimentID, in.PlanID)
			if err != nil {
				return err
			}
			if plan.SubstanceID != substance.ID {
				return ErrPlanSubstanceMismatch
			}
			group, err := tx.GroupAt(ctx, e.ID, at)
			if err != nil {
				return err
			}
			if group != plan.GroupID {
				return ErrPlanGroupMismatch
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

// AdministrationQuery filters by substance, experiment and a [From, To) range.
type AdministrationQuery struct {
	SubstanceID, ExperimentID string
	From, To                  time.Time
	Page, PageSize            int
}

type AdministrationPage struct {
	Administrations       []domain.Administration
	Total, Page, PageSize int
}

func (s *Service) SubjectAdministrations(ctx context.Context, actor access.Actor, subjectID string, q AdministrationQuery) (AdministrationPage, error) {
	return s.administrations(ctx, actor, subjectID, q)
}

func (s *Service) ListAdministrations(ctx context.Context, actor access.Actor, q AdministrationQuery) (AdministrationPage, error) {
	return s.administrations(ctx, actor, "", q)
}

func (s *Service) administrations(ctx context.Context, actor access.Actor, subjectID string, q AdministrationQuery) (AdministrationPage, error) {
	if err := actor.Require(access.Read); err != nil {
		return AdministrationPage{}, err
	}
	if !q.From.IsZero() && !q.To.IsZero() && !q.To.After(q.From) {
		return AdministrationPage{}, ErrInvalidQuery
	}
	page, limit, offset, err := paging(q.Page, q.PageSize)
	if err != nil {
		return AdministrationPage{}, err
	}
	f := AdministrationFilter{SubjectID: subjectID, SubstanceID: q.SubstanceID, ExperimentID: q.ExperimentID, Limit: limit, Offset: offset}
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

// instant stores times in UTC at the database's microsecond precision.
func instant(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC().Truncate(time.Microsecond)
}

func optional[T any](raw *string, parse func(string) (T, error)) (*T, error) {
	if raw == nil {
		return nil, nil
	}
	value, err := parse(*raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
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
