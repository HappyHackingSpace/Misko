// Package postgres implements experiment persistence with sqlc-generated queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// constraintErrors maps named constraints to the error a violation means.
var constraintErrors = map[string]error{
	"experiments_code_key":                 application.ErrCodeTaken,
	"experiment_phases_position_key":       application.ErrPhaseConflict,
	"experiment_phases_name_key":           application.ErrPhaseConflict,
	"experiment_phases_experiment_id_fkey": application.ErrExperimentNotFound,
	"experiment_groups_name_key":           application.ErrGroupNameTaken,
	"experiment_groups_experiment_id_fkey": application.ErrExperimentNotFound,
	"enrollments_subject_key":              application.ErrAlreadyEnrolled,
	"enrollments_experiment_id_fkey":       application.ErrExperimentNotFound,
	"enrollments_subject_id_fkey":          application.ErrSubjectNotFound,
	"group_assignments_enrollment_fkey":    application.ErrEnrollmentNotFound,
	"group_assignments_group_fkey":         domain.ErrGroupNotInExperiment,
	"group_assignments_no_overlap":         application.ErrOverlappingAssignment,
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

func (s *Store) CreateExperiment(ctx context.Context, e domain.Experiment) (domain.Experiment, error) {
	row, err := s.queries.CreateExperiment(ctx, sqlcgen.CreateExperimentParams{Code: e.Code, Title: e.Title, Description: optional(e.Description), RequiresControl: e.RequiresControl})
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

func (s *Store) ListExperiments(ctx context.Context, f application.ExperimentFilter) ([]domain.Experiment, int, error) {
	search := pgtx.EscapeLike(f.Search)
	rows, err := s.queries.ListExperiments(ctx, sqlcgen.ListExperimentsParams{
		Search: search, SortKey: f.Sort, Descending: f.Descending, PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list experiments: %w", err)
	}
	total, err := s.queries.CountExperiments(ctx, search)
	if err != nil {
		return nil, 0, fmt.Errorf("count experiments: %w", err)
	}
	experiments := make([]domain.Experiment, 0, len(rows))
	for _, row := range rows {
		experiments = append(experiments, toExperiment(row))
	}
	return experiments, int(total), nil
}

func (s *Store) UpdateExperiment(ctx context.Context, id string, c application.ExperimentChanges) (domain.Experiment, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Experiment{}, application.ErrExperimentNotFound
	}
	row, err := s.queries.UpdateExperiment(ctx, sqlcgen.UpdateExperimentParams{ID: id, Code: c.Code, Title: c.Title, Description: c.Description, RequiresControl: c.RequiresControl})
	if err != nil {
		return domain.Experiment{}, translate(err, application.ErrExperimentNotFound, "update experiment")
	}
	return toExperiment(row), nil
}

func (s *Store) Phases(ctx context.Context, experimentID string) ([]domain.Phase, error) {
	if !pgtx.ValidUUID(experimentID) {
		return nil, application.ErrExperimentNotFound
	}
	rows, err := s.queries.ListPhases(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list phases: %w", err)
	}
	phases := make([]domain.Phase, 0, len(rows))
	for _, row := range rows {
		phases = append(phases, toPhase(row))
	}
	return phases, nil
}

func (s *Store) CreatePhase(ctx context.Context, p domain.Phase) (domain.Phase, error) {
	if !pgtx.ValidUUID(p.ExperimentID) {
		return domain.Phase{}, application.ErrExperimentNotFound
	}
	row, err := s.queries.CreatePhase(ctx, sqlcgen.CreatePhaseParams{ExperimentID: p.ExperimentID, Name: p.Name, Position: int32(p.Position), Description: optional(p.Description)})
	if err != nil {
		return domain.Phase{}, translate(err, nil, "create phase")
	}
	return toPhase(row), nil
}

func (s *Store) UpdatePhase(ctx context.Context, experimentID, phaseID string, c application.PhaseChanges) (domain.Phase, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(phaseID) {
		return domain.Phase{}, application.ErrPhaseNotFound
	}
	params := sqlcgen.UpdatePhaseParams{ExperimentID: experimentID, ID: phaseID, Name: c.Name, Description: c.Description}
	if c.Position != nil {
		position := int32(*c.Position)
		params.Position = &position
	}
	row, err := s.queries.UpdatePhase(ctx, params)
	if err != nil {
		return domain.Phase{}, translate(err, application.ErrPhaseNotFound, "update phase")
	}
	return toPhase(row), nil
}

func (s *Store) DeletePhase(ctx context.Context, experimentID, phaseID string) error {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(phaseID) {
		return application.ErrPhaseNotFound
	}
	deleted, err := s.queries.DeletePhase(ctx, sqlcgen.DeletePhaseParams{ExperimentID: experimentID, ID: phaseID})
	if err != nil {
		return fmt.Errorf("delete phase: %w", err)
	}
	if deleted == 0 {
		return application.ErrPhaseNotFound
	}
	return nil
}

func (s *Store) GroupSizes(ctx context.Context, experimentID string) ([]application.GroupSize, error) {
	if !pgtx.ValidUUID(experimentID) {
		return nil, application.ErrExperimentNotFound
	}
	rows, err := s.queries.ListGroupSizes(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	sizes := make([]application.GroupSize, 0, len(rows))
	for _, r := range rows {
		group := toGroup(sqlcgen.MiskoExperimentGroup{ID: r.ID, ExperimentID: r.ExperimentID, Name: r.Name, Role: r.Role, TargetSize: r.TargetSize, Description: r.Description, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
		sizes = append(sizes, application.GroupSize{Group: group, ActiveSubjects: int(r.ActiveSubjects)})
	}
	return sizes, nil
}

func (s *Store) Group(ctx context.Context, experimentID, groupID string) (domain.Group, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(groupID) {
		return domain.Group{}, application.ErrGroupNotFound
	}
	row, err := s.queries.GetGroup(ctx, sqlcgen.GetGroupParams{ExperimentID: experimentID, ID: groupID})
	if err != nil {
		return domain.Group{}, translate(err, application.ErrGroupNotFound, "get group")
	}
	return toGroup(row), nil
}

func (s *Store) CreateGroup(ctx context.Context, g domain.Group) (domain.Group, error) {
	if !pgtx.ValidUUID(g.ExperimentID) {
		return domain.Group{}, application.ErrExperimentNotFound
	}
	row, err := s.queries.CreateGroup(ctx, sqlcgen.CreateGroupParams{
		ExperimentID: g.ExperimentID, Name: g.Name, Role: string(g.Role), TargetSize: target(g.TargetSize), Description: optional(g.Description),
	})
	if err != nil {
		return domain.Group{}, translate(err, nil, "create group")
	}
	return toGroup(row), nil
}

func (s *Store) UpdateGroup(ctx context.Context, experimentID, groupID string, c application.GroupChanges) (domain.Group, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(groupID) {
		return domain.Group{}, application.ErrGroupNotFound
	}
	params := sqlcgen.UpdateGroupParams{ExperimentID: experimentID, ID: groupID, Name: c.Name, Description: c.Description}
	if c.Role != nil {
		role := string(*c.Role)
		params.Role = &role
	}
	if c.TargetSize != nil {
		size := int32(*c.TargetSize)
		params.TargetSize = &size
	}
	row, err := s.queries.UpdateGroup(ctx, params)
	if err != nil {
		return domain.Group{}, translate(err, application.ErrGroupNotFound, "update group")
	}
	return toGroup(row), nil
}

func (s *Store) DeleteGroup(ctx context.Context, experimentID, groupID string) error {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(groupID) {
		return application.ErrGroupNotFound
	}
	deleted, err := s.queries.DeleteGroup(ctx, sqlcgen.DeleteGroupParams{ExperimentID: experimentID, ID: groupID})
	if code, _ := pgtx.Violation(err); code == "23503" {
		return application.ErrGroupInUse
	}
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	if deleted == 0 {
		return application.ErrGroupNotFound
	}
	return nil
}

func (s *Store) CreateEnrollment(ctx context.Context, e domain.Enrollment) (domain.Enrollment, error) {
	if !pgtx.ValidUUID(e.ExperimentID) {
		return domain.Enrollment{}, application.ErrExperimentNotFound
	}
	if !pgtx.ValidUUID(e.SubjectID) {
		return domain.Enrollment{}, application.ErrSubjectNotFound
	}
	row, err := s.queries.CreateEnrollment(ctx, sqlcgen.CreateEnrollmentParams{ExperimentID: e.ExperimentID, SubjectID: e.SubjectID, EnrolledAt: e.EnrolledAt})
	if err != nil {
		return domain.Enrollment{}, translate(err, nil, "create enrollment")
	}
	return toEnrollment(row), nil
}

func (s *Store) LockEnrollment(ctx context.Context, experimentID, enrollmentID string) (domain.Enrollment, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(enrollmentID) {
		return domain.Enrollment{}, application.ErrEnrollmentNotFound
	}
	row, err := s.queries.LockEnrollment(ctx, sqlcgen.LockEnrollmentParams{ExperimentID: experimentID, ID: enrollmentID})
	if err != nil {
		return domain.Enrollment{}, translate(err, application.ErrEnrollmentNotFound, "lock enrollment")
	}
	return toEnrollment(row), nil
}

func (s *Store) Enrollment(ctx context.Context, experimentID, enrollmentID string) (domain.Enrollment, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(enrollmentID) {
		return domain.Enrollment{}, application.ErrEnrollmentNotFound
	}
	row, err := s.queries.GetEnrollment(ctx, sqlcgen.GetEnrollmentParams{ExperimentID: experimentID, ID: enrollmentID})
	if err != nil {
		return domain.Enrollment{}, translate(err, application.ErrEnrollmentNotFound, "get enrollment")
	}
	return toEnrollment(row), nil
}

func (s *Store) ListEnrollments(ctx context.Context, f application.EnrollmentFilter) ([]application.EnrollmentSummary, int, error) {
	rows, err := s.queries.ListEnrollments(ctx, sqlcgen.ListEnrollmentsParams{
		ExperimentID: f.ExperimentID, SubjectID: f.SubjectID, GroupID: f.GroupID, Descending: f.Descending,
		PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list enrollments: %w", err)
	}
	total, err := s.queries.CountEnrollments(ctx, sqlcgen.CountEnrollmentsParams{ExperimentID: f.ExperimentID, SubjectID: f.SubjectID, GroupID: f.GroupID})
	if err != nil {
		return nil, 0, fmt.Errorf("count enrollments: %w", err)
	}
	summaries := make([]application.EnrollmentSummary, 0, len(rows))
	for _, r := range rows {
		enrollment := toEnrollment(sqlcgen.MiskoEnrollment{ID: r.ID, ExperimentID: r.ExperimentID, SubjectID: r.SubjectID, EnrolledAt: r.EnrolledAt, CreatedAt: r.CreatedAt})
		summaries = append(summaries, application.EnrollmentSummary{Enrollment: enrollment, CurrentGroupID: r.CurrentGroupID})
	}
	return summaries, int(total), nil
}

func (s *Store) SubjectEnrollments(ctx context.Context, subjectID string) ([]application.EnrollmentSummary, error) {
	if !pgtx.ValidUUID(subjectID) {
		return []application.EnrollmentSummary{}, nil
	}
	rows, err := s.queries.ListSubjectEnrollments(ctx, subjectID)
	if err != nil {
		return nil, fmt.Errorf("list subject enrollments: %w", err)
	}
	summaries := make([]application.EnrollmentSummary, 0, len(rows))
	for _, r := range rows {
		enrollment := toEnrollment(sqlcgen.MiskoEnrollment{ID: r.ID, ExperimentID: r.ExperimentID, SubjectID: r.SubjectID, EnrolledAt: r.EnrolledAt, CreatedAt: r.CreatedAt})
		summaries = append(summaries, application.EnrollmentSummary{Enrollment: enrollment, CurrentGroupID: r.CurrentGroupID})
	}
	return summaries, nil
}

func (s *Store) Assignments(ctx context.Context, enrollmentID string) ([]domain.Assignment, error) {
	if !pgtx.ValidUUID(enrollmentID) {
		return nil, application.ErrEnrollmentNotFound
	}
	rows, err := s.queries.ListAssignments(ctx, enrollmentID)
	if err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}
	history := make([]domain.Assignment, 0, len(rows))
	for _, row := range rows {
		history = append(history, toAssignment(row))
	}
	return history, nil
}

func (s *Store) CloseAssignment(ctx context.Context, assignmentID string, validTo time.Time) error {
	closed, err := s.queries.CloseAssignment(ctx, sqlcgen.CloseAssignmentParams{ID: assignmentID, ValidTo: validTo})
	if err != nil {
		return translate(err, nil, "close assignment")
	}
	if closed == 0 {
		return fmt.Errorf("close assignment %s: no open assignment", assignmentID)
	}
	return nil
}

func (s *Store) CreateAssignment(ctx context.Context, a domain.Assignment) (domain.Assignment, error) {
	row, err := s.queries.CreateAssignment(ctx, sqlcgen.CreateAssignmentParams{ExperimentID: a.ExperimentID, EnrollmentID: a.EnrollmentID, GroupID: a.GroupID, ValidFrom: a.ValidFrom})
	if err != nil {
		return domain.Assignment{}, translate(err, nil, "create assignment")
	}
	return toAssignment(row), nil
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

func toExperiment(r sqlcgen.MiskoExperiment) domain.Experiment {
	return domain.Experiment{ID: r.ID, Code: r.Code, Title: r.Title, Description: value(r.Description), RequiresControl: r.RequiresControl, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func toPhase(r sqlcgen.MiskoExperimentPhase) domain.Phase {
	return domain.Phase{ID: r.ID, ExperimentID: r.ExperimentID, Name: r.Name, Position: int(r.Position), Description: value(r.Description), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func toGroup(r sqlcgen.MiskoExperimentGroup) domain.Group {
	g := domain.Group{ID: r.ID, ExperimentID: r.ExperimentID, Name: r.Name, Role: domain.GroupRole(r.Role), Description: value(r.Description), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
	if r.TargetSize != nil {
		g.TargetSize = int(*r.TargetSize)
	}
	return g
}

func toEnrollment(r sqlcgen.MiskoEnrollment) domain.Enrollment {
	return domain.Enrollment{ID: r.ID, ExperimentID: r.ExperimentID, SubjectID: r.SubjectID, EnrolledAt: r.EnrolledAt.UTC(), CreatedAt: r.CreatedAt.UTC()}
}

func toAssignment(r sqlcgen.MiskoGroupAssignment) domain.Assignment {
	a := domain.Assignment{ID: r.ID, ExperimentID: r.ExperimentID, EnrollmentID: r.EnrollmentID, GroupID: r.GroupID, ValidFrom: r.ValidFrom.UTC(), CreatedAt: r.CreatedAt.UTC()}
	if r.ValidTo.Valid {
		to := r.ValidTo.Time.UTC()
		a.ValidTo = &to
	}
	return a
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

// target stores "no target" as NULL.
func target(size int) *int32 {
	if size == 0 {
		return nil
	}
	n := int32(size)
	return &n
}
