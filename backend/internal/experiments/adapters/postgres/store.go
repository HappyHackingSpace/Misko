// Package postgres implements study design persistence with sqlc-generated
// queries. Triggers and constraints in schema/006_experiments.sql are the last
// line of defence; their violations are translated to application errors.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// constraintErrors maps named constraints, unique indexes and trigger rules to
// the error a violation means.
var constraintErrors = map[string]error{
	"experiments_code_key":                    application.ErrCodeTaken,
	"experiments_transition":                  application.ErrInvalidTransition,
	"experiments_lifecycle_check":             application.ErrInvalidTransition,
	"experiment_groups_code_key":              application.ErrGroupCodeTaken,
	"experiment_groups_name_key":              application.ErrGroupNameTaken,
	"experiment_groups_disease_check":         domain.ErrDiseaseModelRequired,
	"experiment_groups_disease_model_id_fkey": application.ErrDiseaseModelNotFound,
	"experiment_groups_experiment_id_fkey":    application.ErrExperimentNotFound,
	"experiment_groups_frozen":                application.ErrDesignFrozen,
	"regimen_items_group_fkey":                application.ErrGroupNotInExperiment,
	"regimen_items_substance_id_fkey":         application.ErrSubstanceNotFound,
	"regimen_items_treatment_key":             application.ErrTreatmentItemTaken,
	"regimen_items_interval_check":            domain.ErrInvalidInterval,
	"regimen_items_days_check":                domain.ErrInvalidDays,
	"regimen_items_frozen":                    application.ErrDesignFrozen,
	"timepoints_position_key":                 application.ErrTimepointConflict,
	"timepoints_name_key":                     application.ErrTimepointConflict,
	"timepoints_experiment_id_fkey":           application.ErrExperimentNotFound,
	"timepoints_frozen":                       application.ErrDesignFrozen,
	"endpoints_timepoint_fkey":                application.ErrTimepointNotInExperiment,
	"endpoints_baseline_fkey":                 application.ErrTimepointNotInExperiment,
	"endpoints_baseline_check":                domain.ErrBaselineSameTimepoint,
	"endpoints_measure_key":                   application.ErrEndpointDuplicate,
	"endpoints_primary_key":                   application.ErrPrimaryEndpointTaken,
	"endpoints_frozen":                        application.ErrDesignFrozen,
	"allocations_group_fkey":                  application.ErrGroupNotInExperiment,
	"allocations_subject_id_fkey":             application.ErrSubjectNotFound,
	"allocations_subject_key":                 application.ErrAlreadyAllocated,
	"allocations_exclusion_check":             domain.ErrExclusionBeforeAllocation,
	"allocations_frozen":                      application.ErrAllocationFrozen,
}

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool, queries: sqlcgen.New(pool)} }

// Transaction runs fn in one transaction. Inside it, a nested Transaction call
// runs fn in the same transaction.
func (s *Store) Transaction(ctx context.Context, fn func(application.Store) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&txStore{Store{pool: s.pool, queries: s.queries.WithTx(tx)}})
	})
}

type txStore struct{ Store }

func (s *txStore) Transaction(_ context.Context, fn func(application.Store) error) error {
	return fn(s)
}

// Experiments

func (s *Store) CreateExperiment(ctx context.Context, e domain.Experiment) (domain.Experiment, error) {
	row, err := s.queries.CreateExperiment(ctx, sqlcgen.CreateExperimentParams{
		Code: e.Code, Title: e.Title, Description: optional(e.Description), Hypothesis: optional(e.Hypothesis),
		Randomisation: optional(e.Randomisation), Blinded: e.Blinded,
	})
	if err != nil {
		return domain.Experiment{}, translate(err, nil, "create experiment")
	}
	return toExperiment(row), nil
}

func (s *Store) Experiment(ctx context.Context, id string) (domain.Experiment, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Experiment{}, application.ErrExperimentNotFound
	}
	row, err := s.queries.GetExperiment(ctx, id)
	if err != nil {
		return domain.Experiment{}, translate(err, application.ErrExperimentNotFound, "get experiment")
	}
	return toExperiment(row), nil
}

