// Package postgres reads reports from the study design tables and the
// read-only reporting views of schema/012_reports.sql. It never writes.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/HappyHackingSpace/Misko/backend/internal/reports/adapters/postgres/sqlcgen"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	queries *sqlcgen.Queries
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{queries: sqlcgen.New(pool)} }

// The application validates identifiers, so only well-formed UUIDs reach SQL.
func (s *Store) Experiment(ctx context.Context, id string) (domain.Experiment, error) {
	row, err := s.queries.ReportExperiment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Experiment{}, application.ErrExperimentNotFound
	}
	if err != nil {
		return domain.Experiment{}, fmt.Errorf("report experiment: %w", err)
	}
	return domain.Experiment{ID: row.ID, Code: row.Code, Title: row.Title, Status: row.Status, Blinded: row.Blinded}, nil
}

func (s *Store) Groups(ctx context.Context, experimentID string) ([]domain.Group, error) {
	rows, err := s.queries.ReportGroups(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("report groups: %w", err)
	}
	out := make([]domain.Group, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Group{
			ID: r.ID, Code: r.Code, Name: r.Name, Role: r.Role, DiseaseModelID: text(r.DiseaseModelID), DiseaseModelName: text(r.DiseaseModelName),
			SubstanceID: text(r.TreatmentSubstanceID), SubstanceName: text(r.TreatmentSubstanceName), DoseMicro: r.TreatmentDoseMicro,
			Unit: text(r.TreatmentUnit), Allocated: int(r.Allocated), Excluded: int(r.Excluded),
		})
	}
	return out, nil
}

func (s *Store) Timepoints(ctx context.Context, experimentID string) ([]domain.Timepoint, error) {
	rows, err := s.queries.ReportTimepoints(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("report timepoints: %w", err)
	}
	out := make([]domain.Timepoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Timepoint{ID: r.ID, Name: r.Name, StudyDay: int(r.StudyDay), Position: int(r.Position)})
	}
	return out, nil
}

func (s *Store) Endpoints(ctx context.Context, experimentID string) ([]domain.Endpoint, error) {
	rows, err := s.queries.ReportEndpoints(ctx, experimentID)
	if err != nil {
		return nil, fmt.Errorf("report endpoints: %w", err)
	}
	out := make([]domain.Endpoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Endpoint{ID: r.ID, TimepointID: r.TimepointID, BaselineTimepointID: text(r.BaselineTimepointID),
			ParadigmKey: r.ParadigmKey, MetricKey: r.MetricKey, Kind: r.Kind, Direction: r.Direction})
	}
	return out, nil
}

func (s *Store) Measurements(ctx context.Context, experimentID, paradigmKey, metricKey string, limit int) ([]domain.Measurement, error) {
	rows, err := s.queries.ReportMeasurements(ctx, sqlcgen.ReportMeasurementsParams{
		ExperimentID: experimentID, ParadigmKey: paradigmKey, MetricKey: metricKey, RowLimit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("report measurements: %w", err)
	}
	out := make([]domain.Measurement, 0, len(rows))
	for _, r := range rows {
		m := domain.Measurement{
			TestSubjectID: r.TestSubjectID, TestID: r.TestID, AllocationID: r.AllocationID, SubjectID: r.SubjectID, SubjectCode: r.SubjectCode,
			GroupID: r.GroupID, TimepointID: r.TimepointID, Arena: r.EnvironmentName, ScheduledAt: r.ScheduledAt.UTC(), Excluded: r.Excluded,
			ResultStatus: text(r.ResultStatus), ResultJobID: text(r.ResultJobID), ParadigmVersion: int(r.ParadigmVersion),
			// metric_results.unit is NOT NULL: a unit means the accepted run published the metric.
			HasMetric: r.MetricUnit != nil, Unit: text(r.MetricUnit), Value: r.MetricValue, MissingReason: text(r.MissingReason),
		}
		if r.MetricEngineVersion != nil {
			m.MetricEngineVersion = int(*r.MetricEngineVersion)
		}
		out = append(out, m)
	}
	return out, nil
}

func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
