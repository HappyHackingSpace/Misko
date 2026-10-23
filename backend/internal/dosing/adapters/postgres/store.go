// Package postgres implements dosing persistence with sqlc-generated queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/dosing/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/dosing/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/dosing/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// constraintErrors maps named constraints and trigger rules of
// administrations and induction_outcomes to the error a violation means.
var constraintErrors = map[string]error{
	"administrations_not_running":         application.ErrExperimentNotRunning,
	"induction_outcomes_not_running":      application.ErrExperimentNotRunning,
	"administrations_allocation_fkey":     application.ErrAllocationNotFound,
	"administrations_group_fkey":          application.ErrAllocationNotFound,
	"administrations_subject_fkey":        application.ErrAllocationNotFound,
	"administrations_regimen_fkey":        application.ErrRegimenItemOtherGroup,
	"administrations_no_substance":        application.ErrNoSubstanceItem,
	"administrations_weight_fkey":         domain.ErrWeightOtherSubject,
	"administrations_per_kg_weight_check": domain.ErrWeightRequired,
	"administrations_substance_id_fkey":   application.ErrSubstanceNotFound,
	"induction_outcomes_pkey":             application.ErrOutcomeExists,
	"induction_outcomes_allocation_fkey":  application.ErrAllocationNotFound,
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

func (s *Store) Allocation(ctx context.Context, experimentID, allocationID string) (application.AllocationRef, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(allocationID) {
		return application.AllocationRef{}, application.ErrAllocationNotFound
	}
	row, err := s.queries.GetAllocationRef(ctx, sqlcgen.GetAllocationRefParams{ExperimentID: experimentID, ID: allocationID})
	if err != nil {
		return application.AllocationRef{}, translate(err, application.ErrAllocationNotFound, "get allocation")
	}
	return application.AllocationRef{
		Allocation:       allocation(row.ID, row.ExperimentID, row.GroupID, row.SubjectID, row.AllocatedAt, row.ExcludedAt),
		ExperimentStatus: row.ExperimentStatus, HasDisease: row.HasDisease,
	}, nil
}

func (s *Store) RegimenItem(ctx context.Context, experimentID, itemID string) (domain.RegimenItem, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(itemID) {
		return domain.RegimenItem{}, application.ErrRegimenItemNotFound
	}
	r, err := s.queries.GetRegimenItem(ctx, sqlcgen.GetRegimenItemParams{ExperimentID: experimentID, ID: itemID})
	if err != nil {
		return domain.RegimenItem{}, translate(err, application.ErrRegimenItemNotFound, "get regimen item")
	}
	return domain.RegimenItem{
		ID: r.ID, ExperimentID: r.ExperimentID, GroupID: r.GroupID, Purpose: domain.Purpose(r.Purpose),
		SubstanceID: value(r.SubstanceID), SubstanceName: value(r.SubstanceName), Dose: domain.Dose{Micro: micro(r.DoseMicro), Unit: domain.Unit(value(r.Unit))},
		Route: domain.Route(value(r.Route)), Frequency: domain.Frequency(value(r.Frequency)), IntervalDays: interval(r.IntervalDays),
		StartDay: int(r.StartDay), EndDay: int(r.EndDay),
	}, nil
}

func (s *Store) SubstanceName(ctx context.Context, id string) (string, error) {
	if !pgtx.ValidUUID(id) {
		return "", application.ErrSubstanceNotFound
	}
	name, err := s.queries.GetSubstanceName(ctx, id)
	if err != nil {
		return "", translate(err, application.ErrSubstanceNotFound, "get substance")
	}
	return name, nil
}

func (s *Store) Weight(ctx context.Context, id string) (domain.Weight, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Weight{}, application.ErrWeightNotFound
	}
	r, err := s.queries.GetWeight(ctx, id)
	if err != nil {
		return domain.Weight{}, translate(err, application.ErrWeightNotFound, "get weight")
	}
	return domain.Weight{ID: r.ID, SubjectID: r.SubjectID, Milligrams: r.BodyMilligrams, MeasuredAt: r.MeasuredAt.UTC()}, nil
}

func (s *Store) LatestWeight(ctx context.Context, subjectID string, at time.Time) (*domain.Weight, error) {
	r, err := s.queries.LatestWeight(ctx, sqlcgen.LatestWeightParams{SubjectID: subjectID, At: at})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest weight: %w", err)
	}
	return &domain.Weight{ID: r.ID, SubjectID: r.SubjectID, Milligrams: r.BodyMilligrams, MeasuredAt: r.MeasuredAt.UTC()}, nil
}

