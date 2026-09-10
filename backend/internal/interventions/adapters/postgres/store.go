// Package postgres implements intervention persistence with sqlc-generated queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/interventions/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/interventions/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/interventions/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// constraintErrors maps named constraints to the error a violation means.
var constraintErrors = map[string]error{
	"disease_models_name_key":                  application.ErrNameTaken,
	"substances_name_key":                      application.ErrNameTaken,
	"intervention_plans_experiment_id_fkey":    application.ErrExperimentNotFound,
	"intervention_plans_group_fkey":            application.ErrGroupNotFound,
	"intervention_plans_phase_fkey":            application.ErrPhaseNotFound,
	"intervention_plans_substance_id_fkey":     application.ErrSubstanceNotFound,
	"weight_measurements_subject_id_fkey":      application.ErrSubjectNotFound,
	"subject_conditions_subject_id_fkey":       application.ErrSubjectNotFound,
	"subject_conditions_disease_model_id_fkey": application.ErrDiseaseModelNotFound,
	"subject_conditions_enrollment_fkey":       application.ErrEnrollmentOtherSubject,
	"administrations_enrollment_fkey":          application.ErrEnrollmentNotFound,
	"administrations_subject_fkey":             application.ErrEnrollmentNotFound,
	"administrations_plan_fkey":                application.ErrPlanNotFound,
	"administrations_weight_fkey":              domain.ErrWeightOtherSubject,
	"administrations_substance_id_fkey":        application.ErrSubstanceNotFound,
	"administrations_per_kg_weight_check":      domain.ErrWeightRequired,
}

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool, queries: sqlcgen.New(pool)} }

func (s *Store) Transaction(ctx context.Context, fn func(application.Store) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, queries: s.queries.WithTx(tx)})
	})
}

func (s *Store) DiseaseModels(ctx context.Context) ([]domain.DiseaseModel, error) {
	rows, err := s.queries.ListDiseaseModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list disease models: %w", err)
	}
	return mapRows(rows, toDiseaseModel), nil
}

func (s *Store) DiseaseModel(ctx context.Context, id string) (domain.DiseaseModel, error) {
	if !pgtx.ValidUUID(id) {
		return domain.DiseaseModel{}, application.ErrDiseaseModelNotFound
	}
	row, err := s.queries.GetDiseaseModel(ctx, id)
	if err != nil {
		return domain.DiseaseModel{}, translate(err, application.ErrDiseaseModelNotFound, "get disease model")
	}
	return toDiseaseModel(row), nil
}

func (s *Store) CreateDiseaseModel(ctx context.Context, m domain.DiseaseModel) (domain.DiseaseModel, error) {
	row, err := s.queries.CreateDiseaseModel(ctx, sqlcgen.CreateDiseaseModelParams{Name: m.Name, Description: optional(m.Description)})
	if err != nil {
		return domain.DiseaseModel{}, translate(err, nil, "create disease model")
	}
	return toDiseaseModel(row), nil
}

func (s *Store) UpdateDiseaseModel(ctx context.Context, id string, c application.CatalogChanges) (domain.DiseaseModel, error) {
	if !pgtx.ValidUUID(id) {
		return domain.DiseaseModel{}, application.ErrDiseaseModelNotFound
	}
	row, err := s.queries.UpdateDiseaseModel(ctx, sqlcgen.UpdateDiseaseModelParams{ID: id, Name: c.Name, Description: c.Description})
	if err != nil {
		return domain.DiseaseModel{}, translate(err, application.ErrDiseaseModelNotFound, "update disease model")
	}
	return toDiseaseModel(row), nil
}

func (s *Store) Substances(ctx context.Context) ([]domain.Substance, error) {
	rows, err := s.queries.ListSubstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("list substances: %w", err)
	}
	return mapRows(rows, toSubstance), nil
}

func (s *Store) Substance(ctx context.Context, id string) (domain.Substance, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Substance{}, application.ErrSubstanceNotFound
	}
	row, err := s.queries.GetSubstance(ctx, id)
	if err != nil {
		return domain.Substance{}, translate(err, application.ErrSubstanceNotFound, "get substance")
	}
	return toSubstance(row), nil
}

func (s *Store) CreateSubstance(ctx context.Context, sub domain.Substance) (domain.Substance, error) {
	row, err := s.queries.CreateSubstance(ctx, sqlcgen.CreateSubstanceParams{Name: sub.Name, Description: optional(sub.Description)})
	if err != nil {
		return domain.Substance{}, translate(err, nil, "create substance")
	}
	return toSubstance(row), nil
}

