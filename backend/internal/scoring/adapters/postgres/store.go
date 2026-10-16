// Package postgres implements scoring persistence with sqlc-generated queries.
// A completed sheet publishes its SCORED run in the analysis tables, so the
// review queue and the reports read it like any analysed run.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtx"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/scoring/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"maps"
)

// constraintErrors maps named constraints and trigger rules to the error a
// violation means, so the API answers with a code instead of a 500.
var constraintErrors = map[string]error{
	"scoring_sheets_draft_key":             application.ErrConflict,
	"scoring_sheets_version_key":           application.ErrConflict,
	"scoring_sheets_transition":            application.ErrConflict,
	"scoring_sheets_run_key":               application.ErrConflict,
	"scoring_sheets_run_scope":             application.ErrNoVideo,
	"scoring_sheets_recording_fkey":        application.ErrNoVideo,
	"scoring_sheets_test_started":          application.ErrNotScorable,
	"scoring_sheets_test_subject_fkey":     application.ErrTestSubjectNotFound,
	"scoring_sheets_test_fkey":             application.ErrTestSubjectNotFound,
	"scored_observations_frozen":           domain.ErrNotDraft,
	"scored_observations_sheet_fkey":       application.ErrConflict,
	"scored_observations_time_check":       domain.ErrInvalidEventTime,
	"scored_observations_start_us_check":   domain.ErrInvalidEventTime,
	"scored_observations_label_check":      domain.ErrInvalidLabel,
	"scored_observations_event_type_check": domain.ErrUnknownEventType,
	"analysis_runs_test_subject_fkey":      application.ErrTestSubjectNotFound,
	"analysis_runs_recording_fkey":         application.ErrNoVideo,
	"analysis_runs_source_fkey":            application.ErrNoVideo,
	"analysis_runs_paradigm_fkey":          application.ErrConflict,
	"analysis_runs_published":              application.ErrConflict,
	"analysis_runs_transition":             application.ErrConflict,
	"subject_results_succeeded":            application.ErrConflict,
	"subject_results_run_fkey":             application.ErrConflict,
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
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, queries: s.queries.WithTx(tx), inTx: true})
	})
	// Deferred constraint triggers report their violation at commit.
	if _, constraint := pgtx.Violation(err); constraintErrors[constraint] != nil {
		return constraintErrors[constraint]
	}
	return err
}

func (s *Store) LockSubject(ctx context.Context, testSubjectID string) error {
	if !pgtx.ValidUUID(testSubjectID) {
		return application.ErrTestSubjectNotFound
	}
	if err := s.queries.LockSubject(ctx, testSubjectID); err != nil {
		return fmt.Errorf("lock scoring: %w", err)
	}
	return nil
}

func (s *Store) Subject(ctx context.Context, testID, testSubjectID string) (application.Subject, error) {
	if !pgtx.ValidUUID(testID) || !pgtx.ValidUUID(testSubjectID) {
		return application.Subject{}, application.ErrTestSubjectNotFound
	}
	row, err := s.queries.GetSubject(ctx, sqlcgen.GetSubjectParams{TestID: testID, TestSubjectID: testSubjectID})
	if err != nil {
		return application.Subject{}, translate(err, application.ErrTestSubjectNotFound, "get test subject")
	}
	out := application.Subject{
		ExperimentID: row.ExperimentID, TestID: row.TestID, TestSubjectID: row.TestSubjectID, ExperimentStatus: row.ExperimentStatus,
		TestStatus: row.TestStatus, Excluded: row.Excluded, ParadigmKey: row.ParadigmKey, ParadigmVersion: int(row.ParadigmVersion),
	}
	out.Parameters, out.Invalid = parameters(row.Parameters, row.Session)
	if row.RecordingID != "" {
		out.Video = &domain.Video{
			Clip:    domain.Clip{RecordingID: row.RecordingID, StartUs: row.ClipStartUs, EndUs: row.ClipEndUs},
			AssetID: row.AssetID, Generation: row.Generation, CRC32C: uint32(row.Crc32c),
		}
	}
	return out, nil
}

