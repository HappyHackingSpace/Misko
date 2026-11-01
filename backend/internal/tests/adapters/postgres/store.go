// Package postgres implements test, test subject, trial and comment
// persistence with sqlc-generated queries.
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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"time"
)

// constraintErrors maps named constraints and trigger rules to the error a
// violation means. They are the last line of defence behind the use cases.
var constraintErrors = map[string]error{
	"tests_not_running":             application.ErrExperimentNotRunning,
	"tests_timepoint_fkey":          application.ErrTimepointNotFound,
	"tests_protocol_version_fkey":   application.ErrProtocolVersionNotFound,
	"tests_step_fkey":               application.ErrStepNotFound,
	"tests_transition":              domain.ErrInvalidTransition,
	"tests_immutable":               domain.ErrInvalidTransition,
	"test_subjects_test_fkey":       application.ErrTestNotFound,
	"test_subjects_paradigm_fkey":   application.ErrArenaParadigmMismatch,
	"test_subjects_allocation_fkey": application.ErrAllocationNotFound,
	"test_subjects_arena_fkey":      application.ErrArenaParadigmMismatch,
	"test_subjects_animal_key":      domain.ErrDuplicateAnimal,
	"test_subjects_arena_key":       domain.ErrDuplicateArena,
	"test_subjects_frozen":          domain.ErrNotPlanned,
	"trials_test_in_progress":       domain.ErrTestNotInProgress,
	"trials_test_subject_fkey":      application.ErrTestSubjectNotFound,
	"test_comments_test_id_fkey":    application.ErrTestNotFound,
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

func (s *Store) ExperimentStatus(ctx context.Context, experimentID string) (string, error) {
	if !pgtx.ValidUUID(experimentID) {
		return "", application.ErrExperimentNotFound
	}
	status, err := s.queries.GetExperimentStatus(ctx, experimentID)
	if err != nil {
		return "", translate(err, application.ErrExperimentNotFound, "get experiment status")
	}
	return status, nil
}

func (s *Store) Timepoint(ctx context.Context, experimentID, timepointID string) (application.TimepointRef, error) {
	if !pgtx.ValidUUID(experimentID) || !pgtx.ValidUUID(timepointID) {
		return application.TimepointRef{}, application.ErrTimepointNotFound
	}
	row, err := s.queries.GetTimepoint(ctx, sqlcgen.GetTimepointParams{ExperimentID: experimentID, ID: timepointID})
	if err != nil {
		return application.TimepointRef{}, translate(err, application.ErrTimepointNotFound, "get timepoint")
	}
	return application.TimepointRef{ID: row.ID, StudyDay: int(row.StudyDay), DefaultScheduledAt: row.DefaultScheduledAt.UTC()}, nil
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
		Trials: int(row.Trials),
	}, nil
}

func (s *Store) LockAllocations(ctx context.Context, experimentID string, ids []string) ([]application.AllocationRef, error) {
	valid := validIDs(ids)
	if !pgtx.ValidUUID(experimentID) || len(valid) == 0 {
		return []application.AllocationRef{}, nil
	}
	rows, err := s.queries.LockAllocations(ctx, sqlcgen.LockAllocationsParams{ExperimentID: experimentID, Ids: valid})
	if err != nil {
		return nil, fmt.Errorf("lock allocations: %w", err)
	}
	out := make([]application.AllocationRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, application.AllocationRef{ID: r.ID, SubjectID: r.SubjectID, Excluded: r.Excluded})
	}
	return out, nil
}

func (s *Store) Arenas(ctx context.Context, ids []string) ([]application.ArenaRef, error) {
	valid := validIDs(ids)
	if len(valid) == 0 {
		return []application.ArenaRef{}, nil
	}
	rows, err := s.queries.ListArenas(ctx, valid)
	if err != nil {
		return nil, fmt.Errorf("list arenas: %w", err)
	}
	out := make([]application.ArenaRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, application.ArenaRef{ID: r.ID, EnvironmentID: r.EnvironmentID, ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion)})
	}
	return out, nil
}

func (s *Store) ScheduledAllocations(ctx context.Context, slot application.Slot, ids []string, exceptTestID string) ([]string, error) {
	valid := validIDs(ids)
	if len(valid) == 0 || !pgtx.ValidUUID(slot.TimepointID) || !pgtx.ValidUUID(slot.ProtocolVersionID) {
		return []string{}, nil
	}
	var except *string
	if pgtx.ValidUUID(exceptTestID) {
		except = &exceptTestID
	}
	taken, err := s.queries.ScheduledAllocations(ctx, sqlcgen.ScheduledAllocationsParams{
		TimepointID: slot.TimepointID, ProtocolVersionID: slot.ProtocolVersionID, StepPosition: int32(slot.StepPosition), Ids: valid, ExceptTestID: except,
	})
	if err != nil {
		return nil, fmt.Errorf("scheduled allocations: %w", err)
	}
	return taken, nil
}