func (s *Store) UpdateSubstance(ctx context.Context, id string, c application.CatalogChanges) (domain.Substance, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Substance{}, application.ErrSubstanceNotFound
	}
	row, err := s.queries.UpdateSubstance(ctx, sqlcgen.UpdateSubstanceParams{ID: id, Name: c.Name, Description: c.Description})
	if err != nil {
		return domain.Substance{}, translate(err, application.ErrSubstanceNotFound, "update substance")
	}
	return toSubstance(row), nil
}

func (s *Store) Plans(ctx context.Context, experimentID string) ([]domain.Plan, error) {
	if !pgtx.ValidUUID(experimentID) {
		return []domain.Plan{}, nil
	}
	rows, err := s.queries.ListPlans(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	return mapRows(rows, toPlan), nil
}

func (s *Store) Plan(ctx context.Context, experimentID, planID string) (domain.Plan, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(planID) {
		return domain.Plan{}, application.ErrPlanNotFound
	}
	row, err := s.queries.GetPlan(ctx, sqlcgen.GetPlanParams{ExperimentID: experimentID, ID: planID})
	if err != nil {
		return domain.Plan{}, translate(err, application.ErrPlanNotFound, "get plan")
	}
	return toPlan(row), nil
}

func (s *Store) CreatePlan(ctx context.Context, p domain.Plan) (domain.Plan, error) {
	switch {
	case !pgtx.ValidUUID(p.ExperimentID):
		return domain.Plan{}, application.ErrExperimentNotFound
	case !pgtx.ValidUUID(p.GroupID):
		return domain.Plan{}, application.ErrGroupNotFound
	case p.PhaseID != "" && !pgtx.ValidUUID(p.PhaseID):
		return domain.Plan{}, application.ErrPhaseNotFound
	case !pgtx.ValidUUID(p.SubstanceID):
		return domain.Plan{}, application.ErrSubstanceNotFound
	}
	row, err := s.queries.CreatePlan(ctx, sqlcgen.CreatePlanParams{
		ExperimentID: p.ExperimentID, GroupID: p.GroupID, PhaseID: optional(p.PhaseID), SubstanceID: p.SubstanceID,
		AmountMicro: p.Dose.Micro, Unit: string(p.Dose.Unit), Route: string(p.Route), Schedule: p.Schedule, Notes: optional(p.Notes),
	})
	if err != nil {
		return domain.Plan{}, translate(err, nil, "create plan")
	}
	return toPlan(row), nil
}

func (s *Store) UpdatePlan(ctx context.Context, experimentID, planID string, c application.PlanChanges) (domain.Plan, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(planID) {
		return domain.Plan{}, application.ErrPlanNotFound
	}
	params := sqlcgen.UpdatePlanParams{ExperimentID: experimentID, ID: planID, Schedule: c.Schedule, Notes: c.Notes}
	if c.Dose != nil {
		unit := string(c.Dose.Unit)
		params.AmountMicro, params.Unit = &c.Dose.Micro, &unit
	}
	if c.Route != nil {
		route := string(*c.Route)
		params.Route = &route
	}
	row, err := s.queries.UpdatePlan(ctx, params)
	if err != nil {
		return domain.Plan{}, translate(err, application.ErrPlanNotFound, "update plan")
	}
	return toPlan(row), nil
}

func (s *Store) CreateWeight(ctx context.Context, w domain.Weight) (domain.Weight, error) {
	if !pgtx.ValidUUID(w.SubjectID) {
		return domain.Weight{}, application.ErrSubjectNotFound
	}
	row, err := s.queries.CreateWeight(ctx, sqlcgen.CreateWeightParams{SubjectID: w.SubjectID, BodyMilligrams: w.Milligrams, MeasuredAt: w.MeasuredAt, RecordedBy: w.RecordedBy})
	if err != nil {
		return domain.Weight{}, translate(err, nil, "create weight")
	}
	return toWeight(row), nil
}

func (s *Store) Weight(ctx context.Context, id string) (domain.Weight, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Weight{}, application.ErrWeightNotFound
	}
	row, err := s.queries.GetWeight(ctx, id)
	if err != nil {
		return domain.Weight{}, translate(err, application.ErrWeightNotFound, "get weight")
	}
	return toWeight(row), nil
}

