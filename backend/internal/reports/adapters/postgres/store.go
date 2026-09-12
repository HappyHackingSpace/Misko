// Package postgres reads reports from the read-only reporting views of
// schema/012_reports.sql. It never writes.
package postgres

import (
	"context"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct {
	queries *sqlcgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{queries: sqlcgen.New(pool)} }

// The application validates identifiers, so only well-formed UUIDs reach SQL.
func (s *Store) MetricRows(ctx context.Context, f application.Filter) ([]domain.MetricRow, error) {
	rows, err := s.queries.MetricRows(ctx, sqlcgen.MetricRowsParams{
		LatestOnly: f.LatestOnly, ExperimentID: optional(f.ExperimentID), SubjectID: optional(f.SubjectID), GroupID: optional(f.GroupID),
		PhaseID: optional(f.PhaseID), TestID: optional(f.TestID), EnvironmentID: optional(f.EnvironmentID), VideoID: optional(f.VideoID),
		RunID: optional(f.RunID), ParadigmKey: f.ParadigmKey, ParadigmVersion: int32(f.ParadigmVersion),
		MetricEngineVersion: int32(f.MetricEngineVersion), MetricKey: f.MetricKey, DiseaseModelID: optional(f.DiseaseModelID),
		SubstanceID: optional(f.SubstanceID), SortKey: f.Sort, Descending: f.Descending, PageOffset: int32(f.Offset), PageLimit: int32(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("metric report: %w", err)
	}
	out := make([]domain.MetricRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.MetricRow{
			Provenance: provenance(r.MiskoReportRun, r.MiskoReportTest), MetricKey: r.MetricKey, Unit: r.Unit, Value: r.Value, MissingReason: text(r.MissingReason),
		})
	}
	return out, nil
}

func (s *Store) CountMetricRows(ctx context.Context, f application.Filter) (int, error) {
	n, err := s.queries.CountMetricRows(ctx, sqlcgen.CountMetricRowsParams{
		LatestOnly: f.LatestOnly, ExperimentID: optional(f.ExperimentID), SubjectID: optional(f.SubjectID), GroupID: optional(f.GroupID),
		PhaseID: optional(f.PhaseID), TestID: optional(f.TestID), EnvironmentID: optional(f.EnvironmentID), VideoID: optional(f.VideoID),
		RunID: optional(f.RunID), ParadigmKey: f.ParadigmKey, ParadigmVersion: int32(f.ParadigmVersion),
		MetricEngineVersion: int32(f.MetricEngineVersion), MetricKey: f.MetricKey, DiseaseModelID: optional(f.DiseaseModelID),
		SubstanceID: optional(f.SubstanceID),
	})
	if err != nil {
		return 0, fmt.Errorf("count metric report: %w", err)
	}
	return int(n), nil
}

func (s *Store) EventRows(ctx context.Context, f application.Filter) ([]domain.EventRow, error) {
	rows, err := s.queries.EventRows(ctx, sqlcgen.EventRowsParams{
		LatestOnly: f.LatestOnly, ExperimentID: optional(f.ExperimentID), SubjectID: optional(f.SubjectID), GroupID: optional(f.GroupID),
		PhaseID: optional(f.PhaseID), TestID: optional(f.TestID), EnvironmentID: optional(f.EnvironmentID), VideoID: optional(f.VideoID),
		RunID: optional(f.RunID), ParadigmKey: f.ParadigmKey, ParadigmVersion: int32(f.ParadigmVersion),
		MetricEngineVersion: int32(f.MetricEngineVersion), EventType: f.EventType, DiseaseModelID: optional(f.DiseaseModelID),
		SubstanceID: optional(f.SubstanceID), SortKey: f.Sort, Descending: f.Descending, PageOffset: int32(f.Offset), PageLimit: int32(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("event report: %w", err)
	}
	out := make([]domain.EventRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.EventRow{
			Provenance: provenance(r.MiskoReportRun, r.MiskoReportTest), EventID: r.EventID, EventType: r.EventType, Kind: r.Kind,
			StartUs: r.StartUs, EndUs: r.EndUs, Confidence: float64(r.Confidence), TrialID: text(r.TrialID),
			TrialNumber: number(r.TrialNumber), TrialRepetition: number(r.TrialRepetition), TrialAttempt: number(r.TrialAttempt),
		})
	}
	return out, nil
}

func (s *Store) CountEventRows(ctx context.Context, f application.Filter) (int, error) {
	n, err := s.queries.CountEventRows(ctx, sqlcgen.CountEventRowsParams{
		LatestOnly: f.LatestOnly, ExperimentID: optional(f.ExperimentID), SubjectID: optional(f.SubjectID), GroupID: optional(f.GroupID),
		PhaseID: optional(f.PhaseID), TestID: optional(f.TestID), EnvironmentID: optional(f.EnvironmentID), VideoID: optional(f.VideoID),
		RunID: optional(f.RunID), ParadigmKey: f.ParadigmKey, ParadigmVersion: int32(f.ParadigmVersion),
		MetricEngineVersion: int32(f.MetricEngineVersion), EventType: f.EventType, DiseaseModelID: optional(f.DiseaseModelID),
		SubstanceID: optional(f.SubstanceID),
	})
	if err != nil {
		return 0, fmt.Errorf("count event report: %w", err)
	}
	return int(n), nil
}

func provenance(r sqlcgen.MiskoReportRun, t sqlcgen.MiskoReportTest) domain.Provenance {
	var finished time.Time
	if r.FinishedAt != nil {
		finished = r.FinishedAt.UTC()
	}
	return domain.Provenance{
		RunID: r.ID, Latest: r.Latest, Trigger: r.Trigger, ModelVersion: text(r.ModelVersion), MetricEngineVersion: int(r.MetricEngineVersion),
		ResultSchemaVersion: int(r.ResultSchemaVersion), CalibrationID: text(r.CalibrationID), RecordingID: r.RecordingID, SourceVideoID: r.SourceAssetID,
		RunFinishedAt: finished, TestID: t.TestID, TestStatus: t.TestStatus, ScheduledAt: t.ScheduledAt.UTC(), TrialCount: int(t.TrialCount),
		ExperimentID: t.ExperimentID, ExperimentCode: t.ExperimentCode, SubjectID: t.SubjectID, SubjectCode: t.SubjectCode, Species: t.Species, Sex: t.Sex,
		PhaseID: text(t.PhaseID), PhaseName: text(t.PhaseName), GroupID: text(t.GroupID), GroupName: text(t.GroupName), GroupRole: text(t.GroupRole),
		ParadigmKey: t.ParadigmKey, ParadigmVersion: int(t.ParadigmVersion), EnvironmentID: t.EnvironmentID, EnvironmentName: t.EnvironmentName,
		EnvironmentRevision: int(t.EnvironmentRevision),
	}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func number(n *int32) int {
	if n == nil {
		return 0
	}
	return int(*n)
}
