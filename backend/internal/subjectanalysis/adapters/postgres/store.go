// Package postgres implements the experiment side of analysis with
// sqlc-generated queries: links between animals of tests and analysis jobs,
// the readiness of animals and the review of their results.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjectanalysis/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// constraintErrors maps named constraints and trigger rules to the error a
// violation means, so the API answers with a code instead of a 500.
var constraintErrors = map[string]error{
	"test_subject_analyses_test_subject_fkey": application.ErrTestSubjectNotFound,
	"test_subject_analyses_test_fkey":         application.ErrTestSubjectNotFound,
	"test_subject_analyses_recording_fkey":    application.ErrNotReady,
	"test_subject_analyses_requester":         application.ErrNotReady,
	"subject_results_succeeded":               domain.ErrRunNotChosen,
	"subject_results_job_fkey":                domain.ErrRunNotChosen,
	"subject_results_rejection_check":         domain.ErrInvalidRejection,
	"subject_results_note_check":              domain.ErrInvalidNote,
	"subject_results_test_subject_id_fkey":    application.ErrTestSubjectNotFound,
}

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	inTx    bool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool, queries: sqlcgen.New(pool)} }

// Transaction runs fn in a transaction, or in the current one when nested.
func (s *Store) Transaction(ctx context.Context, fn func(application.Store) error) error {
	if s.inTx {
		return fn(s)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, queries: s.queries.WithTx(tx), inTx: true})
	})
}

func (s *Store) ReadyTestSubjects(ctx context.Context, calibrated []string) ([]domain.Candidate, error) {
	rows, err := s.queries.ReadyTestSubjects(ctx, calibrated)
	if err != nil {
		return nil, fmt.Errorf("ready test subjects: %w", err)
	}
	out := make([]domain.Candidate, 0, len(rows))
	for _, r := range rows {
		// A row whose parameters cannot be read is returned as invalid so the
		// orchestrator skips it and still submits the others.
		params, invalid := parameters(r.Parameters, r.Session)
		out = append(out, domain.Candidate{
			ExperimentID: r.ExperimentID, TestID: r.TestID, TestSubjectID: r.TestSubjectID, RecordingID: r.RecordingID, SourceAssetID: r.SourceAssetID,
			ClipStartUs: r.ClipStartUs, ClipEndUs: r.ClipEndUs, CalibrationID: r.CalibrationID, ZoneKeys: r.ZoneKeys, ParadigmKey: r.ParadigmKey,
			ParadigmVersion: int(r.ParadigmVersion), Parameters: params,
			// The query returns eligible candidates only.
			VideoVerified: true, ExperimentStatus: "RUNNING", Invalid: invalid,
		})
	}
	return out, nil
}

func (s *Store) Candidate(ctx context.Context, testSubjectID string) (domain.Candidate, error) {
	if !pgtx.ValidUUID(testSubjectID) {
		return domain.Candidate{}, application.ErrTestSubjectNotFound
	}
	r, err := s.queries.GetCandidate(ctx, testSubjectID)
	if err != nil {
		return domain.Candidate{}, translate(err, application.ErrTestSubjectNotFound, "get candidate")
	}
	params, invalid := parameters(r.Parameters, r.Session)
	return domain.Candidate{
		ExperimentID: r.ExperimentID, TestID: r.TestID, TestSubjectID: r.TestSubjectID, RecordingID: r.RecordingID, SourceAssetID: r.SourceAssetID,
		ClipStartUs: r.ClipStartUs, ClipEndUs: r.ClipEndUs, CalibrationID: r.CalibrationID, ZoneKeys: r.ZoneKeys, ParadigmKey: r.ParadigmKey,
		ParadigmVersion: int(r.ParadigmVersion), Parameters: params, VideoVerified: r.VideoStatus == "VERIFIED", TestCancelled: r.TestStatus == "CANCELLED",
		ExperimentStatus: r.ExperimentStatus, Excluded: r.Excluded, Invalid: invalid,
	}, nil
}

func (s *Store) LockTestSubject(ctx context.Context, testSubjectID string) error {
	if err := s.queries.LockTestSubjectAnalyses(ctx, testSubjectID); err != nil {
		return fmt.Errorf("lock test subject analyses: %w", err)
	}
	return nil
}