func (s *Store) LatestSheet(ctx context.Context, testSubjectID string) (*domain.Sheet, error) {
	row, err := s.queries.LatestSheet(ctx, testSubjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest scoring sheet: %w", err)
	}
	sheet := toSheet(row)
	return &sheet, nil
}

func (s *Store) CreateSheet(ctx context.Context, sheet domain.Sheet) (domain.Sheet, error) {
	row, err := s.queries.CreateSheet(ctx, sqlcgen.CreateSheetParams{
		ExperimentID: sheet.ExperimentID, TestID: sheet.TestID, TestSubjectID: sheet.TestSubjectID, Version: int32(sheet.Version), CreatedBy: sheet.CreatedBy,
	})
	if err != nil {
		return domain.Sheet{}, translate(err, nil, "create scoring sheet")
	}
	return toSheet(row), nil
}

func (s *Store) CompleteSheet(ctx context.Context, sheet domain.Sheet) (domain.Sheet, error) {
	row, err := s.queries.CompleteSheet(ctx, sqlcgen.CompleteSheetParams{
		ID: sheet.ID, ScoredTypes: sheet.ScoredTypes, DurationUs: sheet.DurationUs, RecordingID: &sheet.RecordingID, RunID: &sheet.RunID,
		CompletedBy: &sheet.CompletedBy, CompletedAt: sheet.CompletedAt,
	})
	if err != nil {
		return domain.Sheet{}, translate(err, domain.ErrNotDraft, "complete scoring sheet")
	}
	return toSheet(row), nil
}

func (s *Store) Observations(ctx context.Context, sheetID string) ([]domain.Observation, error) {
	rows, err := s.queries.ListObservations(ctx, sheetID)
	if err != nil {
		return nil, fmt.Errorf("list observations: %w", err)
	}
	out := make([]domain.Observation, 0, len(rows))
	for _, r := range rows {
		out = append(out, toObservation(r))
	}
	return out, nil
}

func (s *Store) CreateObservation(ctx context.Context, o domain.Observation) (domain.Observation, error) {
	row, err := s.queries.CreateObservation(ctx, sqlcgen.CreateObservationParams{
		SheetID: o.SheetID, TestSubjectID: o.TestSubjectID, EventType: o.Type, Kind: string(o.Kind), Label: optional(o.Label),
		StartUs: o.StartUs, EndUs: o.EndUs, ScoredBy: o.ScoredBy,
	})
	if err != nil {
		return domain.Observation{}, translate(err, nil, "create observation")
	}
	return toObservation(row), nil
}

func (s *Store) UpdateObservation(ctx context.Context, o domain.Observation) (domain.Observation, error) {
	if !pgtx.ValidUUID(o.ID) {
		return domain.Observation{}, application.ErrEventNotFound
	}
	row, err := s.queries.UpdateObservation(ctx, sqlcgen.UpdateObservationParams{
		ID: o.ID, SheetID: o.SheetID, EventType: o.Type, Kind: string(o.Kind), Label: optional(o.Label), StartUs: o.StartUs, EndUs: o.EndUs, ScoredBy: o.ScoredBy,
	})
	if err != nil {
		return domain.Observation{}, translate(err, application.ErrEventNotFound, "update observation")
	}
	return toObservation(row), nil
}

func (s *Store) DeleteObservation(ctx context.Context, sheetID, id string) error {
	if !pgtx.ValidUUID(id) {
		return application.ErrEventNotFound
	}
	n, err := s.queries.DeleteObservation(ctx, sqlcgen.DeleteObservationParams{ID: id, SheetID: sheetID})
	if err != nil {
		return translate(err, nil, "delete observation")
	}
	if n == 0 {
		return application.ErrEventNotFound
	}
	return nil
}

func (s *Store) CopyObservations(ctx context.Context, from, to domain.Sheet) error {
	if err := s.queries.CopyObservations(ctx, sqlcgen.CopyObservationsParams{FromSheetID: from.ID, ToSheetID: to.ID}); err != nil {
		return translate(err, nil, "copy observations")
	}
	return nil
}

