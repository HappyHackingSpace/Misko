// Package postgres implements test, trial and comment persistence with
// sqlc-generated queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"time"
)

// constraintErrors maps named constraints to the error a violation means.
var constraintErrors = map[string]error{
	"tests_enrollment_fkey":       application.ErrEnrollmentNotFound,
	"tests_subject_fkey":          application.ErrEnrollmentNotFound,
	"tests_phase_fkey":            application.ErrPhaseNotFound,
	"tests_protocol_version_fkey": application.ErrProtocolVersionNotFound,
	"tests_step_fkey":             application.ErrStepNotFound,
	"tests_transition":            domain.ErrInvalidTransition,
	"trials_test_in_progress":     domain.ErrTestNotInProgress,
	"test_comments_test_id_fkey":  application.ErrTestNotFound,
	"trials_test_id_fkey":         application.ErrTestNotFound,
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

func (s *Store) LockEnrollment(ctx context.Context, experimentID, enrollmentID string) (application.EnrollmentRef, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(enrollmentID) {
		return application.EnrollmentRef{}, application.ErrEnrollmentNotFound
	}
	row, err := s.queries.LockEnrollmentRef(ctx, sqlcgen.LockEnrollmentRefParams{ExperimentID: experimentID, ID: enrollmentID})
	if err != nil {
		return application.EnrollmentRef{}, translate(err, application.ErrEnrollmentNotFound, "lock enrollment")
	}
	return application.EnrollmentRef{ID: row.ID, ExperimentID: row.ExperimentID, SubjectID: row.SubjectID, EnrolledAt: row.EnrolledAt.UTC()}, nil
}

func (s *Store) Phase(ctx context.Context, experimentID, phaseID string) error {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(phaseID) {
		return application.ErrPhaseNotFound
	}
	exists, err := s.queries.PhaseExists(ctx, sqlcgen.PhaseExistsParams{ExperimentID: experimentID, ID: phaseID})
	if err != nil {
		return fmt.Errorf("check phase: %w", err)
	}
	if !exists {
		return application.ErrPhaseNotFound
	}
	return nil
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

func (s *Store) ProtocolStep(ctx context.Context, experimentID, versionID string, position int) (application.StepRef, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(versionID) {
		return application.StepRef{}, application.ErrProtocolVersionNotFound
	}
	if position < 1 || position > math.MaxInt32 {
		return application.StepRef{}, application.ErrStepNotFound
	}
	row, err := s.queries.GetProtocolStep(ctx, sqlcgen.GetProtocolStepParams{ExperimentID: experimentID, ProtocolVersionID: versionID, Position: int32(position)})
	if errors.Is(err, pgx.ErrNoRows) {
		exists, err := s.queries.ProtocolVersionExists(ctx, sqlcgen.ProtocolVersionExistsParams{ExperimentID: experimentID, ID: versionID})
		switch {
		case err != nil:
			return application.StepRef{}, fmt.Errorf("check protocol version: %w", err)
		case exists:
			return application.StepRef{}, application.ErrStepNotFound
		}
		return application.StepRef{}, application.ErrProtocolVersionNotFound
	}
	if err != nil {
		return application.StepRef{}, fmt.Errorf("get protocol step: %w", err)
	}
	return application.StepRef{
		ProtocolVersionID: row.ProtocolVersionID, Position: int(row.Position), ParadigmKey: row.ParadigmKey, ParadigmVersion: int(row.ParadigmVersion),
		EnvironmentRevisionID: row.EnvironmentRevisionID, Trials: int(row.Trials),
	}, nil
}

func (s *Store) CreateTest(ctx context.Context, t domain.Test) (domain.Test, error) {
	row, err := s.queries.CreateTest(ctx, sqlcgen.CreateTestParams{
		ExperimentID: t.ExperimentID, EnrollmentID: t.EnrollmentID, SubjectID: t.SubjectID, PhaseID: optional(t.PhaseID), GroupID: optional(t.GroupID),
		ProtocolVersionID: t.ProtocolVersionID, StepPosition: int32(t.StepPosition), ParadigmKey: t.ParadigmKey, ParadigmVersion: int32(t.ParadigmVersion),
		EnvironmentRevisionID: t.EnvironmentRevisionID, PlannedTrials: int32(t.PlannedTrials), ScheduledAt: t.ScheduledAt, Notes: optional(t.Notes), CreatedBy: t.CreatedBy,
	})
	if err != nil {
		return domain.Test{}, translate(err, nil, "create test")
	}
	return toTest(row), nil
}

func (s *Store) Test(ctx context.Context, id string) (domain.Test, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Test{}, application.ErrTestNotFound
	}
	row, err := s.queries.GetTest(ctx, id)
	if err != nil {
		return domain.Test{}, translate(err, application.ErrTestNotFound, "get test")
	}
	return toTest(row), nil
}

func (s *Store) LockTest(ctx context.Context, id string) (domain.Test, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Test{}, application.ErrTestNotFound
	}
	row, err := s.queries.LockTest(ctx, id)
	if err != nil {
		return domain.Test{}, translate(err, application.ErrTestNotFound, "lock test")
	}
	return toTest(row), nil
}