func (s *Store) Weights(ctx context.Context, subjectID string) ([]domain.Weight, error) {
	if !pgtx.ValidUUID(subjectID) {
		return []domain.Weight{}, nil
	}
	rows, err := s.queries.ListWeights(ctx, subjectID)
	if err != nil {
		return nil, fmt.Errorf("list weights: %w", err)
	}
	return mapRows(rows, toWeight), nil
}

func (s *Store) CreateCondition(ctx context.Context, c domain.Condition) (domain.Condition, error) {
	if !pgtx.ValidUUID(c.SubjectID) {
		return domain.Condition{}, application.ErrSubjectNotFound
	}
	if c.EnrollmentID != "" && !pgtx.ValidUUID(c.EnrollmentID) {
		return domain.Condition{}, application.ErrEnrollmentOtherSubject
	}
	row, err := s.queries.CreateCondition(ctx, sqlcgen.CreateConditionParams{
		SubjectID: c.SubjectID, DiseaseModelID: c.DiseaseModelID, DiseaseModelName: c.DiseaseModelName, EnrollmentID: optional(c.EnrollmentID),
		Status: string(c.Status), ObservedAt: c.ObservedAt, Notes: optional(c.Notes), RecordedBy: c.RecordedBy,
	})
	if err != nil {
		return domain.Condition{}, translate(err, nil, "create condition")
	}
	return toCondition(row), nil
}

func (s *Store) SubjectConditions(ctx context.Context, subjectID, diseaseModelID string) ([]domain.Condition, error) {
	if !pgtx.ValidUUID(subjectID) {
		return []domain.Condition{}, nil
	}
	rows, err := s.queries.ListSubjectConditions(ctx, sqlcgen.ListSubjectConditionsParams{SubjectID: subjectID, DiseaseModelID: diseaseModelID})
	if err != nil {
		return nil, fmt.Errorf("list subject conditions: %w", err)
	}
	return mapRows(rows, toCondition), nil
}

func (s *Store) ListConditions(ctx context.Context, f application.ConditionFilter) ([]domain.Condition, int, error) {
	rows, err := s.queries.ListConditions(ctx, sqlcgen.ListConditionsParams{
		DiseaseModelID: f.DiseaseModelID, Status: string(f.Status), CurrentOnly: f.CurrentOnly, PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list conditions: %w", err)
	}
	total, err := s.queries.CountConditions(ctx, sqlcgen.CountConditionsParams{DiseaseModelID: f.DiseaseModelID, Status: string(f.Status), CurrentOnly: f.CurrentOnly})
	if err != nil {
		return nil, 0, fmt.Errorf("count conditions: %w", err)
	}
	return mapRows(rows, toCondition), int(total), nil
}

func (s *Store) Enrollment(ctx context.Context, experimentID, enrollmentID string) (application.EnrollmentRef, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(enrollmentID) {
		return application.EnrollmentRef{}, application.ErrEnrollmentNotFound
	}
	row, err := s.queries.GetEnrollmentRef(ctx, sqlcgen.GetEnrollmentRefParams{ExperimentID: experimentID, ID: enrollmentID})
	if err != nil {
		return application.EnrollmentRef{}, translate(err, application.ErrEnrollmentNotFound, "get enrollment")
	}
	return application.EnrollmentRef{ID: row.ID, ExperimentID: row.ExperimentID, SubjectID: row.SubjectID, EnrolledAt: row.EnrolledAt.UTC()}, nil
}

func (s *Store) GroupAt(ctx context.Context, enrollmentID string, t time.Time) (string, error) {
	group, err := s.queries.GroupAt(ctx, sqlcgen.GroupAtParams{EnrollmentID: enrollmentID, At: t})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("group at: %w", err)
	}
	return group, nil
}

func (s *Store) CreateAdministration(ctx context.Context, a domain.Administration) (domain.Administration, error) {
	params := sqlcgen.CreateAdministrationParams{
		ExperimentID: a.ExperimentID, EnrollmentID: a.EnrollmentID, SubjectID: a.SubjectID, SubstanceID: a.SubstanceID, SubstanceName: a.SubstanceName,
		PlanID: optional(a.PlanID), WeightMeasurementID: optional(a.WeightID), AmountMicro: a.Dose.Micro, Unit: string(a.Dose.Unit),
		Route: string(a.Route), AdministeredAt: a.AdministeredAt, Notes: optional(a.Notes), RecordedBy: a.RecordedBy,
	}
	if a.WeightID != "" {
		params.BodyMilligrams = &a.BodyMilligrams
	}
	row, err := s.queries.CreateAdministration(ctx, params)
	if err != nil {
		return domain.Administration{}, translate(err, nil, "create administration")
	}
	return toAdministration(row), nil
}