// CreateRun must run in the transaction that completes the run's sheet: a
// deferred constraint rejects a scored run without its completed sheet.
func (s *Store) CreateRun(ctx context.Context, r domain.Run) (string, error) {
	params, err := json.Marshal(nonNil(r.Parameters))
	if err != nil {
		return "", fmt.Errorf("encode run parameters: %w", err)
	}
	id, err := s.queries.CreateScoredRun(ctx, sqlcgen.CreateScoredRunParams{
		ExperimentID: r.ExperimentID, TestID: r.TestID, TestSubjectID: r.TestSubjectID, RecordingID: r.Video.RecordingID,
		SourceAssetID: r.Video.AssetID, SourceGeneration: r.Video.Generation, SourceCrc32c: int64(r.Video.CRC32C), ClipStartUs: r.Video.StartUs,
		ClipEndUs: r.Video.EndUs, ParadigmKey: r.ParadigmKey, ParadigmVersion: int32(r.ParadigmVersion),
		MetricEngineVersion: int32(r.Computed.MetricEngineVersion), ResultSchemaVersion: int32(r.Computed.ResultSchemaVersion),
		Parameters: params, AvailableAt: r.FinishedAt, ScoredBy: &r.ScoredBy,
	})
	if err != nil {
		return "", translate(err, nil, "create scored run")
	}
	for _, m := range r.Computed.Metrics {
		if err := s.queries.CreateMetric(ctx, sqlcgen.CreateMetricParams{RunID: id, MetricKey: m.Key, Unit: m.Unit, Value: m.Value, MissingReason: optional(m.MissingReason)}); err != nil {
			return "", translate(err, nil, "create metric")
		}
	}
	for _, e := range r.Computed.Events {
		if err := s.queries.CreateEvent(ctx, sqlcgen.CreateEventParams{RunID: id, TestSubjectID: r.TestSubjectID, EventType: e.Type, Kind: string(e.Kind),
			StartUs: e.StartUs, EndUs: e.EndUs}); err != nil {
			return "", translate(err, nil, "create event")
		}
	}
	finished := r.FinishedAt
	n, err := s.queries.FinishScoredRun(ctx, sqlcgen.FinishScoredRunParams{ID: id, FinishedAt: &finished})
	if err != nil {
		return "", translate(err, nil, "finish scored run")
	}
	if n != 1 {
		return "", application.ErrConflict
	}
	return id, nil
}

func (s *Store) OpenResult(ctx context.Context, testSubjectID, runID string) error {
	if err := s.queries.OpenResult(ctx, sqlcgen.OpenResultParams{TestSubjectID: testSubjectID, RunID: runID}); err != nil {
		return translate(err, nil, "open result")
	}
	return nil
}

func toSheet(r sqlcgen.MiskoScoringSheet) domain.Sheet {
	out := domain.Sheet{
		ID: r.ID, ExperimentID: r.ExperimentID, TestID: r.TestID, TestSubjectID: r.TestSubjectID, Version: int(r.Version), Status: domain.Status(r.Status),
		ScoredTypes: r.ScoredTypes, DurationUs: r.DurationUs, RecordingID: value(r.RecordingID), RunID: value(r.RunID), CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt.UTC(), CompletedBy: value(r.CompletedBy),
	}
	if r.CompletedAt != nil {
		at := r.CompletedAt.UTC()
		out.CompletedAt = &at
	}
	return out
}

func toObservation(r sqlcgen.MiskoScoredObservation) domain.Observation {
	return domain.Observation{
		ID: r.ID, SheetID: r.SheetID, TestSubjectID: r.TestSubjectID, Type: r.EventType, Kind: domain.Kind(r.Kind), Label: value(r.Label),
		StartUs: r.StartUs, EndUs: r.EndUs, ScoredBy: r.ScoredBy, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

// parameters snapshots the step's session values and the arena's apparatus
// and analysis values. The arena wins on a shared key, as for analysed runs.
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

func nonNil(m map[string]float64) map[string]float64 {
	if m == nil {
		return map[string]float64{}
	}
	return m
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