func (s *Store) LockExperiment(ctx context.Context, id string) (domain.Experiment, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Experiment{}, application.ErrExperimentNotFound
	}
	row, err := s.queries.LockExperiment(ctx, id)
	if err != nil {
		return domain.Experiment{}, translate(err, application.ErrExperimentNotFound, "lock experiment")
	}
	return toExperiment(row), nil
}

func (s *Store) ListExperiments(ctx context.Context, f application.ExperimentFilter) ([]domain.Experiment, int, error) {
	search := pgtx.EscapeLike(f.Search)
	rows, err := s.queries.ListExperiments(ctx, sqlcgen.ListExperimentsParams{
		Search: search, Status: f.Status, SortKey: f.Sort, Descending: f.Descending, PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list experiments: %w", err)
	}
	total, err := s.queries.CountExperiments(ctx, sqlcgen.CountExperimentsParams{Search: search, Status: f.Status})
	if err != nil {
		return nil, 0, fmt.Errorf("count experiments: %w", err)
	}
	return mapSlice(rows, toExperiment), int(total), nil
}

func (s *Store) UpdateExperiment(ctx context.Context, e domain.Experiment) (domain.Experiment, error) {
	row, err := s.queries.UpdateExperiment(ctx, sqlcgen.UpdateExperimentParams{
		ID: e.ID, Code: e.Code, Title: e.Title, Description: optional(e.Description), Hypothesis: optional(e.Hypothesis),
		Randomisation: optional(e.Randomisation), Blinded: e.Blinded,
	})
	if err != nil {
		return domain.Experiment{}, translate(err, application.ErrExperimentNotFound, "update experiment")
	}
	return toExperiment(row), nil
}

func (s *Store) StartExperiment(ctx context.Context, id string, dayZero time.Time) (domain.Experiment, error) {
	row, err := s.queries.StartExperiment(ctx, sqlcgen.StartExperimentParams{ID: id, DayZero: dayZero})
	if err != nil {
		return domain.Experiment{}, translate(err, application.ErrNotDraft, "start experiment")
	}
	return toExperiment(row), nil
}

func (s *Store) CompleteExperiment(ctx context.Context, id string) (domain.Experiment, error) {
	row, err := s.queries.CompleteExperiment(ctx, id)
	if err != nil {
		return domain.Experiment{}, translate(err, application.ErrNotRunning, "complete experiment")
	}
	return toExperiment(row), nil
}

func (s *Store) OpenTests(ctx context.Context, experimentID string) (int, error) {
	n, err := s.queries.CountOpenTests(ctx, experimentID)
	if err != nil {
		return 0, fmt.Errorf("count open tests: %w", err)
	}
	return int(n), nil
}

func (s *Store) ProtocolParadigms(ctx context.Context, experimentID string) ([]application.ParadigmVersion, error) {
	if !pgtx.ValidUUID(experimentID) {
		return nil, application.ErrExperimentNotFound
	}
	rows, err := s.queries.ListProtocolParadigms(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list protocol paradigms: %w", err)
	}
	return mapSlice(rows, func(r sqlcgen.ListProtocolParadigmsRow) application.ParadigmVersion {
		return application.ParadigmVersion{Key: r.ParadigmKey, Version: int(r.ParadigmVersion)}
	}), nil
}

// Groups

func (s *Store) Groups(ctx context.Context, experimentID string) ([]application.GroupFill, error) {
	if !pgtx.ValidUUID(experimentID) {
		return nil, application.ErrExperimentNotFound
	}
	rows, err := s.queries.ListGroupFills(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	return mapSlice(rows, func(r sqlcgen.ListGroupFillsRow) application.GroupFill {
		g := toGroup(sqlcgen.MiskoExperimentGroup{ID: r.ID, ExperimentID: r.ExperimentID, Code: r.Code, Name: r.Name, Role: r.Role,
			DiseaseModelID: r.DiseaseModelID, TargetSize: r.TargetSize, Description: r.Description, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
		return application.GroupFill{Group: g, Allocated: int(r.Allocated), Excluded: int(r.Excluded)}
	}), nil
}

func (s *Store) LockGroup(ctx context.Context, experimentID, groupID string) (domain.Group, error) {
	if !pgtx.ValidUUID(groupID) {
		return domain.Group{}, application.ErrGroupNotFound
	}
	row, err := s.queries.LockGroup(ctx, sqlcgen.LockGroupParams{ExperimentID: experimentID, ID: groupID})
	if err != nil {
		return domain.Group{}, translate(err, application.ErrGroupNotFound, "lock group")
	}
	return toGroup(row), nil
}

func (s *Store) CreateGroup(ctx context.Context, g domain.Group) (domain.Group, error) {
	if g.DiseaseModelID != "" && !pgtx.ValidUUID(g.DiseaseModelID) {
		return domain.Group{}, application.ErrDiseaseModelNotFound
	}
	row, err := s.queries.CreateGroup(ctx, sqlcgen.CreateGroupParams{
		ExperimentID: g.ExperimentID, Code: g.Code, Name: g.Name, Role: string(g.Role), DiseaseModelID: optional(g.DiseaseModelID),
		TargetSize: target(g.TargetSize), Description: optional(g.Description),
	})
	if err != nil {
		return domain.Group{}, translate(err, nil, "create group")
	}
	return toGroup(row), nil
}

func (s *Store) UpdateGroup(ctx context.Context, g domain.Group) (domain.Group, error) {
	if g.DiseaseModelID != "" && !pgtx.ValidUUID(g.DiseaseModelID) {
		return domain.Group{}, application.ErrDiseaseModelNotFound
	}
	row, err := s.queries.UpdateGroup(ctx, sqlcgen.UpdateGroupParams{
		ExperimentID: g.ExperimentID, ID: g.ID, Code: g.Code, Name: g.Name, Role: string(g.Role), DiseaseModelID: optional(g.DiseaseModelID),
		TargetSize: target(g.TargetSize), Description: optional(g.Description),
	})
	if err != nil {
		return domain.Group{}, translate(err, application.ErrGroupNotFound, "update group")
	}
	return toGroup(row), nil
}

func (s *Store) DeleteGroup(ctx context.Context, experimentID, groupID string) error {
	if !pgtx.ValidUUID(groupID) {
		return application.ErrGroupNotFound
	}
	if err := s.queries.DeleteGroupRegimen(ctx, sqlcgen.DeleteGroupRegimenParams{ExperimentID: experimentID, GroupID: groupID}); err != nil {
		return translate(err, nil, "delete group regimen")
	}
	deleted, err := s.queries.DeleteGroup(ctx, sqlcgen.DeleteGroupParams{ExperimentID: experimentID, ID: groupID})
	return deleteResult(deleted, err, application.ErrGroupNotFound, application.ErrGroupInUse, "delete group")
}

// Regimen items

func (s *Store) RegimenItems(ctx context.Context, experimentID string) ([]domain.RegimenItem, error) {
	if !pgtx.ValidUUID(experimentID) {
		return nil, application.ErrExperimentNotFound
	}
	rows, err := s.queries.ListRegimenItems(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list regimen items: %w", err)
	}
	return mapSlice(rows, toRegimenItem), nil
}

func (s *Store) LockRegimenItem(ctx context.Context, experimentID, itemID string) (domain.RegimenItem, error) {
	if !pgtx.ValidUUID(itemID) {
		return domain.RegimenItem{}, application.ErrRegimenItemNotFound
	}
	row, err := s.queries.LockRegimenItem(ctx, sqlcgen.LockRegimenItemParams{ExperimentID: experimentID, ID: itemID})
	if err != nil {
		return domain.RegimenItem{}, translate(err, application.ErrRegimenItemNotFound, "lock regimen item")
	}
	return toRegimenItem(row), nil
}

func (s *Store) CreateRegimenItem(ctx context.Context, r domain.RegimenItem) (domain.RegimenItem, error) {
	if err := regimenReferences(r); err != nil {
		return domain.RegimenItem{}, err
	}
	row, err := s.queries.CreateRegimenItem(ctx, sqlcgen.CreateRegimenItemParams{
		ExperimentID: r.ExperimentID, GroupID: r.GroupID, Purpose: string(r.Purpose), SubstanceID: r.SubstanceID, DoseMicro: r.DoseMicro,
		Unit: r.Unit, Route: r.Route, Frequency: string(r.Frequency), IntervalDays: target(r.IntervalDays), StartDay: int32(r.StartDay), EndDay: int32(r.EndDay),
	})
	if err != nil {
		return domain.RegimenItem{}, translate(err, nil, "create regimen item")
	}
	return toRegimenItem(row), nil
}

func (s *Store) UpdateRegimenItem(ctx context.Context, r domain.RegimenItem) (domain.RegimenItem, error) {
	if err := regimenReferences(r); err != nil {
		return domain.RegimenItem{}, err
	}
	row, err := s.queries.UpdateRegimenItem(ctx, sqlcgen.UpdateRegimenItemParams{
		ExperimentID: r.ExperimentID, ID: r.ID, GroupID: r.GroupID, Purpose: string(r.Purpose), SubstanceID: r.SubstanceID, DoseMicro: r.DoseMicro,
		Unit: r.Unit, Route: r.Route, Frequency: string(r.Frequency), IntervalDays: target(r.IntervalDays), StartDay: int32(r.StartDay), EndDay: int32(r.EndDay),
	})
	if err != nil {
		return domain.RegimenItem{}, translate(err, application.ErrRegimenItemNotFound, "update regimen item")
	}
	return toRegimenItem(row), nil
}

func (s *Store) DeleteRegimenItem(ctx context.Context, experimentID, itemID string) error {
	if !pgtx.ValidUUID(itemID) {
		return application.ErrRegimenItemNotFound
	}
	deleted, err := s.queries.DeleteRegimenItem(ctx, sqlcgen.DeleteRegimenItemParams{ExperimentID: experimentID, ID: itemID})
	return deleteResult(deleted, err, application.ErrRegimenItemNotFound, nil, "delete regimen item")
}

func regimenReferences(r domain.RegimenItem) error {
	if !pgtx.ValidUUID(r.GroupID) {
		return application.ErrGroupNotInExperiment
	}
	if !pgtx.ValidUUID(r.SubstanceID) {
		return application.ErrSubstanceNotFound
	}
	return nil
}

// Timepoints

func (s *Store) Timepoints(ctx context.Context, experimentID string) ([]domain.Timepoint, error) {
	if !pgtx.ValidUUID(experimentID) {
		return nil, application.ErrExperimentNotFound
	}
	rows, err := s.queries.ListTimepoints(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list timepoints: %w", err)
	}
	return mapSlice(rows, toTimepoint), nil
}

func (s *Store) LockTimepoint(ctx context.Context, experimentID, timepointID string) (domain.Timepoint, error) {
	if !pgtx.ValidUUID(timepointID) {
		return domain.Timepoint{}, application.ErrTimepointNotFound
	}
	row, err := s.queries.LockTimepoint(ctx, sqlcgen.LockTimepointParams{ExperimentID: experimentID, ID: timepointID})
	if err != nil {
		return domain.Timepoint{}, translate(err, application.ErrTimepointNotFound, "lock timepoint")
	}
	return toTimepoint(row), nil
}

func (s *Store) CreateTimepoint(ctx context.Context, t domain.Timepoint) (domain.Timepoint, error) {
	row, err := s.queries.CreateTimepoint(ctx, sqlcgen.CreateTimepointParams{ExperimentID: t.ExperimentID, Name: t.Name, StudyDay: int32(t.StudyDay), Position: int32(t.Position)})
	if err != nil {
		return domain.Timepoint{}, translate(err, nil, "create timepoint")
	}
	return toTimepoint(row), nil
}

func (s *Store) UpdateTimepoint(ctx context.Context, t domain.Timepoint) (domain.Timepoint, error) {
	row, err := s.queries.UpdateTimepoint(ctx, sqlcgen.UpdateTimepointParams{ExperimentID: t.ExperimentID, ID: t.ID, Name: t.Name, StudyDay: int32(t.StudyDay), Position: int32(t.Position)})
	if err != nil {
		return domain.Timepoint{}, translate(err, application.ErrTimepointNotFound, "update timepoint")
	}
	return toTimepoint(row), nil
}

func (s *Store) DeleteTimepoint(ctx context.Context, experimentID, timepointID string) error {
	if !pgtx.ValidUUID(timepointID) {
		return application.ErrTimepointNotFound
	}
	deleted, err := s.queries.DeleteTimepoint(ctx, sqlcgen.DeleteTimepointParams{ExperimentID: experimentID, ID: timepointID})
	return deleteResult(deleted, err, application.ErrTimepointNotFound, application.ErrTimepointInUse, "delete timepoint")
}

// Endpoints

func (s *Store) Endpoints(ctx context.Context, experimentID string) ([]domain.Endpoint, error) {
	if !pgtx.ValidUUID(experimentID) {
		return nil, application.ErrExperimentNotFound
	}
	rows, err := s.queries.ListEndpoints(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list endpoints: %w", err)
	}
	return mapSlice(rows, toEndpoint), nil
}

func (s *Store) LockEndpoint(ctx context.Context, experimentID, endpointID string) (domain.Endpoint, error) {
	if !pgtx.ValidUUID(endpointID) {
		return domain.Endpoint{}, application.ErrEndpointNotFound
	}
	row, err := s.queries.LockEndpoint(ctx, sqlcgen.LockEndpointParams{ExperimentID: experimentID, ID: endpointID})
	if err != nil {
		return domain.Endpoint{}, translate(err, application.ErrEndpointNotFound, "lock endpoint")
	}
	return toEndpoint(row), nil
}

func (s *Store) CreateEndpoint(ctx context.Context, e domain.Endpoint) (domain.Endpoint, error) {
	if err := endpointReferences(e); err != nil {
		return domain.Endpoint{}, err
	}
	row, err := s.queries.CreateEndpoint(ctx, sqlcgen.CreateEndpointParams{
		ExperimentID: e.ExperimentID, TimepointID: e.TimepointID, BaselineTimepointID: optional(e.BaselineTimepointID),
		ParadigmKey: e.ParadigmKey, MetricKey: e.MetricKey, Kind: string(e.Kind), Direction: string(e.Direction),
	})
	if err != nil {
		return domain.Endpoint{}, translate(err, nil, "create endpoint")
	}
	return toEndpoint(row), nil
}

func (s *Store) UpdateEndpoint(ctx context.Context, e domain.Endpoint) (domain.Endpoint, error) {
	if err := endpointReferences(e); err != nil {
		return domain.Endpoint{}, err
	}
	row, err := s.queries.UpdateEndpoint(ctx, sqlcgen.UpdateEndpointParams{
		ExperimentID: e.ExperimentID, ID: e.ID, TimepointID: e.TimepointID, BaselineTimepointID: optional(e.BaselineTimepointID),
		ParadigmKey: e.ParadigmKey, MetricKey: e.MetricKey, Kind: string(e.Kind), Direction: string(e.Direction),
	})
	if err != nil {
		return domain.Endpoint{}, translate(err, application.ErrEndpointNotFound, "update endpoint")
	}
	return toEndpoint(row), nil
}

func (s *Store) DeleteEndpoint(ctx context.Context, experimentID, endpointID string) error {
	if !pgtx.ValidUUID(endpointID) {
		return application.ErrEndpointNotFound
	}
	deleted, err := s.queries.DeleteEndpoint(ctx, sqlcgen.DeleteEndpointParams{ExperimentID: experimentID, ID: endpointID})
	return deleteResult(deleted, err, application.ErrEndpointNotFound, nil, "delete endpoint")
}

func endpointReferences(e domain.Endpoint) error {
	if !pgtx.ValidUUID(e.TimepointID) || (e.BaselineTimepointID != "" && !pgtx.ValidUUID(e.BaselineTimepointID)) {
		return application.ErrTimepointNotInExperiment
	}
	return nil
}

// Allocations

func (s *Store) Allocations(ctx context.Context, f application.AllocationFilter) ([]application.AllocationView, error) {
	if !pgtx.ValidUUID(f.ExperimentID) {
		return nil, application.ErrExperimentNotFound
	}
	params := sqlcgen.ListAllocationsParams{ExperimentID: f.ExperimentID, Excluded: f.Excluded}
	if f.GroupID != "" {
		if !pgtx.ValidUUID(f.GroupID) {
			return []application.AllocationView{}, nil
		}
		params.GroupID = &f.GroupID
	}
	rows, err := s.queries.ListAllocations(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list allocations: %w", err)
	}
	return mapSlice(rows, func(r sqlcgen.ListAllocationsRow) application.AllocationView {
		a := toAllocation(sqlcgen.MiskoAllocation{ID: r.ID, ExperimentID: r.ExperimentID, GroupID: r.GroupID, SubjectID: r.SubjectID,
			AllocatedAt: r.AllocatedAt, Cage: r.Cage, ExcludedAt: r.ExcludedAt, ExclusionReason: r.ExclusionReason, ExclusionNote: r.ExclusionNote,
			CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt})
		return application.AllocationView{Allocation: a, SubjectCode: r.SubjectCode}
	}), nil
}

func (s *Store) LockAllocation(ctx context.Context, experimentID, allocationID string) (domain.Allocation, error) {
	if !pgtx.ValidUUID(allocationID) {
		return domain.Allocation{}, application.ErrAllocationNotFound
	}
	row, err := s.queries.LockAllocation(ctx, sqlcgen.LockAllocationParams{ExperimentID: experimentID, ID: allocationID})
	if err != nil {
		return domain.Allocation{}, translate(err, application.ErrAllocationNotFound, "lock allocation")
	}
	return toAllocation(row), nil
}

func (s *Store) CreateAllocation(ctx context.Context, a domain.Allocation) (domain.Allocation, error) {
	if !pgtx.ValidUUID(a.GroupID) {
		return domain.Allocation{}, application.ErrGroupNotInExperiment
	}
	if !pgtx.ValidUUID(a.SubjectID) {
		return domain.Allocation{}, application.ErrSubjectNotFound
	}
	row, err := s.queries.CreateAllocation(ctx, sqlcgen.CreateAllocationParams{
		ExperimentID: a.ExperimentID, GroupID: a.GroupID, SubjectID: a.SubjectID, AllocatedAt: a.AllocatedAt, Cage: optional(a.Cage), CreatedBy: a.CreatedBy,
	})
	if err != nil {
		return domain.Allocation{}, translate(err, nil, "create allocation")
	}
	return toAllocation(row), nil
}

func (s *Store) UpdateAllocation(ctx context.Context, a domain.Allocation) (domain.Allocation, error) {
	if !pgtx.ValidUUID(a.GroupID) {
		return domain.Allocation{}, application.ErrGroupNotInExperiment
	}
	params := sqlcgen.UpdateAllocationParams{ExperimentID: a.ExperimentID, ID: a.ID, GroupID: a.GroupID, Cage: optional(a.Cage),
		ExcludedAt: a.ExcludedAt, ExclusionNote: optional(a.ExclusionNote)}
	if a.ExcludedAt != nil {
		reason := string(a.ExclusionReason)
		params.ExclusionReason = &reason
	}
	row, err := s.queries.UpdateAllocation(ctx, params)
	if err != nil {
		return domain.Allocation{}, translate(err, application.ErrAllocationNotFound, "update allocation")
	}
	return toAllocation(row), nil
}

func (s *Store) DeleteAllocation(ctx context.Context, experimentID, allocationID string) error {
	if !pgtx.ValidUUID(allocationID) {
		return application.ErrAllocationNotFound
	}
	deleted, err := s.queries.DeleteAllocation(ctx, sqlcgen.DeleteAllocationParams{ExperimentID: experimentID, ID: allocationID})
	return deleteResult(deleted, err, application.ErrAllocationNotFound, application.ErrAllocationFrozen, "delete allocation")
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

// deleteResult maps a foreign key violation (a row still referencing the
// deleted one) to inUse and no deleted row to notFound.
func deleteResult(deleted int64, err, notFound, inUse error, operation string) error {
	if code, _ := pgtx.Violation(err); code == "23503" && inUse != nil {
		return inUse
	}
	if err != nil {
		return translate(err, nil, operation)
	}
	if deleted == 0 {
		return notFound
	}
	return nil
}

func toExperiment(r sqlcgen.MiskoExperiment) domain.Experiment {
	e := domain.Experiment{ID: r.ID, Code: r.Code, Title: r.Title, Description: value(r.Description), Hypothesis: value(r.Hypothesis),
		Randomisation: value(r.Randomisation), Blinded: r.Blinded, Status: domain.Status(r.Status),
		StartedAt: utc(r.StartedAt), CompletedAt: utc(r.CompletedAt), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
	if r.DayZero != nil {
		day := time.Date(r.DayZero.Year(), r.DayZero.Month(), r.DayZero.Day(), 0, 0, 0, 0, time.UTC)
		e.DayZero = &day
	}
	return e
}

func toGroup(r sqlcgen.MiskoExperimentGroup) domain.Group {
	g := domain.Group{ID: r.ID, ExperimentID: r.ExperimentID, Code: r.Code, Name: r.Name, Role: domain.Role(r.Role), DiseaseModelID: value(r.DiseaseModelID),
		Description: value(r.Description), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
	if r.TargetSize != nil {
		g.TargetSize = int(*r.TargetSize)
	}
	return g
}

func toRegimenItem(r sqlcgen.MiskoRegimenItem) domain.RegimenItem {
	item := domain.RegimenItem{ID: r.ID, ExperimentID: r.ExperimentID, GroupID: r.GroupID, Purpose: domain.Purpose(r.Purpose), SubstanceID: r.SubstanceID,
		DoseMicro: r.DoseMicro, Unit: r.Unit, Route: r.Route, Frequency: domain.Frequency(r.Frequency), StartDay: int(r.StartDay), EndDay: int(r.EndDay),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
	if r.IntervalDays != nil {
		item.IntervalDays = int(*r.IntervalDays)
	}
	return item
}

func toTimepoint(r sqlcgen.MiskoTimepoint) domain.Timepoint {
	return domain.Timepoint{ID: r.ID, ExperimentID: r.ExperimentID, Name: r.Name, StudyDay: int(r.StudyDay), Position: int(r.Position),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func toEndpoint(r sqlcgen.MiskoEndpoint) domain.Endpoint {
	return domain.Endpoint{ID: r.ID, ExperimentID: r.ExperimentID, TimepointID: r.TimepointID, BaselineTimepointID: value(r.BaselineTimepointID),
		ParadigmKey: r.ParadigmKey, MetricKey: r.MetricKey, Kind: domain.EndpointKind(r.Kind), Direction: domain.Direction(r.Direction),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func toAllocation(r sqlcgen.MiskoAllocation) domain.Allocation {
	return domain.Allocation{ID: r.ID, ExperimentID: r.ExperimentID, GroupID: r.GroupID, SubjectID: r.SubjectID, AllocatedAt: r.AllocatedAt.UTC(),
		Cage: value(r.Cage), ExcludedAt: utc(r.ExcludedAt), ExclusionReason: domain.ExclusionReason(value(r.ExclusionReason)),
		ExclusionNote: value(r.ExclusionNote), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC()}
}

func mapSlice[T, U any](items []T, convert func(T) U) []U {
	out := make([]U, 0, len(items))
	for _, item := range items {
		out = append(out, convert(item))
	}
	return out
}

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

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// target stores 0 as NULL.
func target(n int) *int32 {
	if n == 0 {
		return nil
	}
	v := int32(n)
	return &v
}