func (s *Store) CreateLink(ctx context.Context, l domain.Link) (bool, error) {
	n, err := s.queries.CreateLink(ctx, sqlcgen.CreateLinkParams{
		JobID: l.JobID, ExperimentID: l.ExperimentID, TestID: l.TestID, TestSubjectID: l.TestSubjectID, RecordingID: l.RecordingID,
		CalibrationID: optional(l.CalibrationID), Trigger: string(l.Trigger), CreatedBy: optional(l.CreatedBy),
	})
	if err != nil {
		return false, translate(err, nil, "create link")
	}
	return n == 1, nil
}

func (s *Store) Link(ctx context.Context, jobID string) (domain.Link, error) {
	if !pgtx.ValidUUID(jobID) {
		return domain.Link{}, application.ErrRunNotFound
	}
	row, err := s.queries.GetLink(ctx, jobID)
	if err != nil {
		return domain.Link{}, translate(err, application.ErrRunNotFound, "get link")
	}
	return toLink(row), nil
}

func (s *Store) Links(ctx context.Context, testSubjectIDs []string) ([]domain.Link, error) {
	rows, err := s.queries.ListLinks(ctx, validUUIDs(testSubjectIDs))
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	return toLinks(rows), nil
}

func (s *Store) TestLinks(ctx context.Context, testID, testSubjectID string) ([]domain.Link, error) {
	if !pgtx.ValidUUID(testID) || (testSubjectID != "" && !pgtx.ValidUUID(testSubjectID)) {
		return []domain.Link{}, nil
	}
	rows, err := s.queries.ListTestLinks(ctx, sqlcgen.ListTestLinksParams{TestID: testID, TestSubjectID: optional(testSubjectID)})
	if err != nil {
		return nil, fmt.Errorf("list test links: %w", err)
	}
	return toLinks(rows), nil
}

func (s *Store) Unopened(ctx context.Context, limit int) ([]domain.Link, error) {
	rows, err := s.queries.Unopened(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("unopened results: %w", err)
	}
	out := make([]domain.Link, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Link{TestSubjectID: r.TestSubjectID, JobID: r.JobID})
	}
	return out, nil
}

// OpenResult opens nothing for a job that is not linked to the animal yet;
// the orchestrator opens it once the link exists.
func (s *Store) OpenResult(ctx context.Context, r domain.SubjectResult) error {
	if !pgtx.ValidUUID(r.TestSubjectID) || !pgtx.ValidUUID(r.JobID) {
		return nil
	}
	err := s.queries.OpenResult(ctx, sqlcgen.OpenResultParams{TestSubjectID: r.TestSubjectID, JobID: r.JobID})
	if _, constraint := pgtx.Violation(err); constraint == "subject_results_job_fkey" {
		return nil
	}
	if err != nil {
		return translate(err, nil, "open result")
	}
	return nil
}

func (s *Store) LockResult(ctx context.Context, testSubjectID string) (domain.SubjectResult, string, error) {
	if !pgtx.ValidUUID(testSubjectID) {
		return domain.SubjectResult{}, "", application.ErrResultNotFound
	}
	row, err := s.queries.LockResult(ctx, testSubjectID)
	if err != nil {
		return domain.SubjectResult{}, "", translate(err, application.ErrResultNotFound, "lock result")
	}
	return toResult(sqlcgen.MiskoSubjectResult{TestSubjectID: row.TestSubjectID, JobID: row.JobID, Status: row.Status, Reason: row.Reason, Note: row.Note,
		ReviewedBy: row.ReviewedBy, ReviewedAt: row.ReviewedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}), row.ExperimentStatus, nil
}

func (s *Store) UpdateResult(ctx context.Context, r domain.SubjectResult) (domain.SubjectResult, error) {
	row, err := s.queries.UpdateResult(ctx, sqlcgen.UpdateResultParams{
		TestSubjectID: r.TestSubjectID, JobID: r.JobID, Status: string(r.Status), Reason: optional(r.Reason), Note: optional(r.Note),
		ReviewedBy: optional(r.ReviewedBy), ReviewedAt: r.ReviewedAt,
	})
	if err != nil {
		return domain.SubjectResult{}, translate(err, application.ErrResultNotFound, "update result")
	}
	return toResult(row), nil
}