func (s *Store) UpdateTestStatus(ctx context.Context, t domain.Test, from domain.Status) (domain.Test, error) {
	row, err := s.queries.UpdateTestStatus(ctx, sqlcgen.UpdateTestStatusParams{
		ID: t.ID, FromStatus: string(from), Status: string(t.Status), StartedAt: t.StartedAt, CompletedAt: t.CompletedAt,
		CancelledAt: t.CancelledAt, CancelReason: optional(t.CancelReason),
	})
	if err != nil {
		return domain.Test{}, translate(err, domain.ErrInvalidTransition, "update test status")
	}
	return toTest(row), nil
}

func (s *Store) ListTests(ctx context.Context, f application.TestFilter) ([]domain.Test, int, error) {
	if !pgtx.ValidUUID(f.ExperimentID) || (f.SubjectID != "" && !pgtx.ValidUUID(f.SubjectID)) || (f.PhaseID != "" && !pgtx.ValidUUID(f.PhaseID)) {
		return []domain.Test{}, 0, nil
	}
	subject, phase := optional(f.SubjectID), optional(f.PhaseID)
	rows, err := s.queries.ListTests(ctx, sqlcgen.ListTestsParams{
		ExperimentID: f.ExperimentID, SubjectID: subject, PhaseID: phase, Status: string(f.Status), ParadigmKey: f.ParadigmKey,
		PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list tests: %w", err)
	}
	total, err := s.queries.CountTests(ctx, sqlcgen.CountTestsParams{
		ExperimentID: f.ExperimentID, SubjectID: subject, PhaseID: phase, Status: string(f.Status), ParadigmKey: f.ParadigmKey,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count tests: %w", err)
	}
	out := make([]domain.Test, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTest(row))
	}
	return out, int(total), nil
}

func (s *Store) Trials(ctx context.Context, testID string) ([]domain.Trial, error) {
	if !pgtx.ValidUUID(testID) {
		return []domain.Trial{}, nil
	}
	rows, err := s.queries.ListTrials(ctx, testID)
	if err != nil {
		return nil, fmt.Errorf("list trials: %w", err)
	}
	out := make([]domain.Trial, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTrial(row))
	}
	return out, nil
}

func (s *Store) CreateTrial(ctx context.Context, tr domain.Trial) (domain.Trial, error) {
	row, err := s.queries.CreateTrial(ctx, sqlcgen.CreateTrialParams{
		TestID: tr.TestID, Number: int32(tr.Number), Repetition: int32(tr.Repetition), Attempt: int32(tr.Attempt),
		StartedAt: tr.StartedAt, EndedAt: tr.EndedAt, Notes: optional(tr.Notes), RecordedBy: tr.RecordedBy,
	})
	if err != nil {
		return domain.Trial{}, translate(err, nil, "create trial")
	}
	return toTrial(row), nil
}