func (s *Store) ListAdministrations(ctx context.Context, f application.AdministrationFilter) ([]domain.Administration, int, error) {
	rows, err := s.queries.ListAdministrations(ctx, sqlcgen.ListAdministrationsParams{
		SubjectID: f.SubjectID, SubstanceID: f.SubstanceID, ExperimentID: f.ExperimentID, FromTime: f.From, ToTime: f.To,
		PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list administrations: %w", err)
	}
	total, err := s.queries.CountAdministrations(ctx, sqlcgen.CountAdministrationsParams{
		SubjectID: f.SubjectID, SubstanceID: f.SubstanceID, ExperimentID: f.ExperimentID, FromTime: f.From, ToTime: f.To,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count administrations: %w", err)
	}
	return mapRows(rows, toAdministration), int(total), nil
}

// translate maps a missing row to notFound and named constraint violations to
// their errors. Other errors are wrapped so PostgreSQL codes stay inspectable.
func translate(err, notFound error, operation string) error {
	if notFound != nil && errors.Is(err, pgx.ErrNoRows) {
		return notFound
	}
	if _, constraint := pgtx.Violation(err); constraintErrors[constraint] != nil {
		return constraintErrors[constraint]
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func toDiseaseModel(r sqlcgen.MiskoDiseaseModel) domain.DiseaseModel {
	return domain.DiseaseModel{ID: r.ID, Name: r.Name, Description: value(r.Description), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func toSubstance(r sqlcgen.MiskoSubstance) domain.Substance {
	return domain.Substance{ID: r.ID, Name: r.Name, Description: value(r.Description), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func toPlan(r sqlcgen.MiskoInterventionPlan) domain.Plan {
	return domain.Plan{
		ID: r.ID, ExperimentID: r.ExperimentID, GroupID: r.GroupID, PhaseID: value(r.PhaseID), SubstanceID: r.SubstanceID,
		Dose: domain.Dose{Micro: r.AmountMicro, Unit: domain.Unit(r.Unit)}, Route: domain.Route(r.Route),
		Schedule: r.Schedule, Notes: value(r.Notes), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func toWeight(r sqlcgen.MiskoWeightMeasurement) domain.Weight {
	return domain.Weight{ID: r.ID, SubjectID: r.SubjectID, Milligrams: r.BodyMilligrams, MeasuredAt: r.MeasuredAt.UTC(), RecordedBy: r.RecordedBy, CreatedAt: r.CreatedAt.UTC()}
}

func toCondition(r sqlcgen.MiskoSubjectCondition) domain.Condition {
	return domain.Condition{
		ID: r.ID, SubjectID: r.SubjectID, DiseaseModelID: r.DiseaseModelID, DiseaseModelName: r.DiseaseModelName, EnrollmentID: value(r.EnrollmentID),
		Status: domain.ConditionStatus(r.Status), ObservedAt: r.ObservedAt.UTC(), Notes: value(r.Notes), RecordedBy: r.RecordedBy, CreatedAt: r.CreatedAt.UTC(),
	}
}

func toAdministration(r sqlcgen.MiskoAdministration) domain.Administration {
	a := domain.Administration{
		ID: r.ID, ExperimentID: r.ExperimentID, EnrollmentID: r.EnrollmentID, SubjectID: r.SubjectID,
		SubstanceID: r.SubstanceID, SubstanceName: r.SubstanceName, PlanID: value(r.PlanID), WeightID: value(r.WeightMeasurementID),
		Dose: domain.Dose{Micro: r.AmountMicro, Unit: domain.Unit(r.Unit)}, Route: domain.Route(r.Route),
		AdministeredAt: r.AdministeredAt.UTC(), Notes: value(r.Notes), RecordedBy: r.RecordedBy, CreatedAt: r.CreatedAt.UTC(),
	}
	if r.BodyMilligrams != nil {
		a.BodyMilligrams = *r.BodyMilligrams
	}
	return a
}

func mapRows[T, U any](rows []T, convert func(T) U) []U {
	out := make([]U, 0, len(rows))
	for _, row := range rows {
		out = append(out, convert(row))
	}
	return out
}

// optional stores an empty value as NULL.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