func (s *Store) CreateAdministration(ctx context.Context, a domain.Administration) (domain.Administration, error) {
	if !pgtx.ValidUUID(a.SubstanceID) {
		return domain.Administration{}, application.ErrSubstanceNotFound
	}
	row, err := s.queries.CreateAdministration(ctx, sqlcgen.CreateAdministrationParams{
		ExperimentID: a.ExperimentID, AllocationID: a.AllocationID, GroupID: a.GroupID, SubjectID: a.SubjectID,
		RegimenItemID: optional(a.RegimenItemID), Purpose: string(a.Purpose), SubstanceID: a.SubstanceID,
		DoseMicro: a.Dose.Micro, Unit: string(a.Dose.Unit), Route: string(a.Route), WeightMeasurementID: optional(a.WeightID),
		AdministeredAt: a.AdministeredAt, Notes: optional(a.Notes), RecordedBy: a.RecordedBy,
	})
	if err != nil {
		return domain.Administration{}, translate(err, nil, "create administration")
	}
	a.ID, a.CreatedAt = row.ID, row.CreatedAt.UTC()
	return a, nil
}

func (s *Store) ListAdministrations(ctx context.Context, f application.AdministrationFilter) ([]domain.Administration, int, error) {
	for _, id := range []string{f.ExperimentID, f.AllocationID, f.SubjectID, f.SubstanceID} {
		if id != "" && !pgtx.ValidUUID(id) {
			return []domain.Administration{}, 0, nil
		}
	}
	rows, err := s.queries.ListAdministrations(ctx, sqlcgen.ListAdministrationsParams{
		ExperimentID: f.ExperimentID, AllocationID: f.AllocationID, SubjectID: f.SubjectID, SubstanceID: f.SubstanceID,
		Purpose: string(f.Purpose), FromTime: f.From, ToTime: f.To, PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list administrations: %w", err)
	}
	total, err := s.queries.CountAdministrations(ctx, sqlcgen.CountAdministrationsParams{
		ExperimentID: f.ExperimentID, AllocationID: f.AllocationID, SubjectID: f.SubjectID, SubstanceID: f.SubstanceID,
		Purpose: string(f.Purpose), FromTime: f.From, ToTime: f.To,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count administrations: %w", err)
	}
	return mapRows(rows, toAdministration), int(total), nil
}

func (s *Store) ExperimentDay(ctx context.Context, experimentID string) (application.ExperimentDay, error) {
	if !pgtx.ValidUUID(experimentID) {
		return application.ExperimentDay{}, application.ErrExperimentNotFound
	}
	row, err := s.queries.GetExperimentDay(ctx, experimentID)
	if err != nil {
		return application.ExperimentDay{}, translate(err, application.ErrExperimentNotFound, "get experiment day")
	}
	today, err := domain.ParseDate(row.Today)
	if err != nil {
		return application.ExperimentDay{}, fmt.Errorf("laboratory date %q: %w", row.Today, err)
	}
	out := application.ExperimentDay{Status: row.Status, Today: today}
	if row.DayZero != "" {
		dayZero, err := domain.ParseDate(row.DayZero)
		if err != nil {
			return application.ExperimentDay{}, fmt.Errorf("day 0 %q: %w", row.DayZero, err)
		}
		out.DayZero = &dayZero
	}
	return out, nil
}

func (s *Store) DueCandidates(ctx context.Context, experimentID string, day time.Time) ([]domain.DueCandidate, error) {
	rows, err := s.queries.ListDueCandidates(ctx, sqlcgen.ListDueCandidatesParams{ExperimentID: experimentID, Day: day.Format(time.DateOnly)})
	if err != nil {
		return nil, fmt.Errorf("list due doses: %w", err)
	}
	return mapRows(rows, func(r sqlcgen.ListDueCandidatesRow) domain.DueCandidate {
		c := domain.DueCandidate{
			Allocation:  allocation(r.AllocationID, r.ExperimentID, r.GroupID, r.SubjectID, r.AllocatedAt, r.ExcludedAt),
			SubjectCode: r.SubjectCode, GroupCode: r.GroupCode, Cage: value(r.Cage), Excluded: r.Excluded,
			Item: domain.RegimenItem{
				ID: r.RegimenItemID, ExperimentID: r.ExperimentID, GroupID: r.GroupID, Purpose: domain.Purpose(r.Purpose),
				SubstanceID: value(r.SubstanceID), SubstanceName: r.SubstanceName, Dose: domain.Dose{Micro: micro(r.DoseMicro), Unit: domain.Unit(value(r.Unit))},
				Route: domain.Route(value(r.Route)), Frequency: domain.Frequency(value(r.Frequency)), IntervalDays: interval(r.IntervalDays),
				StartDay: int(r.StartDay), EndDay: int(r.EndDay),
			},
			Given: int(r.Given),
		}
		if r.WeightID != "" {
			c.Weight = &domain.Weight{ID: r.WeightID, SubjectID: r.SubjectID, Milligrams: r.WeightMilligrams, MeasuredAt: r.WeightMeasuredAt.UTC()}
		}
		return c
	}), nil
}

func (s *Store) CreateInductionOutcome(ctx context.Context, o domain.InductionOutcome) (domain.InductionOutcome, error) {
	r, err := s.queries.CreateInductionOutcome(ctx, sqlcgen.CreateInductionOutcomeParams{
		AllocationID: o.AllocationID, ExperimentID: o.ExperimentID, Status: string(o.Status), AssessedAt: o.AssessedAt,
		Note: optional(o.Note), RecordedBy: o.RecordedBy,
	})
	if err != nil {
		return domain.InductionOutcome{}, translate(err, nil, "create induction outcome")
	}
	return domain.InductionOutcome{
		AllocationID: r.AllocationID, ExperimentID: r.ExperimentID, Status: domain.OutcomeStatus(r.Status), AssessedAt: r.AssessedAt.UTC(),
		Note: value(r.Note), RecordedBy: r.RecordedBy, CreatedAt: r.CreatedAt.UTC(),
	}, nil
}

func (s *Store) ListInductionOutcomes(ctx context.Context, experimentID string, f application.OutcomeFilter) ([]application.OutcomeRow, error) {
	if !pgtx.ValidUUID(experimentID) {
		return []application.OutcomeRow{}, nil
	}
	rows, err := s.queries.ListInductionOutcomes(ctx, sqlcgen.ListInductionOutcomesParams{ExperimentID: experimentID, Status: string(f.Status)})
	if err != nil {
		return nil, fmt.Errorf("list induction outcomes: %w", err)
	}
	return mapRows(rows, func(r sqlcgen.ListInductionOutcomesRow) application.OutcomeRow {
		return application.OutcomeRow{
			InductionOutcome: domain.InductionOutcome{
				AllocationID: r.AllocationID, ExperimentID: r.ExperimentID, Status: domain.OutcomeStatus(r.Status), AssessedAt: r.AssessedAt.UTC(),
				Note: value(r.Note), RecordedBy: r.RecordedBy, CreatedAt: r.CreatedAt.UTC(),
			},
			SubjectID: r.SubjectID, SubjectCode: r.SubjectCode, GroupID: r.GroupID, GroupCode: r.GroupCode,
		}
	}), nil
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

func allocation(id, experimentID, groupID, subjectID string, allocatedAt time.Time, excludedAt *time.Time) domain.Allocation {
	a := domain.Allocation{ID: id, ExperimentID: experimentID, GroupID: groupID, SubjectID: subjectID, AllocatedAt: allocatedAt.UTC()}
	if excludedAt != nil {
		t := excludedAt.UTC()
		a.ExcludedAt = &t
	}
	return a
}

func toAdministration(r sqlcgen.ListAdministrationsRow) domain.Administration {
	a := domain.Administration{
		ID: r.ID, ExperimentID: r.ExperimentID, AllocationID: r.AllocationID, GroupID: r.GroupID, SubjectID: r.SubjectID,
		RegimenItemID: value(r.RegimenItemID), Purpose: domain.Purpose(r.Purpose), SubstanceID: r.SubstanceID, SubstanceName: r.SubstanceName,
		Dose: domain.Dose{Micro: r.DoseMicro, Unit: domain.Unit(r.Unit)}, Route: domain.Route(r.Route), WeightID: value(r.WeightMeasurementID),
		AdministeredAt: r.AdministeredAt.UTC(), Notes: value(r.Notes), RecordedBy: r.RecordedBy, CreatedAt: r.CreatedAt.UTC(),
	}
	if r.BodyMilligrams != nil {
		a.BodyMilligrams = *r.BodyMilligrams
	}
	return a
}

func interval(days *int32) int {
	if days == nil {
		return 0
	}
	return int(*days)
}

// micro reads a dose that a no-substance period leaves NULL as 0.
func micro(dose *int64) int64 {
	if dose == nil {
		return 0
	}
	return *dose
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