func (s *Store) Comments(ctx context.Context, testID string) ([]domain.Comment, error) {
	if !pgtx.ValidUUID(testID) {
		return []domain.Comment{}, nil
	}
	rows, err := s.queries.ListComments(ctx, testID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	out := make([]domain.Comment, 0, len(rows))
	for _, row := range rows {
		out = append(out, toComment(sqlcgen.GetCommentRow(row)))
	}
	return out, nil
}

func (s *Store) Comment(ctx context.Context, testID, commentID string) (domain.Comment, error) {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(commentID) {
		return domain.Comment{}, application.ErrCommentNotFound
	}
	row, err := s.queries.GetComment(ctx, sqlcgen.GetCommentParams{TestID: testID, ID: commentID})
	if err != nil {
		return domain.Comment{}, translate(err, application.ErrCommentNotFound, "get comment")
	}
	return toComment(row), nil
}

func (s *Store) CreateComment(ctx context.Context, c domain.Comment) (domain.Comment, error) {
	if !pgtx.ValidUUID(c.TestID) {
		return domain.Comment{}, application.ErrTestNotFound
	}
	id, err := s.queries.CreateComment(ctx, sqlcgen.CreateCommentParams{TestID: c.TestID, AuthorID: c.AuthorID, Body: c.Body})
	if err != nil {
		return domain.Comment{}, translate(err, nil, "create comment")
	}
	return s.Comment(ctx, c.TestID, id)
}

func (s *Store) UpdateComment(ctx context.Context, testID, commentID, body string) (domain.Comment, error) {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(commentID) {
		return domain.Comment{}, application.ErrCommentNotFound
	}
	updated, err := s.queries.UpdateComment(ctx, sqlcgen.UpdateCommentParams{TestID: testID, ID: commentID, Body: body})
	if err != nil {
		return domain.Comment{}, fmt.Errorf("update comment: %w", err)
	}
	if updated == 0 {
		return domain.Comment{}, application.ErrCommentNotFound
	}
	return s.Comment(ctx, testID, commentID)
}

func (s *Store) DeleteComment(ctx context.Context, testID, commentID string) error {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(commentID) {
		return application.ErrCommentNotFound
	}
	deleted, err := s.queries.DeleteComment(ctx, sqlcgen.DeleteCommentParams{TestID: testID, ID: commentID})
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	if deleted == 0 {
		return application.ErrCommentNotFound
	}
	return nil
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

func toTest(r sqlcgen.MiskoTest) domain.Test {
	return domain.Test{
		ID: r.ID, ExperimentID: r.ExperimentID, EnrollmentID: r.EnrollmentID, SubjectID: r.SubjectID, PhaseID: value(r.PhaseID), GroupID: value(r.GroupID),
		ProtocolVersionID: r.ProtocolVersionID, StepPosition: int(r.StepPosition), ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion),
		EnvironmentRevisionID: r.EnvironmentRevisionID, PlannedTrials: int(r.PlannedTrials), Status: domain.Status(r.Status),
		ScheduledAt: r.ScheduledAt.UTC(), StartedAt: utc(r.StartedAt), CompletedAt: utc(r.CompletedAt), CancelledAt: utc(r.CancelledAt),
		CancelReason: value(r.CancelReason), Notes: value(r.Notes), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func toTrial(r sqlcgen.MiskoTrial) domain.Trial {
	return domain.Trial{
		ID: r.ID, TestID: r.TestID, Number: int(r.Number), Repetition: int(r.Repetition), Attempt: int(r.Attempt),
		StartedAt: r.StartedAt.UTC(), EndedAt: utc(r.EndedAt), Notes: value(r.Notes), RecordedBy: r.RecordedBy, CreatedAt: r.CreatedAt.UTC(),
	}
}

func toComment(r sqlcgen.GetCommentRow) domain.Comment {
	return domain.Comment{
		ID: r.ID, TestID: r.TestID, AuthorID: r.AuthorID, AuthorName: r.AuthorName, Body: r.Body, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
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