func (s *Store) ReviewItems(ctx context.Context, f application.ReviewFilter) ([]application.ReviewItem, int, error) {
	rows, err := s.queries.ReviewItems(ctx, sqlcgen.ReviewItemsParams{
		ExperimentID: optional(f.ExperimentID), TestSubjectID: optional(f.TestSubjectID), Status: f.Status,
		PageLimit: int32(f.Limit), PageOffset: int32(f.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("review items: %w", err)
	}
	out := make([]application.ReviewItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, toReviewItem(r))
	}
	if f.TestSubjectID != "" {
		return out, len(out), nil
	}
	total, err := s.queries.CountReviewItems(ctx, sqlcgen.CountReviewItemsParams{ExperimentID: optional(f.ExperimentID), Status: f.Status})
	if err != nil {
		return nil, 0, fmt.Errorf("count review items: %w", err)
	}
	return out, int(total), nil
}

func toReviewItem(r sqlcgen.ReviewItemsRow) application.ReviewItem {
	return application.ReviewItem{
		Result: toResult(sqlcgen.MiskoSubjectResult{TestSubjectID: r.TestSubjectID, JobID: r.JobID, Status: r.Status, Reason: r.Reason, Note: r.Note,
			ReviewedBy: r.ReviewedBy, ReviewedAt: r.ReviewedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}),
		TestID: r.TestID, ExperimentID: r.ExperimentID, ExperimentCode: r.ExperimentCode, ExperimentStatus: r.ExperimentStatus, Blinded: r.Blinded,
		SubjectCode: r.SubjectCode, Excluded: r.Excluded, EnvironmentRevisionID: r.EnvironmentRevisionID, EnvironmentID: r.EnvironmentID,
		EnvironmentName: r.EnvironmentName, EnvironmentRevision: int(r.EnvironmentRevision), TimepointID: r.TimepointID, TimepointName: r.TimepointName,
		ParadigmKey: r.ParadigmKey, ParadigmVersion: int(r.ParadigmVersion), ScheduledAt: r.ScheduledAt.UTC(),
		Run: application.ReviewRun{ID: r.JobID, Trigger: r.Trigger, CalibrationID: value(r.CalibrationID), RecordingID: r.RecordingID},
		Group: &application.ReviewGroup{ID: r.GroupID, Code: r.GroupCode, Name: r.GroupName, Role: r.GroupRole, DiseaseModelName: value(r.DiseaseModelName),
			SubstanceName: value(r.TreatmentSubstanceName), DoseMicro: r.TreatmentDoseMicro, Unit: value(r.TreatmentUnit)},
	}
}

func toResult(r sqlcgen.MiskoSubjectResult) domain.SubjectResult {
	var reviewedAt = r.ReviewedAt
	if reviewedAt != nil {
		t := reviewedAt.UTC()
		reviewedAt = &t
	}
	return domain.SubjectResult{
		TestSubjectID: r.TestSubjectID, JobID: r.JobID, Status: domain.ResultStatus(r.Status), Reason: value(r.Reason), Note: value(r.Note),
		ReviewedBy: value(r.ReviewedBy), ReviewedAt: reviewedAt, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func toLinks(rows []sqlcgen.MiskoTestSubjectAnalysis) []domain.Link {
	out := make([]domain.Link, 0, len(rows))
	for _, r := range rows {
		out = append(out, toLink(r))
	}
	return out
}

func toLink(r sqlcgen.MiskoTestSubjectAnalysis) domain.Link {
	return domain.Link{JobID: r.JobID, ExperimentID: r.ExperimentID, TestID: r.TestID, TestSubjectID: r.TestSubjectID, RecordingID: r.RecordingID,
		CalibrationID: value(r.CalibrationID), Trigger: domain.Trigger(r.Trigger), CreatedBy: value(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()}
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

// parameters snapshots the step's session values and the arena's apparatus
// and analysis values (its environment revision's parameters). The arena wins
// on a shared key.
func parameters(arena, session []byte) (map[string]float64, error) {
	out := map[string]float64{}
	if err := json.Unmarshal(session, &out); err != nil {
		return nil, fmt.Errorf("decode session: %w", err)
	}
	environment := map[string]float64{}
	if err := json.Unmarshal(arena, &environment); err != nil {
		return nil, fmt.Errorf("decode arena parameters: %w", err)
	}
	maps.Copy(out, environment)
	return out, nil
}

func validUUIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if pgtx.ValidUUID(id) {
			out = append(out, id)
		}
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