// CreateTest stores the test and its subjects; it must run inside
// Transaction so a rejected subject leaves no test behind.
func (s *Store) CreateTest(ctx context.Context, t domain.Test) (domain.Test, error) {
	id, err := s.queries.CreateTest(ctx, sqlcgen.CreateTestParams{
		ExperimentID: t.ExperimentID, TimepointID: t.TimepointID, ProtocolVersionID: t.ProtocolVersionID, StepPosition: int32(t.StepPosition),
		ParadigmKey: t.ParadigmKey, ParadigmVersion: int32(t.ParadigmVersion), PlannedTrials: int32(t.PlannedTrials),
		ScheduledAt: t.ScheduledAt, Notes: optional(t.Notes), CreatedBy: t.CreatedBy,
	})
	if err != nil {
		return domain.Test{}, translate(err, nil, "create test")
	}
	for _, subject := range t.Subjects {
		subject.TestID = id
		if err := s.AddTestSubject(ctx, t.ExperimentID, subject, t.ParadigmKey, t.ParadigmVersion); err != nil {
			return domain.Test{}, err
		}
	}
	return s.Test(ctx, id)
}

func (s *Store) AddTestSubject(ctx context.Context, experimentID string, ts domain.TestSubject, paradigmKey string, paradigmVersion int) error {
	if !pgtx.ValidUUID(ts.AllocationID) {
		return application.ErrAllocationNotFound
	}
	if !pgtx.ValidUUID(ts.EnvironmentRevisionID) {
		return application.ErrArenaNotFound
	}
	err := s.queries.CreateTestSubject(ctx, sqlcgen.CreateTestSubjectParams{
		ExperimentID: experimentID, TestID: ts.TestID, AllocationID: ts.AllocationID, EnvironmentRevisionID: ts.EnvironmentRevisionID,
		ParadigmKey: paradigmKey, ParadigmVersion: int32(paradigmVersion), CreatedBy: ts.CreatedBy,
	})
	if err != nil {
		return translate(err, nil, "create test subject")
	}
	return nil
}

func (s *Store) RemoveTestSubject(ctx context.Context, testID, subjectID string) error {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(subjectID) {
		return application.ErrTestSubjectNotFound
	}
	deleted, err := s.queries.DeleteTestSubject(ctx, sqlcgen.DeleteTestSubjectParams{TestID: testID, ID: subjectID})
	if err != nil {
		return translate(err, nil, "delete test subject")
	}
	if deleted == 0 {
		return application.ErrTestSubjectNotFound
	}
	return nil
}

func (s *Store) Test(ctx context.Context, id string) (domain.Test, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Test{}, application.ErrTestNotFound
	}
	row, err := s.queries.GetTest(ctx, id)
	if err != nil {
		return domain.Test{}, translate(err, application.ErrTestNotFound, "get test")
	}
	return s.withSubject(ctx, toTest(row))
}

func (s *Store) LockTest(ctx context.Context, id string) (domain.Test, error) {
	if !pgtx.ValidUUID(id) {
		return domain.Test{}, application.ErrTestNotFound
	}
	row, err := s.queries.LockTest(ctx, id)
	if err != nil {
		return domain.Test{}, translate(err, application.ErrTestNotFound, "lock test")
	}
	return s.withSubject(ctx, toTest(sqlcgen.GetTestRow(row)))
}

func (s *Store) UpdateTestStatus(ctx context.Context, t domain.Test, from domain.Status) (domain.Test, error) {
	updated, err := s.queries.UpdateTestStatus(ctx, sqlcgen.UpdateTestStatusParams{
		ID: t.ID, FromStatus: string(from), Status: string(t.Status), StartedAt: t.StartedAt, CompletedAt: t.CompletedAt,
		CancelledAt: t.CancelledAt, CancelReason: optional(t.CancelReason),
	})
	if err != nil {
		return domain.Test{}, translate(err, nil, "update test status")
	}
	if updated == 0 {
		return domain.Test{}, domain.ErrInvalidTransition
	}
	return s.Test(ctx, t.ID)
}

// ListTests lists tests with their subjects. An empty experiment id means
// every experiment.
func (s *Store) ListTests(ctx context.Context, f application.TestFilter) ([]domain.Test, int, error) {
	for _, id := range []string{f.ExperimentID, f.TimepointID, f.SubjectID} {
		if id != "" && !pgtx.ValidUUID(id) {
			return []domain.Test{}, 0, nil
		}
	}
	from, err := date(f.From)
	if err != nil {
		return nil, 0, err
	}
	to, err := date(f.To)
	if err != nil {
		return nil, 0, err
	}
	params := sqlcgen.ListTestsParams{
		ExperimentID: optional(f.ExperimentID), TimepointID: optional(f.TimepointID), SubjectID: optional(f.SubjectID),
		Statuses: statuses(f.Statuses), ParadigmKey: f.ParadigmKey, Search: pgtx.EscapeLike(f.Search), FromDate: from, ToDate: to, NewestFirst: f.NewestFirst,
		PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	}
	rows, err := s.queries.ListTests(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list tests: %w", err)
	}
	total, err := s.queries.CountTests(ctx, sqlcgen.CountTestsParams{
		ExperimentID: params.ExperimentID, TimepointID: params.TimepointID, SubjectID: params.SubjectID,
		Statuses: params.Statuses, ParadigmKey: params.ParadigmKey, Search: params.Search, FromDate: from, ToDate: to,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count tests: %w", err)
	}
	out := make([]domain.Test, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTest(sqlcgen.GetTestRow(row)))
	}
	if out, err = s.withSubjects(ctx, out); err != nil {
		return nil, 0, err
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
		TestID: tr.TestID, TestSubjectID: tr.TestSubjectID, Number: int32(tr.Number), Repetition: int32(tr.Repetition), Attempt: int32(tr.Attempt),
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

func (s *Store) withSubject(ctx context.Context, t domain.Test) (domain.Test, error) {
	tests, err := s.withSubjects(ctx, []domain.Test{t})
	if err != nil {
		return domain.Test{}, err
	}
	return tests[0], nil
}

// withSubjects loads the subjects of all tests with one query.
func (s *Store) withSubjects(ctx context.Context, tests []domain.Test) ([]domain.Test, error) {
	if len(tests) == 0 {
		return tests, nil
	}
	ids := make([]string, 0, len(tests))
	for _, t := range tests {
		ids = append(ids, t.ID)
	}
	rows, err := s.queries.ListTestSubjects(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list test subjects: %w", err)
	}
	subjects := map[string][]domain.TestSubject{}
	for _, r := range rows {
		subjects[r.TestID] = append(subjects[r.TestID], domain.TestSubject{
			ID: r.ID, TestID: r.TestID, AllocationID: r.AllocationID, SubjectID: r.SubjectID, SubjectCode: r.SubjectCode, Excluded: r.Excluded,
			EnvironmentRevisionID: r.EnvironmentRevisionID, EnvironmentID: r.EnvironmentID, EnvironmentName: r.EnvironmentName,
			EnvironmentRevision: int(r.EnvironmentRevision), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
		})
	}
	for i := range tests {
		tests[i].Subjects = subjects[tests[i].ID]
		if tests[i].Subjects == nil {
			tests[i].Subjects = []domain.TestSubject{}
		}
	}
	return tests, nil
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

func toTest(r sqlcgen.GetTestRow) domain.Test {
	return domain.Test{
		ID: r.ID, ExperimentID: r.ExperimentID, ExperimentCode: r.ExperimentCode, TimepointID: r.TimepointID, TimepointName: r.TimepointName,
		StudyDay: int(r.StudyDay), ProtocolID: r.ProtocolID, ProtocolName: r.ProtocolName, ProtocolVersionID: r.ProtocolVersionID,
		ProtocolVersion: int(r.ProtocolVersion), StepPosition: int(r.StepPosition), ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion),
		PlannedTrials: int(r.PlannedTrials), Status: domain.Status(r.Status),
		ScheduledAt: r.ScheduledAt.UTC(), StartedAt: utc(r.StartedAt), CompletedAt: utc(r.CompletedAt), CancelledAt: utc(r.CancelledAt),
		CancelReason: value(r.CancelReason), Notes: value(r.Notes), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func toTrial(r sqlcgen.MiskoTrial) domain.Trial {
	return domain.Trial{
		ID: r.ID, TestID: r.TestID, TestSubjectID: r.TestSubjectID, Number: int(r.Number), Repetition: int(r.Repetition), Attempt: int(r.Attempt),
		StartedAt: r.StartedAt.UTC(), EndedAt: utc(r.EndedAt), Notes: value(r.Notes), RecordedBy: r.RecordedBy, CreatedAt: r.CreatedAt.UTC(),
	}
}

func toComment(r sqlcgen.GetCommentRow) domain.Comment {
	return domain.Comment{
		ID: r.ID, TestID: r.TestID, AuthorID: r.AuthorID, AuthorName: r.AuthorName, Body: r.Body, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

// date parses an inclusive filter date; the use case already validated it.
func date(raw string) (pgtype.Date, error) {
	if raw == "" {
		return pgtype.Date{}, nil
	}
	d, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return pgtype.Date{}, application.ErrInvalidQuery
	}
	return pgtype.Date{Time: d, Valid: true}, nil
}

func validIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if pgtx.ValidUUID(id) {
			out = append(out, id)
		}
	}
	return out
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

func statuses(in []domain.Status) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, string(s))
	}
	return out
}
